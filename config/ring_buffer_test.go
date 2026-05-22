//go:build testing

package config

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/architagr/lognugget/enum"
)

// Test_RingBuffer_PushPop_Sequential verifies FIFO ordering: push N events,
// pop N events, values arrive in the same order.
func Test_RingBuffer_PushPop_Sequential(t *testing.T) {
	r := newMpscRingBuffer()
	const n = 64

	for i := 0; i < n; i++ {
		r.Push(LogEvent{Level: enum.LogLevel(i), Data: nil})
	}
	for i := 0; i < n; i++ {
		e, ok := r.Pop()
		if !ok {
			t.Fatalf("expected event at i=%d, got empty", i)
		}
		if int(e.Level) != i {
			t.Errorf("FIFO violation at i=%d: got Level=%d", i, e.Level)
		}
	}
	_, ok := r.Pop()
	if ok {
		t.Error("expected empty ring after draining all events")
	}
}

// Test_RingBuffer_Empty verifies Pop on an empty ring returns (zero, false).
func Test_RingBuffer_Empty(t *testing.T) {
	r := newMpscRingBuffer()
	e, ok := r.Pop()
	if ok {
		t.Errorf("expected (_, false) on empty ring, got (%v, true)", e)
	}
	if e.Level != 0 || e.Data != nil {
		t.Errorf("expected zero LogEvent on empty pop, got %v", e)
	}
}

// Test_RingBuffer_Full_Spins fills the ring to capacity and verifies that a
// subsequent Push completes after a Pop frees one slot.
func Test_RingBuffer_Full_Spins(t *testing.T) {
	r := newMpscRingBuffer()

	// Fill ring completely.
	for i := 0; i < ringSize; i++ {
		r.Push(LogEvent{Level: enum.LogLevel(i), Data: nil})
	}

	pushed := make(chan struct{})
	go func() {
		r.Push(LogEvent{Level: enum.LevelInfo, Data: nil})
		close(pushed)
	}()

	// Brief pause to give the goroutine time to spin on a full ring.
	time.Sleep(5 * time.Millisecond)

	select {
	case <-pushed:
		t.Error("Push should not complete on a full ring before Pop frees a slot")
	default:
	}

	// Free one slot — Push should now succeed.
	r.Pop()

	select {
	case <-pushed:
		// correct
	case <-time.After(500 * time.Millisecond):
		t.Error("Push did not complete within 500ms after Pop freed a slot")
	}
}

// Test_RingBuffer_MPSC_Race runs 8 producer goroutines each pushing 1000
// events and 1 consumer goroutine popping all events. Verifies no data race
// (must be run with go test -race) and all events are received.
func Test_RingBuffer_MPSC_Race(t *testing.T) {
	const (
		producers  = 8
		perProducer = 1000
		total       = producers * perProducer
	)

	r := newMpscRingBuffer()
	var received atomic.Int64
	done := make(chan struct{})

	// Consumer.
	go func() {
		for {
			_, ok := r.Pop()
			if ok {
				if received.Add(1) == total {
					close(done)
					return
				}
			}
		}
	}()

	// Producers.
	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				r.Push(LogEvent{Level: enum.LevelInfo, Data: []byte{byte(id)}})
			}
		}(p)
	}
	wg.Wait()

	select {
	case <-done:
		// All events received.
	case <-time.After(5 * time.Second):
		t.Errorf("timed out: only %d/%d events received", received.Load(), total)
	}
}

// Test_RingBuffer_DrainOnStop pushes 100 events then triggers a config reset,
// which closes ringDoneCh and causes ProcessLogEvent to drain the ring before
// returning. Verifies all 100 events are dispatched — whether by normal pop
// or by the drain-on-stop path.
func Test_RingBuffer_DrainOnStop(t *testing.T) {
	TestResetConfig()

	var count atomic.Int64
	InitPreProcessors(&testCounterProc{n: &count})

	const n = 100
	payload := []byte(`{"level":"INFO"}`)
	for i := 0; i < n; i++ {
		PublishLog(enum.LevelInfo, payload)
	}

	// TestResetConfig closes the current ringDoneCh, triggering the drain path
	// in ProcessLogEvent, then starts a fresh ring+goroutine.
	TestResetConfig()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if count.Load() >= n {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := count.Load(); got < n {
		t.Errorf("drain-on-stop: %d/%d events dispatched", got, n)
	}
}

type testCounterProc struct{ n *atomic.Int64 }

func (c *testCounterProc) Name() string                           { return "counter" }
func (c *testCounterProc) PreProcess(_ enum.LogLevel, _ []byte)  { c.n.Add(1) }

// Test_RingBuffer_Len verifies Len() returns 0 on empty and ringSize on full.
func Test_RingBuffer_Len(t *testing.T) {
	r := newMpscRingBuffer()
	if r.Len() != 0 {
		t.Errorf("Len on empty ring = %d, want 0", r.Len())
	}
	for i := 0; i < ringSize; i++ {
		r.Push(LogEvent{Level: enum.LevelInfo})
	}
	if r.Len() != ringSize {
		t.Errorf("Len on full ring = %d, want %d", r.Len(), ringSize)
	}
}

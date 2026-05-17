//go:build testing

// Package integration: TS-25 — F21/F22/F23 backpressure via SlowHook.
//
// F21: config.ch is the channel through which log events flow.
// F22: channel buffer size is 10; up to 10 events can be in-flight.
// F23: PublishLog blocks the caller when the channel is full.
package integration

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// Test_Integration_ChannelBuffersAndBlocks is the TS-25 integration test.
// It verifies F21/F22/F23:
//   - F21: events travel through config.ch to ProcessLogEvent
//   - F22: channel buffers up to 10 events without blocking
//   - F23: sender blocks when the channel is full and ProcessLogEvent is stuck
//
// Approach:
//  1. Register a blocking preProc adapter; send 1 event to get ProcessLogEvent stuck.
//  2. Wait for preProc to enter (entered channel signals block is live).
//  3. Fill remaining channel capacity (bufSize-1 more events — channel is now full).
//  4. Assert next send blocks; Release hook; assert unblocks.
func Test_Integration_ChannelBuffersAndBlocks(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Build()

	slow := support.NewSlowHook("slow")
	entered := make(chan struct{})
	spy := &slowPreProcSignal{hook: slow, entered: entered}
	config.InitPreProcessors(spy)

	const bufSize = 10 // matches config.ch buffer (make(chan LogEvent, 10))
	payload := []byte(`{"level":"info","msg":"x"}`)

	// Trigger ProcessLogEvent to pick up and block on the first event.
	config.PublishLog(enum.LevelInfo, payload)

	// Wait until the preProc is actually blocked.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("preProc never entered; ProcessLogEvent may not be running")
	}

	// Channel now has 0 items (ProcessLogEvent consumed the first one and is
	// stuck). Fill the remaining capacity with bufSize items.
	for i := 0; i < bufSize; i++ {
		config.PublishLog(enum.LevelInfo, payload)
	}

	// Next send must block: channel is full and ProcessLogEvent is stuck.
	var unblocked atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		config.PublishLog(enum.LevelInfo, payload)
		unblocked.Store(true)
	}()

	time.Sleep(80 * time.Millisecond)
	if unblocked.Load() {
		t.Error("F23: sender returned immediately on full channel; must block")
	}

	slow.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked sender did not unblock after SlowHook.Release()")
	}
}

// Test_Integration_ChannelDrainsAsyncAfterRelease verifies that once
// SlowHook is released, ProcessLogEvent delivers all buffered events.
func Test_Integration_ChannelDrainsAsyncAfterRelease(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Build()

	fake := support.NewFakePostProcessor("drain-spy")
	spy := &fakePreProc{hook: fake}
	config.InitPreProcessors(spy)

	const n = 5
	payload := []byte(`{"level":"info","msg":"drain"}`)
	for i := 0; i < n; i++ {
		config.PublishLog(enum.LevelInfo, payload)
	}

	// Wait for all n events to be delivered asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fake.Got()) >= n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(fake.Got()); got < n {
		t.Errorf("delivered %d events; want %d", got, n)
	}
}

// --- minimal preProcessingObserverContract adapters ---
// These wrap support doubles to satisfy the unexported config interface.

// slowPreProcSignal wraps SlowHook and closes entered on the first PreProcess call.
type slowPreProcSignal struct {
	hook    *support.SlowHook
	entered chan struct{}
	once    sync.Once
}

func (s *slowPreProcSignal) Name() string { return s.hook.Name() }
func (s *slowPreProcSignal) PreProcess(_ enum.LogLevel, logMsg []byte) {
	s.once.Do(func() { close(s.entered) })
	s.hook.PublishLogMessage(logMsg)
}

type fakePreProc struct {
	mu   sync.Mutex
	hook *support.FakePostProcessor
}

func (f *fakePreProc) Name() string { return f.hook.Name() }
func (f *fakePreProc) PreProcess(_ enum.LogLevel, logMsg []byte) {
	f.hook.PublishLogMessage(logMsg)
}

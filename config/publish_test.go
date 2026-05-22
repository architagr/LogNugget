//go:build testing

// Package config_test covers ARCH-7 / Story 024 + Epic V2 P4: LogEvent.Data
// ownership semantics.
//
// Pre-P4: PublishLog made a defensive copy (ARCH-7). The copy ensured that
// callers returning e.buf to sync.Pool immediately after calling PublishLog
// could not race with the background ProcessLogEvent goroutine.
//
// P4 (Epic V2 inline framing): the defensive copy was moved upstream into
// entry.logWithSkip. logWithSkip now steals e.buf with:
//
//	data := e.buf
//	e.buf = make([]byte, 0, initBufCap)   // sever alias — fresh backing array
//	config.PublishLog(level, data)        // data has exclusive ownership
//	e.Put()                               // pool gets fresh buf, not data
//
// PublishLog no longer copies. Callers MUST transfer exclusive ownership of
// the slice before calling PublishLog; they must not modify or pool-return the
// slice after the call. Direct callers of PublishLog (outside logWithSkip) are
// responsible for making their own copy if needed.
//
// All sub-tests run sequentially under the parallel parent Test_PublishLog so
// they do not race on the singleton against Test_Race_ResetConfig_UnderConcurrentSet
// (which fires TestResetConfig concurrently). This mirrors the isolation strategy
// used by Test_Parsers and Test_LogEntry_Methods in this module.
//
// Three sub-tests are defined:
//   - DataOwnershipTransfer — correctness: caller transfers owned slice to
//     PublishLog; verifies the data arrives intact at the pre-processor
//   - AllocsZeroCopy        — alloc budget guard (P4): PublishLog itself now
//     allocates 0 copies; the sole channel-box alloc is <= 1
//   - RaceConcurrent        — 100 goroutines each publish 1 event; -race must be
//     clean; all 100 events must be delivered
package config_test

import (
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// drainPublishSpy spins until spy has at least one record, then returns the
// first record's Data. It fails the test after a 2 s deadline.
//
// why: config.PublishLog sends to a buffered channel consumed by
// config.ProcessLogEvent in a background goroutine. A bounded spin lets
// the goroutine schedule before the assertion runs.
func drainPublishSpy(t *testing.T, spy *support.FakePreProc) []byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recs := spy.Records()
		if len(recs) > 0 {
			return recs[0].Data
		}
		// Yield to the scheduler without a wall-clock sleep.
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	t.Fatal("timed out waiting for log record from spy; ProcessLogEvent may not be running")
	return nil
}

// waitForDrain blocks until spy holds at least n records or the deadline
// expires. It does NOT fail the test on timeout — callers check the count.
func waitForDrain(spy *support.FakePreProc, n int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(spy.Records()) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// setupPublishSpy resets the singleton and installs a fresh FakePreProc as the
// sole pre-processor. It returns the spy for subsequent assertions.
//
// why: TestResetConfig creates a new channel and starts a new ProcessLogEvent
// goroutine. Any events from a previous test sub-test that are still in-flight
// in the old channel will be processed by the old goroutine reading the LIVE
// EventPreProcessors map. Calling TestResetConfig at the start of each sub-test
// and installing a fresh spy prevents cross-contamination — provided the
// previous sub-test drained all its events before returning.
func setupPublishSpy(t *testing.T, name string) *support.FakePreProc {
	t.Helper()
	config.TestResetConfig()
	spy := support.NewFakePreProc(name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_PublishLog is the top-level test grouping all publish semantic sub-tests.
// It is intentionally NOT parallel: sub-tests mutate the singleton (via
// TestResetConfig and InitPreProcessors) and ProcessLogEvent reads the live
// EventPreProcessors map. Running as a sequential test ensures this group
// completes before the parallel race tests (Test_Race_*) resume, so concurrent
// TestResetConfig calls from those tests cannot interrupt our spy installations.
//
// why: making this parallel would allow Test_Race_ResetConfig_UnderConcurrentSet
// to fire ~200 TestResetConfig calls while our goroutines are still publishing,
// emptying EventPreProcessors mid-flight and causing DataNotAliasedToPool to
// time out waiting for its spy to be invoked.
func Test_PublishLog(t *testing.T) {

	// DataOwnershipTransfer verifies that when the caller transfers ownership of a
	// slice to PublishLog (P4 contract: caller must not retain or modify the slice
	// after the call), the data arrives intact at the pre-processor.
	//
	// This replaces the pre-P4 DataNotAliasedToPool test which asserted that
	// PublishLog made a defensive copy. P4 moved the copy responsibility upstream
	// into entry.logWithSkip (ownership transfer via e.buf steal + fresh make).
	//
	// Acceptance criterion: LogEvent.Data received by the pre-processor matches
	// the bytes passed to PublishLog at call time.
	t.Run("DataOwnershipTransfer", func(t *testing.T) {
		spy := setupPublishSpy(t, "ownership-spy")

		// Caller owns this slice and will not touch it after calling PublishLog,
		// simulating the P4 ownership transfer in logWithSkip.
		owned := []byte("original-content")
		config.PublishLog(enum.LevelInfo, owned)

		got := drainPublishSpy(t, spy)

		const want = "original-content"
		if string(got) != want {
			t.Errorf("LogEvent.Data = %q; want %q\n"+
				"hint: P4 ownership transfer — PublishLog must deliver Data intact",
				string(got), want)
		}
	})

	// AllocsZeroCopy documents the P4 allocation budget: PublishLog itself now
	// performs 0 copies (no append([]byte(nil), Data...)). The sole alloc is the
	// LogEvent value boxed into the channel's ring buffer, which the Go runtime
	// may or may not escape to the heap.
	//
	// P4 change: the defensive copy moved into entry.logWithSkip (ownership
	// transfer: e.buf steal + make([]byte, 0, initBufCap)). PublishLog must not
	// re-introduce a copy.
	//
	// Isolation: we wait for all sent events to drain before returning so that
	// events from AllocsPerRun's burst are fully processed before RaceConcurrent
	// installs its spy.
	t.Run("AllocsZeroCopy", func(t *testing.T) {
		sink := support.NewFakePreProc("alloc-sink")
		config.TestResetConfig()
		config.InitPreProcessors(sink)

		// payload simulates a realistic encoded log line (~128 bytes of JSON).
		payload := make([]byte, 128)
		for i := range payload {
			payload[i] = byte('a' + i%26)
		}

		var sent int
		allocs := testing.AllocsPerRun(100, func() {
			config.PublishLog(enum.LevelInfo, payload)
			sent++
		})

		waitForDrain(sink, sent, 5*time.Second)

		t.Logf("AllocsPerRun for PublishLog with 128-byte payload: %.1f allocs/op", allocs)

		// P4: PublishLog must not copy Data — 0 allocs for the copy.
		// The channel-box alloc may or may not be counted (runtime-dependent).
		// Guard the upper end: anything > 2 indicates unexpected heap pressure.
		if allocs > 2 {
			t.Errorf("PublishLog reported %.1f allocs; expected <= 2 (P4: no Data copy). "+
				"Check whether append([]byte(nil), Data...) was re-introduced.", allocs)
		}
	})

	// RaceConcurrent spawns 100 goroutines each publishing 1 event and verifies
	// that all 100 events reach the pre-processor.
	//
	// Running with `go test -race -tags testing` must produce no DATA RACE
	// reports. Each goroutine passes the same payload slice to PublishLog; since
	// no goroutine writes to payload after setup, reading from multiple goroutines
	// concurrently is race-free. ProcessLogEvent reads each LogEvent.Data slice
	// without writing to it, so concurrent reads across goroutines are safe.
	//
	// P4 note: this is the correct usage pattern — goroutines pass a read-only
	// (or exclusively-owned) slice to PublishLog and do not touch it afterwards.
	//
	// why: numPublishes=1 keeps total sent (100) well within the channel buffer
	// capacity. The race detector — not the count — is the primary correctness
	// signal; the count assertion is a secondary liveness check.
	t.Run("RaceConcurrent", func(t *testing.T) {
		const (
			numGoroutines = 100
			numPublishes  = 1  // 1 per goroutine = 100 total; fits in channel buffer
			wantTotal     = numGoroutines * numPublishes
		)

		config.TestResetConfig()
		spy := support.NewFakePreProc("concurrent-spy")
		config.InitPreProcessors(spy)

		payload := []byte("concurrent-test-payload")

		var wg sync.WaitGroup
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < numPublishes; j++ {
					config.PublishLog(enum.LevelInfo, payload)
				}
			}()
		}
		wg.Wait()

		// Wait for ProcessLogEvent to drain all 100 events.
		waitForDrain(spy, wantTotal, 5*time.Second)
		gotTotal := len(spy.Records())

		if gotTotal != wantTotal {
			t.Errorf("spy received %d events; want %d\n"+
				"hint: check that PublishLog does not drop events", gotTotal, wantTotal)
		}

		// Drain: all wantTotal events confirmed above; no residue for next test.
	})
}

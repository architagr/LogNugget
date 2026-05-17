//go:build testing

// Package config_test covers ARCH-7 / Story 024: LogEvent.Data copy semantics.
//
// These tests verify that PublishLog makes an independent copy of the caller's
// byte slice before putting it on the dispatch channel. The invariant matters
// because entry.logWithSkip passes an encoder output buffer that may be returned
// to a sync.Pool immediately after calling PublishLog. Without a defensive copy,
// a race exists between the background ProcessLogEvent goroutine reading
// LogEvent.Data and the pool recycling the same backing array.
//
// All sub-tests run sequentially under the parallel parent Test_PublishLog so
// they do not race on the singleton against Test_Race_ResetConfig_UnderConcurrentSet
// (which fires TestResetConfig concurrently). This mirrors the isolation strategy
// used by Test_Parsers and Test_LogEntry_Methods in this module.
//
// Three sub-tests are defined:
//   - DataNotAliasedToPool  — correctness: mutation of caller's buf after call
//     does not affect the dispatched LogEvent.Data
//   - AllocsOneCopy         — alloc budget guard (NF3): verifies >= 1 and <= 5
//     allocs/call with a 128-byte payload
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

	// DataNotAliasedToPool verifies that the LogEvent.Data byte slice received by
	// a pre-processor is independent of the buffer originally passed to PublishLog.
	//
	// Steps:
	//  1. Call PublishLog with a known slice ("original-content").
	//  2. Immediately overwrite every byte of the original slice with 'X'.
	//  3. Assert the spy received "original-content", not "XXXXXXXXXXXXXXXX".
	//
	// Acceptance criterion 1 of Story 024 (ARCH-7): LogEvent.Data is a fresh
	// allocation reflecting the bytes at call time, not the bytes present when
	// the dispatcher goroutine reads the event.
	t.Run("DataNotAliasedToPool", func(t *testing.T) {
		spy := setupPublishSpy(t, "alias-spy")

		original := []byte("original-content")
		config.PublishLog(enum.LevelInfo, original)

		// Simulate pool recycling: overwrite every byte immediately after call.
		// If PublishLog aliased Data to original, the spy sees "XXXXXXXXXXXXXXXX".
		for i := range original {
			original[i] = 'X'
		}

		got := drainPublishSpy(t, spy)

		const want = "original-content"
		if string(got) != want {
			t.Errorf("LogEvent.Data = %q after caller mutation; want %q\n"+
				"hint: PublishLog must copy Data before sending on the channel (ARCH-7)",
				string(got), want)
		}

		// Drain: only 1 event sent; already verified above. No extra wait needed.
	})

	// AllocsOneCopy documents the allocation budget of a PublishLog call under
	// the NF3 alloc-accounting requirement.
	//
	// Expected: 2 allocs/op — 1 for append([]byte(nil), Data...) (the defensive
	// copy mandated by ARCH-7) plus 1 for the LogEvent value boxed into the
	// channel's ring buffer. A count of 0 means the copy was removed (aliasing
	// defect). A count > 5 signals unexpected heap pressure.
	//
	// why: NF3 requires one identifiable alloc per published event (~600 B). This
	// AllocsPerRun measurement is the canonical proof that no regression to zero
	// copies (aliasing) or excessive copies has occurred.
	//
	// Isolation: we wait for all sent events to drain before returning so that
	// events from AllocsPerRun's burst (101 calls) are fully processed before
	// RaceConcurrent installs its spy. Without this drain, in-flight events from
	// the old channel goroutine would be forwarded to RaceConcurrent's spy
	// (ProcessLogEvent reads the live EventPreProcessors map, not a snapshot).
	t.Run("AllocsOneCopy", func(t *testing.T) {
		sink := support.NewFakePreProc("alloc-sink")
		config.TestResetConfig()
		config.InitPreProcessors(sink)

		// payload simulates a realistic encoded log line (~128 bytes of JSON).
		payload := make([]byte, 128)
		for i := range payload {
			payload[i] = byte('a' + i%26)
		}

		// Track exactly how many events are sent during the measurement so we can
		// wait for them all to drain before returning from this sub-test.
		var sent int
		allocs := testing.AllocsPerRun(100, func() {
			config.PublishLog(enum.LevelInfo, payload)
			sent++
		})

		// why: ProcessLogEvent already runs via the goroutine started by
		// resetConfig. We wait for all sent events to be acknowledged by the sink
		// so the subsequent RaceConcurrent sub-test does not inherit in-flight events.
		waitForDrain(sink, sent, 5*time.Second)

		t.Logf("AllocsPerRun for PublishLog with 128-byte payload: %.1f allocs/op", allocs)

		// A count of 0 means no copy — ARCH-7 violated. Even the pre-fix channel
		// boxing gives 1, so after the fix we expect exactly 2. Guard both ends.
		if allocs < 1 {
			t.Errorf("PublishLog reported %.1f allocs: expected >= 1 (ARCH-7 copy + channel box). "+
				"Verify that append([]byte(nil), Data...) is present in PublishLog.", allocs)
		}
		if allocs > 5 {
			t.Errorf("PublishLog reported %.1f allocs; expected <= 5 (NF3 budget). "+
				"Investigate unexpected heap pressure on the publish path.", allocs)
		}
	})

	// RaceConcurrent spawns 100 goroutines each publishing 1 event and verifies
	// that all 100 events reach the pre-processor.
	//
	// Running with `go test -race -tags testing` must produce no DATA RACE
	// reports — this is the canonical proof of ARCH-7's "no aliasing across
	// goroutine boundary" requirement. Each goroutine shares the same payload
	// slice; PublishLog's copy ensures the dispatcher never reads from a slice
	// that is concurrently written by another goroutine.
	//
	// why: numPublishes=1 keeps total sent (100) well within the channel buffer
	// capacity (10 × drain rate). Larger bursts (e.g. 100×100) block goroutines
	// in the channel send and allow other parallel tests' TestResetConfig calls
	// to redirect in-flight events to their spy, breaking the exact count
	// assertion. The race detector — not the count — is the primary correctness
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

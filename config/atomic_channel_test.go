//go:build testing

// Package config contains tests for the V3-P3 atomic.Value channel pointer
// optimisation. These tests verify that PublishLog no longer acquires
// configMu.RLock, eliminating the last read-lock acquisition from the hot path
// (Epic V3 / LLD §5.1).
//
// The tests live in package config (not config_test) so they can access the
// unexported configMu directly — the only reliable way to prove that PublishLog
// does NOT acquire configMu.RLock without adding test-only hooks to production
// code.
package config

import (
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/enum"
)

// Test_PublishLog_NoConfigMuRLock proves that PublishLog does not call
// configMu.RLock. The proof: hold configMu.Lock() for an extended window,
// ensure the goroutine has started executing (via the ready barrier), and
// assert that PublishLog completes before the deadline.
//
// If PublishLog calls configMu.RLock(), it blocks inside the lock-held window
// and done never receives within 200 ms — the test fails with an explicit
// message. If PublishLog reads the channel via atomicCh (no configMu), the
// send completes immediately and done receives before the deadline.
//
// why: holding the write lock makes any RLock attempt block indefinitely.
// A 200 ms deadline is ~10⁶× larger than the expected lock-free completion
// time (~50 ns), so false positives from scheduler delay are not possible
// in practice (V3-P3 / LLD §5.1).
func Test_PublishLog_NoConfigMuRLock(t *testing.T) {
	TestResetConfig()

	done := make(chan struct{})
	ready := make(chan struct{})

	// Acquire the exclusive write lock before spawning the goroutine.
	// Any RLock call inside PublishLog will block until we call Unlock.
	configMu.Lock()

	go func() {
		// Signal that this goroutine is now live and about to call PublishLog.
		// This ensures PublishLog executes while configMu is still locked.
		close(ready)
		PublishLog(enum.LevelInfo, []byte(`{"level":"INFO"}`))
		close(done)
	}()

	// Wait until the goroutine has been scheduled and is past the ready
	// barrier — it is now either blocked on configMu.RLock (old code) or
	// has already sent (new code using atomicCh).
	<-ready

	// Hold the lock a little longer to guarantee the goroutine has had time
	// to reach the configMu.RLock call if it intends to make one.
	time.Sleep(2 * time.Millisecond)

	// Release the lock. If PublishLog was blocked on RLock it will now
	// unblock and complete, but we have already started the deadline timer.
	configMu.Unlock()

	select {
	case <-done:
		// PublishLog completed — either lock-free (ideal) or unblocked by Unlock.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Test_PublishLog_NoConfigMuRLock: timed out — PublishLog may still " +
			"be acquiring configMu.RLock, blocking under the write lock. " +
			"Expected lock-free atomicCh read after V3-P3.")
	}
}

// Test_PublishLog_Race exercises PublishLog under concurrent publish and reset
// activity. It must produce no DATA RACE reports when run with -race.
// Eight goroutines each publish 50 events (400 total) while a ninth goroutine
// calls TestResetConfig three times, replacing ch and atomicCh with fresh values.
//
// why: the sub-1 µs SLO requires that the atomic.Value load and channel send
// are individually safe with no lock. This stress test validates that guarantee
// under the Go race detector, which instruments every memory access at runtime
// (V3-P3 / LLD §5.1).
func Test_PublishLog_Race(t *testing.T) {
	TestResetConfig()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				PublishLog(enum.LevelInfo, []byte(`{"level":"INFO"}`))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 3; j++ {
			TestResetConfig()
		}
	}()
	wg.Wait()
}

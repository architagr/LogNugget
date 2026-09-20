package pipelineStage

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// safeCountWriter is a thread-safe io.Writer that atomically counts every
// Write call. It is distinct from mockWriter (which uses a mutex) to
// demonstrate that the concurrent-publish path needs no external
// serialisation — each Write call is an independent atomic increment.
type safeCountWriter struct {
	calls   atomic.Int64
	records atomic.Int64
}

// Write counts both the call and the records inside it, then discards p.
//
// why records: one Write now carries a whole flush batch, so counting calls
// no longer counts messages. Records are newline-delimited, as the encoder
// produces them.
func (w *safeCountWriter) Write(p []byte) (int, error) {
	w.calls.Add(1)
	w.records.Add(int64(bytes.Count(p, []byte{'\n'})))
	return len(p), nil
}

// Count returns the number of records written so far.
func (w *safeCountWriter) Count() int {
	return int(w.records.Load())
}

// Writes returns the number of Write calls — one per flush batch.
func (w *safeCountWriter) Writes() int {
	return int(w.calls.Load())
}

// Test_Race_PostProcessorPublish exercises concurrent PublishLogMessage calls
// and verifies that every message is eventually delivered after Stop().
//
// 100 goroutines × 1 000 publishes = 100 000 messages. Each message
// must arrive exactly once, batched into far fewer Write calls. maxBucketSize is set high enough (110 000) so
// that no capacity-triggered flush fires mid-test; all messages drain on
// Stop(), which blocks until the final flush goroutine completes.
//
// NF9: the -race detector must report no data races.
func Test_Race_PostProcessorPublish(t *testing.T) {
	const goroutines = 100
	const msgsPerGoroutine = 1_000
	const totalMessages = goroutines * msgsPerGoroutine
	// why records, not Write calls: a flush writes its whole batch in one
	// Write, so the assertion counts newline-delimited records instead.
	const expectedRecords = totalMessages

	out := &safeCountWriter{}
	// why: 10-minute rate prevents ticker flushes from firing during the
	// test so the only drain path is the Stop() call, making the assertion
	// deterministic (no race between ticker and Stop).
	proc := NewUnsetLogEventPostProcessor(10*time.Minute, 110_000, out)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < msgsPerGoroutine; i++ {
				proc.PublishLogMessage([]byte("payload\n"))
			}
		}()
	}
	wg.Wait()

	// Stop blocks until all enqueued messages are written. After it
	// returns, out.Count() must equal expectedWrites with no further
	// goroutines touching the processor.
	proc.Stop()

	assert.Equal(t, expectedRecords, out.Count(),
		"every published message must reach the writer exactly once")
	assert.Less(t, out.Writes(), expectedRecords,
		"batching must produce fewer Write calls than records")
}

// Test_Race_StopWhileLogging verifies that calling Stop() concurrently with
// active publishers neither panics nor deadlocks.
//
// 50 goroutines publish continuously; after a 10 ms head start one
// goroutine calls Stop(). The test asserts:
//   - Stop() returns (no deadlock).
//   - No panic is raised.
//   - The drain count is non-negative (best-effort; exact value is
//     non-deterministic because Stop races with in-flight appends).
//
// NF9: the -race detector must report no data races on this path.
func Test_Race_StopWhileLogging(t *testing.T) {
	const publishers = 50

	out := &safeCountWriter{}
	// why: small maxBucketSize (32) so that capacity-triggered flushes
	// fire during publishing, exercising the concurrent flush+stop path.
	proc := NewUnsetLogEventPostProcessor(10*time.Minute, 32, out)

	var (
		wg      sync.WaitGroup
		stopped atomic.Bool
	)

	for g := 0; g < publishers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stopped.Load() {
				proc.PublishLogMessage([]byte("msg"))
			}
		}()
	}

	// Give publishers a head start, then stop the processor.
	time.Sleep(10 * time.Millisecond)
	stopped.Store(true) // signal goroutines to stop publishing
	// Stop blocks until the drain is complete; if it hangs, the test
	// fails via the -timeout flag.
	proc.Stop()
	wg.Wait()

	// Any non-negative write count is acceptable: the test goal is
	// race-freedom and panic-freedom, not a specific count.
	assert.GreaterOrEqual(t, out.Count(), 0,
		"drain count must be non-negative after concurrent Stop")
}

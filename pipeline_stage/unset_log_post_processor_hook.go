package pipelineStage

import (
	"io"
	"sync"
	"time"
)

// unsetLogEventPostProcessor batches log messages and flushes them
// either periodically or when the bucket reaches capacity.
type unsetLogEventPostProcessor struct {
	mu            sync.Mutex
	activeBucket  [][]byte
	maxBucketSize int
	rate          time.Duration
	ticker        *time.Ticker
	output        io.Writer
	stopCh        chan struct{}
	// doneCh is closed by activeBucketWatcher after the final drain completes.
	// Stop() blocks on doneCh so callers get a synchronous shutdown guarantee
	// without busy-waiting. D-11 / SC5.
	doneCh   chan struct{}
	stopOnce sync.Once
	// flushWg tracks in-flight async flush goroutines started by flushLogMessages
	// so that the stop-path drain can wait for them before closing doneCh.
	flushWg sync.WaitGroup
	// stopping is set to true inside mu.Lock() when the stop path begins.
	// PublishLogMessage and flushLogMessages check this flag (under mu) before
	// calling flushWg.Add(1) to ensure no Add races with the stop-path
	// flushWg.Wait() call after the counter reaches zero.
	stopping bool
}

// NewUnsetLogEventPostProcessor creates a new post processor.
func NewUnsetLogEventPostProcessor(rate time.Duration, maxBufferSize int, output io.Writer) *unsetLogEventPostProcessor {
	obj := &unsetLogEventPostProcessor{
		activeBucket:  make([][]byte, 0, maxBufferSize),
		maxBucketSize: maxBufferSize,
		rate:          rate,
		output:        output,
		ticker:        time.NewTicker(rate),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	go obj.activeBucketWatcher()
	return obj
}

// activeBucketWatcher periodically flushes messages and handles shutdown.
func (h *unsetLogEventPostProcessor) activeBucketWatcher() {
	for {
		select {
		case <-h.ticker.C:
			h.flushLogMessages()
		case <-h.stopCh:
			h.ticker.Stop()
			// Drain the active bucket synchronously on the stop path.
			// why: caller of Stop() expects all enqueued messages to be
			// written before Stop returns. We take the lock once, swap the
			// bucket, set stopping=true (so no new flushWg.Add(1) calls can
			// race with the flushWg.Wait() below), then wait for any
			// in-flight async flushes and write the remaining bucket
			// directly without spawning a goroutine. No time.Sleep — D-11.
			h.mu.Lock()
			h.stopping = true
			remaining := h.activeBucket
			h.activeBucket = make([][]byte, 0, h.maxBucketSize)
			h.mu.Unlock()

			// Wait for any async flushes that were in flight before stopCh fired.
			// stopping=true ensures no new Add(1) calls can race with Wait().
			h.flushWg.Wait()
			// Write the final batch synchronously.
			h.printMessage(remaining)
			close(h.doneCh)
			return
		}
	}
}

// resetBucket clears the active bucket.
func (h *unsetLogEventPostProcessor) resetBucket() {
	h.activeBucket = make([][]byte, 0, h.maxBucketSize)
}

// flushLogMessages safely extracts the active bucket and writes it
// asynchronously. Add(1) is called inside the lock so that the stop-path
// drain (which also runs under the same lock) cannot see a zero flushWg
// count after a swap has been committed but before the goroutine is registered.
// If stopping is true, no new goroutine is launched — the stop path owns the
// drain at that point.
func (h *unsetLogEventPostProcessor) flushLogMessages() {
	h.mu.Lock()
	if len(h.activeBucket) == 0 || h.stopping {
		h.mu.Unlock()
		return
	}
	backupBucket := h.activeBucket
	h.resetBucket()
	h.flushWg.Add(1)
	h.mu.Unlock()

	go func() {
		defer h.flushWg.Done()
		h.printMessage(backupBucket)
	}()
}

// printMessage writes buffered messages to the output, one per line.
// Write errors are intentionally ignored: log delivery is best-effort and
// the caller has already released the bucket; there is nothing to retry.
func (h *unsetLogEventPostProcessor) printMessage(data [][]byte) {
	for _, d := range data {
		_, _ = h.output.Write(d)
		_, _ = h.output.Write([]byte{'\n'})
	}
}

// PublishLogMessage appends entry to the active bucket and, when the bucket
// reaches capacity, atomically swaps it out under the lock and spawns a
// flush goroutine after releasing the lock.
//
// The swap-under-lock pattern eliminates the Unlock+flush+Lock sequence that
// was the root cause of D-12: a deferred Unlock paired with an explicit
// Unlock inside the same method causes a double-unlock panic under concurrent
// load.  By capturing the full slice reference before resetting the field and
// only calling printMessage after the lock is released, each slice is owned
// by exactly one goroutine and is never written to again.
func (h *unsetLogEventPostProcessor) PublishLogMessage(entry []byte) {
	var toFlush [][]byte

	h.mu.Lock()
	h.activeBucket = append(h.activeBucket, entry)
	if len(h.activeBucket) >= h.maxBucketSize && !h.stopping {
		// why: capture slice and Add(1) inside the lock so the stop drain
		// (which also holds mu before calling flushWg.Wait) cannot observe a
		// zero count after the swap has been committed. Guard with !h.stopping
		// to prevent Add(1) racing with the stop-path flushWg.Wait().
		toFlush = h.activeBucket
		h.activeBucket = make([][]byte, 0, h.maxBucketSize)
		h.flushWg.Add(1)
	}
	h.mu.Unlock()

	if toFlush != nil {
		go func() {
			defer h.flushWg.Done()
			h.printMessage(toFlush)
		}()
	}
}

// Name returns processor name.
func (h *unsetLogEventPostProcessor) Name() string {
	return "unsetLogEventPostProcessor"
}

// Stop signals the processor to shut down and blocks until the active bucket
// is fully drained and all writes complete. Idempotent: a second call is a
// no-op and returns immediately. D-11 / SC5.
func (h *unsetLogEventPostProcessor) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopCh)
	})
	<-h.doneCh
}

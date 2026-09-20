package pipelineStage

import (
	"io"
	"sync"
	"time"
)

const (
	// initialArenaCap is the starting capacity of a pending-line arena. 32 KB
	// holds ~250 typical structured log lines before the first regrow.
	initialArenaCap = 32 * 1024
	// maxArenaRetainCap bounds the arenas kept in arenaPool. An arena grown
	// past this by an unusually large batch is dropped instead of recycled so
	// a single burst cannot pin megabytes for the process lifetime.
	maxArenaRetainCap = 1 << 20 // 1 MiB
)

// arenaPool recycles the byte arenas that hold copies of pending log lines.
var arenaPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, initialArenaCap)
		return &b
	},
}

func getArena() []byte {
	return *arenaPool.Get().(*[]byte)
}

func putArena(b []byte) {
	if cap(b) > maxArenaRetainCap {
		return
	}
	b = b[:0]
	arenaPool.Put(&b)
}

// unsetLogEventPostProcessor batches log messages and flushes them
// either periodically or when the bucket reaches capacity.
//
// Pending lines are stored as bytes copied into arena, with ends[i] holding
// the exclusive end offset of line i. why: the []byte handed to
// PublishLogMessage belongs to the dispatch buffer pool and is recycled as
// soon as the call returns — retaining the slice (as this type used to) let a
// later log call overwrite a line that had not been written yet, producing
// truncated and duplicated output under concurrent load. Copying into an
// arena keeps the batching behaviour without holding a borrowed buffer, and
// the arena itself is pooled so the copy costs no allocation per message.
type unsetLogEventPostProcessor struct {
	mu            sync.Mutex
	arena         []byte
	ends          []int
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
		arena:         getArena(),
		ends:          make([]int, 0, maxBufferSize),
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
			arena, ends := h.swapBucket()
			h.mu.Unlock()

			// Wait for any async flushes that were in flight before stopCh fired.
			// stopping=true ensures no new Add(1) calls can race with Wait().
			h.flushWg.Wait()
			// Write the final batch synchronously.
			h.printMessage(arena, ends)
			close(h.doneCh)
			return
		}
	}
}

// swapBucket hands the pending arena and offsets to the caller and installs
// fresh ones. Callers must hold h.mu; the returned arena is owned by exactly
// one goroutine from that point and must be passed to printMessage.
func (h *unsetLogEventPostProcessor) swapBucket() (arena []byte, ends []int) {
	arena, ends = h.arena, h.ends
	h.arena = getArena()
	h.ends = make([]int, 0, h.maxBucketSize)
	return arena, ends
}

// flushLogMessages safely extracts the active bucket and writes it
// asynchronously. Add(1) is called inside the lock so that the stop-path
// drain (which also runs under the same lock) cannot see a zero flushWg
// count after a swap has been committed but before the goroutine is registered.
// If stopping is true, no new goroutine is launched — the stop path owns the
// drain at that point.
func (h *unsetLogEventPostProcessor) flushLogMessages() {
	h.mu.Lock()
	if len(h.ends) == 0 || h.stopping {
		h.mu.Unlock()
		return
	}
	arena, ends := h.swapBucket()
	h.flushWg.Add(1)
	h.mu.Unlock()

	go func() {
		defer h.flushWg.Done()
		h.printMessage(arena, ends)
	}()
}

// printMessage writes the buffered records held in arena to the output, one
// Write per record, then recycles the arena.
//
// Each record already carries its own terminator: the encoder's CloseBytes
// ends every line with "\n" (ARCH-14). why: this method used to write a
// second newline after each record, so every emitted stream contained a blank
// line between records — valid-but-noisy NDJSON that some collectors ingest
// as empty entries.
//
// Write errors are intentionally ignored: log delivery is best-effort and
// the caller has already released the bucket; there is nothing to retry.
func (h *unsetLogEventPostProcessor) printMessage(arena []byte, ends []int) {
	start := 0
	for _, end := range ends {
		_, _ = h.output.Write(arena[start:end])
		start = end
	}
	putArena(arena)
}

// PublishLogMessage copies entry into the pending arena and, when the bucket
// reaches capacity, atomically swaps the arena out under the lock and spawns a
// flush goroutine after releasing the lock.
//
// entry is borrowed: it is valid only for the duration of this call (the
// dispatcher returns it to the buffer pool immediately afterwards), which is
// why the bytes are copied rather than referenced.
//
// The swap-under-lock pattern eliminates the Unlock+flush+Lock sequence that
// was the root cause of D-12: a deferred Unlock paired with an explicit
// Unlock inside the same method causes a double-unlock panic under concurrent
// load. By capturing the arena before installing a fresh one and only calling
// printMessage after the lock is released, each arena is owned by exactly one
// goroutine and is never written to again.
func (h *unsetLogEventPostProcessor) PublishLogMessage(entry []byte) {
	var (
		toFlush []byte
		ends    []int
	)

	h.mu.Lock()
	h.arena = append(h.arena, entry...)
	h.ends = append(h.ends, len(h.arena))
	if len(h.ends) >= h.maxBucketSize && !h.stopping {
		// why: capture the arena and Add(1) inside the lock so the stop drain
		// (which also holds mu before calling flushWg.Wait) cannot observe a
		// zero count after the swap has been committed. Guard with !h.stopping
		// to prevent Add(1) racing with the stop-path flushWg.Wait().
		toFlush, ends = h.swapBucket()
		h.flushWg.Add(1)
	}
	h.mu.Unlock()

	if ends != nil {
		go func() {
			defer h.flushWg.Done()
			h.printMessage(toFlush, ends)
		}()
	}
}

// Name returns processor name.
func (h *unsetLogEventPostProcessor) Name() string {
	return "unsetLogEventPostProcessor"
}

// SetOutput redirects subsequent writes to output. Records already flushed are
// unaffected; records still pending in the bucket are written to the new
// output when the bucket next flushes. Passing nil is a no-op.
//
// why: config.SetOutput is documented as "the output writer for the default
// collector", and the default collector is this type. Without a runtime
// setter that call could only ever change a struct field nobody read.
func (h *unsetLogEventPostProcessor) SetOutput(output io.Writer) {
	if output == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.output = output
}

// SetRate changes the interval at which buffered records are flushed. Values
// ≤ 0 are ignored. The change takes effect from the next tick.
func (h *unsetLogEventPostProcessor) SetRate(rate time.Duration) {
	if rate <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rate = rate
	h.ticker.Reset(rate)
}

// SetMaxBucketSize changes how many records accumulate before a size-triggered
// flush. Values ≤ 0 are ignored.
//
// Lowering the threshold below the number of records already pending does not
// flush them immediately; they go out on the next publish or tick.
func (h *unsetLogEventPostProcessor) SetMaxBucketSize(size int) {
	if size <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.maxBucketSize = size
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

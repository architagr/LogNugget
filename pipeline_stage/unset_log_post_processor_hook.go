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
	}
	go obj.activeBucketWatcher()
	return obj
}

// activeBucketWatcher periodically flushes messages.
func (h *unsetLogEventPostProcessor) activeBucketWatcher() {
	for {
		select {
		case <-h.ticker.C:
			h.flushLogMessages()
		case <-h.stopCh:
			h.flushLogMessages()
			for len(h.activeBucket) > 0 {
				time.Sleep(h.rate)
			}
			h.ticker.Stop()
			return
		}
	}
}

// resetBucket clears the active bucket.
func (h *unsetLogEventPostProcessor) resetBucket() {
	h.activeBucket = make([][]byte, 0, h.maxBucketSize)
}

// flushLogMessages safely extracts and processes messages.
func (h *unsetLogEventPostProcessor) flushLogMessages() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.activeBucket) == 0 {
		return
	}

	backupBucket := h.activeBucket
	h.resetBucket()

	// process asynchronously
	go h.printMessage(backupBucket)
}

// printMessage writes buffered messages to the output.
func (h *unsetLogEventPostProcessor) printMessage(data [][]byte) {
	for _, d := range data {
		h.output.Write(d)
		h.output.Write([]byte{'\n'})
	}
}

// PublishLogMessage appends a message and flushes if capacity reached.
func (h *unsetLogEventPostProcessor) PublishLogMessage(entry []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.activeBucket) >= h.maxBucketSize {
		// extract bucket and reset before writing (avoid blocking)
		h.mu.Unlock()
		h.flushLogMessages()
		h.mu.Lock()
	}

	h.activeBucket = append(h.activeBucket, entry)
}

// Name returns processor name.
func (h *unsetLogEventPostProcessor) Name() string {
	return "unsetLogEventPostProcessor"
}

// Stop safely shuts down the processor.
func (h *unsetLogEventPostProcessor) Stop() {
	close(h.stopCh)
}

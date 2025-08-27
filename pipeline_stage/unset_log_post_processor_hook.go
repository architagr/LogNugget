package pipelineStage

import (
	"io"
	"sync"
	"time"
)

type unsetLogEventPostProsessor struct {
	mu            sync.Mutex
	activeBucket  [][]byte
	maxBucketSize int
	rate          time.Duration
	ticker        *time.Ticker
	output        io.Writer
}

func NewUnsetLogEventPostProcessor(rate time.Duration, maxBufferSize int, output io.Writer) *unsetLogEventPostProsessor {
	obj := &unsetLogEventPostProsessor{
		activeBucket:  make([][]byte, 0, maxBufferSize),
		maxBucketSize: maxBufferSize,
		rate:          rate,
		output:        output,
		ticker:        time.NewTicker(rate),
	}
	go obj.activeBucketWatcher()
	return obj
}

func (h *unsetLogEventPostProsessor) activeBucketWatcher() {
	for range h.ticker.C {
		h.flushLogMessages()
	}
}

func (h *unsetLogEventPostProsessor) resetBucket() {
	h.activeBucket = make([][]byte, 0, h.maxBucketSize)
}

func (h *unsetLogEventPostProsessor) flushLogMessages() {
	if len(h.activeBucket) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	defer h.resetBucket()

	// backupBucket := make([][]byte, 0, len(h.activeBucket))
	// copy(backupBucket, h.activeBucket)
	backupBucket := h.activeBucket
	go h.printMessage(backupBucket)
}

func (h *unsetLogEventPostProsessor) printMessage(data [][]byte) {
	for _, d := range data {
		h.output.Write(d)
	}
}

func (h *unsetLogEventPostProsessor) PublishLogMessage(entry []byte) {
	if len(h.activeBucket) >= h.maxBucketSize {
		h.ticker.Stop()
		h.flushLogMessages()
		h.ticker.Reset(h.rate)
	}

	h.activeBucket = append(h.activeBucket, entry)
}

func (h *unsetLogEventPostProsessor) Name() string {
	return "unsetLogEventPostProsessor"
}

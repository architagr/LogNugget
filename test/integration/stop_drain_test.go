//go:build testing

// Package integration: SC5 — Stop() drains all queued messages before returning.
package integration

import (
	"sync"
	"testing"
	"time"

	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/stretchr/testify/assert"
)

// countingWriter records the number of Write calls, thread-safe.
type countingWriter struct {
	mu    sync.Mutex
	count int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.count++
	w.mu.Unlock()
	return len(p), nil
}

func (w *countingWriter) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.count
}

// Test_Integration_StopDrainsAllQueued is the SC5 integration test.
// It enqueues 2×maxBucketSize messages via PublishLogMessage directly (N-7
// scoping: not through the full entry.Info channel pipeline) and asserts that
// Stop drains every message before returning.
//
// why: 2×maxBucketSize exercises the path where a mid-publish flush is
// triggered (bucket hits capacity) and a second batch waits in the active
// bucket when Stop fires.
func Test_Integration_StopDrainsAllQueued(t *testing.T) {
	maxBucketSize := 100
	total := 2 * maxBucketSize

	out := &countingWriter{}
	// Slow ticker so only capacity-triggered and stop-triggered flushes run.
	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, maxBucketSize, out)

	for i := 0; i < total; i++ {
		proc.PublishLogMessage([]byte("payload"))
	}

	proc.Stop()

	// Each message produces 2 Write calls (data bytes + "\n").
	assert.Equal(t, total*2, out.Count(), "Stop must drain all %d enqueued messages", total)
}

// Test_Integration_StopIdempotent verifies that a second Stop call after
// the first one has completed returns immediately and does not panic.
func Test_Integration_StopIdempotent(t *testing.T) {
	out := &countingWriter{}
	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, 10, out)
	proc.PublishLogMessage([]byte("x"))

	proc.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		proc.Stop()
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second Stop did not return within 500 ms; must be idempotent")
	}
}

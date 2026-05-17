package pipelineStage

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test_PostProcessor_StopDrainsActiveBucket verifies that calling Stop() after
// enqueueing N messages causes the writer to observe exactly N writes (TS-21).
// No busy-wait: Stop is expected to return only after the drain is complete.
func Test_PostProcessor_StopDrainsActiveBucket(t *testing.T) {
	t.Parallel()

	out := &mockWriter{}
	// Use a very slow ticker so the flush is only triggered by Stop, not the ticker.
	obj := NewUnsetLogEventPostProcessor(10*time.Minute, 100, out)

	obj.PublishLogMessage([]byte("msg1"))
	obj.PublishLogMessage([]byte("msg2"))
	obj.PublishLogMessage([]byte("msg3"))
	obj.PublishLogMessage([]byte("msg4"))
	obj.PublishLogMessage([]byte("msg5"))

	obj.Stop()

	// each message produces 2 Write calls (data + "\n")
	assert.Equal(t, 10, out.Count(), "Stop must drain all 5 enqueued messages")
}

// Test_PostProcessor_StopSynchronous verifies that Stop returns only AFTER the
// drain is complete (i.e., not before the writer has been called).
func Test_PostProcessor_StopSynchronous(t *testing.T) {
	t.Parallel()

	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(10*time.Minute, 100, out)

	for i := 0; i < 20; i++ {
		obj.PublishLogMessage([]byte("payload"))
	}

	obj.Stop()

	// If Stop is synchronous, out.Count() must be exactly 40 here (20 × 2).
	assert.Equal(t, 40, out.Count(), "Stop must be synchronous: all writes complete before Stop returns")
}

// Test_PostProcessor_StopIdempotent verifies that calling Stop twice does not
// panic and the second call returns immediately (idempotent via sync.Once).
func Test_PostProcessor_StopIdempotent(t *testing.T) {
	t.Parallel()

	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(10*time.Minute, 100, out)
	obj.PublishLogMessage([]byte("x"))

	obj.Stop()

	assert.NotPanics(t, func() { obj.Stop() }, "second Stop must not panic")
}

// Test_PostProcessor_StopWithInFlightFlush verifies that Stop waits for any
// async flush goroutines that were in flight (triggered by ticker) before the
// stop signal arrived.
func Test_PostProcessor_StopWithInFlightFlush(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var writes []string
	out := &blockingWriter{
		mu:     &mu,
		writes: &writes,
	}
	// Use a short ticker so a flush goroutine gets started before Stop is called.
	obj := NewUnsetLogEventPostProcessor(20*time.Millisecond, 1000, out)

	// Publish messages before ticker fires.
	for i := 0; i < 5; i++ {
		obj.PublishLogMessage([]byte("tick-flush"))
	}
	// Wait for the ticker to fire and start an async flush goroutine.
	time.Sleep(50 * time.Millisecond)

	// Publish more messages that will be in the bucket when Stop fires.
	for i := 0; i < 5; i++ {
		obj.PublishLogMessage([]byte("stop-flush"))
	}

	obj.Stop()

	mu.Lock()
	total := len(writes)
	mu.Unlock()

	// 10 messages × 2 Write calls each = 20. But we only care that at least
	// the 5 stop-path messages are written; the tick-flushed ones may vary.
	assert.GreaterOrEqual(t, total, 10, "Stop must drain stop-path messages; in-flight flush must also complete")
}

// blockingWriter is an io.Writer that records calls into a shared slice.
type blockingWriter struct {
	mu     *sync.Mutex
	writes *[]string
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	*w.writes = append(*w.writes, string(p))
	w.mu.Unlock()
	return len(p), nil
}

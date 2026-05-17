package pipelineStage

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockWriter is a thread-safe io.Writer that counts Write calls.
// It is shared across test files in this package.
type mockWriter struct {
	mu     sync.Mutex
	called int
}

// Write increments the call counter and satisfies io.Writer.
func (mw *mockWriter) Write(p []byte) (n int, err error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.called++
	return len(p), nil
}

// Count returns the number of times Write has been called.
func (mw *mockWriter) Count() int {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	return mw.called
}

// TestPublishMessageAndNoIO verifies that messages below the bucket limit
// are never written to the output before a flush is triggered.
func TestPublishMessageAndNoIO(t *testing.T) {
	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(time.Second, 10, out)
	defer obj.Stop()

	assert.Equal(t, "unsetLogEventPostProcessor", obj.Name())
	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	obj.PublishLogMessage([]byte("test message 3"))
	assert.Equal(t, 0, out.Count())
}

// TestPublishMessageWithIOAfterBufferReached verifies that publishing
// maxBucketSize messages triggers an async flush of exactly those messages.
// With maxBucketSize=3, the swap fires on the 3rd append (append-first
// semantics: append msg3 → len=3 >= max=3 → swap and spawn flush goroutine).
// 3 messages × 2 Write calls (data + "\n") = 6 total writes.
// msg4 arrives into a fresh bucket and stays there (rate=1min).
func TestPublishMessageWithIOAfterBufferReached(t *testing.T) {
	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(time.Minute, 3, out)
	defer obj.Stop()

	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	// After msg2: len=2 < max=3, no flush yet.
	assert.Equal(t, 0, out.Count())

	// msg3 fills the bucket: append → len=3 >= max=3 → swap is triggered
	// under the lock; a goroutine is spawned to flush [1,2,3].
	obj.PublishLogMessage([]byte("test message 3"))

	// 3 messages × 2 writes each = 6 total writes, delivered asynchronously.
	assert.Eventually(t, func() bool { return out.Count() == 6 }, 200*time.Millisecond, 10*time.Millisecond)

	// msg4 arrives into the fresh bucket and stays buffered.
	obj.PublishLogMessage([]byte("test message 4"))
	assert.Equal(t, 6, out.Count())
}

// TestPublishMessageWithIOAfterRate verifies that the ticker-driven flush
// writes all buffered messages even when the bucket limit is never reached.
// Uses assert.Eventually instead of time.Sleep to avoid flakiness (T-8 fix).
func TestPublishMessageWithIOAfterRate(t *testing.T) {
	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(50*time.Millisecond, 10, out)
	defer obj.Stop()

	// No events → ticker fires → nothing written.
	assert.Equal(t, 0, out.Count())

	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	obj.PublishLogMessage([]byte("test message 3"))

	// 3 messages × 2 writes each = 6 total writes, delivered by the ticker.
	assert.Eventually(t, func() bool { return out.Count() == 6 }, 2*time.Second, 20*time.Millisecond)
}

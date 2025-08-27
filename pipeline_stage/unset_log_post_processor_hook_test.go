package pipelineStage

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockWriter struct {
	mu     sync.Mutex
	called int
}

func (mw *mockWriter) Write(p []byte) (n int, err error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.called++
	return len(p), nil
}

func (mw *mockWriter) Count() int {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	return mw.called
}

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

func TestPublishMessageWithIOAfterBufferReached(t *testing.T) {
	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(time.Minute, 3, out)
	defer obj.Stop()

	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	obj.PublishLogMessage([]byte("test message 3"))
	assert.Equal(t, 0, out.Count())

	obj.PublishLogMessage([]byte("test message 4"))

	assert.Eventually(t, func() bool { return out.Count() == 3 }, 200*time.Millisecond, 50*time.Millisecond)
}

func TestPublishMessageWithIOAfterRate(t *testing.T) {
	out := &mockWriter{}
	obj := NewUnsetLogEventPostProcessor(500*time.Millisecond, 10, out)
	defer obj.Stop()
	time.Sleep(time.Second)
	assert.Equal(t, 0, out.Count())
	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	obj.PublishLogMessage([]byte("test message 3"))

	assert.Eventually(t, func() bool { return out.Count() == 3 }, time.Second, 50*time.Millisecond)
}

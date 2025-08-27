package pipelineStage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockWriter struct {
	called int
}

func (mw *mockWriter) Write(p []byte) (n int, err error) {
	mw.called++
	return len(p), nil
}

func TestPublishMessageAndNoIO(t *testing.T) {
	out := &mockWriter{
		called: 0,
	}
	obj := NewUnsetLogEventPostProcessor(time.Second, 10, out)
	assert.Equal(t, "unsetLogEventPostProsessor", obj.Name())
	obj.PublishLogMessage([]byte("test message 1"))
	assert.Equal(t, 0, out.called)
	obj.PublishLogMessage([]byte("test message 2"))
	assert.Equal(t, 0, out.called)
	obj.PublishLogMessage([]byte("test message 3"))
	assert.Equal(t, 0, out.called)
	obj.PublishLogMessage([]byte("test message 4"))
	assert.Equal(t, 0, out.called)
	obj.PublishLogMessage([]byte("test message 5"))
	assert.Equal(t, 0, out.called)
}

func TestPublishMessageWithIOAfterBufferReached(t *testing.T) {
	out := &mockWriter{
		called: 0,
	}
	obj := NewUnsetLogEventPostProcessor(time.Minute, 3, out)

	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	obj.PublishLogMessage([]byte("test message 3"))
	assert.Equal(t, 0, out.called)
	obj.PublishLogMessage([]byte("test message 4"))
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 3, out.called)
	obj.PublishLogMessage([]byte("test message 5"))
	assert.Equal(t, 3, out.called)
}

func TestPublishMessageWithIOAfterRate(t *testing.T) {
	out := &mockWriter{
		called: 0,
	}
	obj := NewUnsetLogEventPostProcessor(time.Second, 3, out)

	obj.PublishLogMessage([]byte("test message 1"))
	obj.PublishLogMessage([]byte("test message 2"))
	obj.PublishLogMessage([]byte("test message 3"))
	assert.Equal(t, 0, out.called)
	time.Sleep(2 * time.Second)
	assert.Equal(t, 3, out.called)
	obj.PublishLogMessage([]byte("test message 4"))
	obj.PublishLogMessage([]byte("test message 5"))
	assert.Equal(t, 3, out.called)
}

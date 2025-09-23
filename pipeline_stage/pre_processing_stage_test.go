package pipelineStage

import (
	"testing"

	"github.com/architagr/lognugget/enum"
	"github.com/stretchr/testify/assert"
)

type mockUnsetHook struct {
	isCalled bool
}

func (e *mockUnsetHook) PublishLogMessage(entry []byte) {
	e.isCalled = true

}
func (e *mockUnsetHook) Name() string {
	return "mockUnsetHook"
}

type mockDebugHook struct {
	isCalled bool
}

func (e *mockDebugHook) PublishLogMessage(entry []byte) {
	e.isCalled = true

}
func (e *mockDebugHook) Name() string {
	return "mockUnsetHook"
}

func TestPublishMessageNoLevelHookCalled(t *testing.T) {
	unsetHook := &mockUnsetHook{}
	debugHook := &mockDebugHook{}
	obj := newEventPreProcessingObserver()
	obj.DeRegisterHook(enum.LevelUnSet, unsetHook.Name())
	obj.RegisterHook(enum.LevelUnSet, unsetHook)
	obj.RegisterHook(enum.LevelDebug, debugHook)

	obj.PreProcess(enum.LevelError, []byte(""))
	assert.Equal(t, "EventPreProcessorObserver", obj.Name())
	assert.True(t, unsetHook.isCalled)
	assert.False(t, debugHook.isCalled)
}

func TestPublishMessage(t *testing.T) {
	unsetHook := &mockUnsetHook{}
	debugHook := &mockDebugHook{}
	obj := newEventPreProcessingObserver()
	obj.RegisterHook(enum.LevelUnSet, unsetHook)
	obj.RegisterHook(enum.LevelDebug, debugHook)

	obj.PreProcess(enum.LevelDebug, []byte(""))

	assert.True(t, unsetHook.isCalled)
	assert.True(t, debugHook.isCalled)
}

func TestDeregister(t *testing.T) {
	unsetHook := &mockUnsetHook{}
	debugHook := &mockDebugHook{}
	obj := newEventPreProcessingObserver()
	obj.RegisterHook(enum.LevelUnSet, unsetHook)
	obj.RegisterHook(enum.LevelDebug, debugHook)
	obj.DeRegisterHook(enum.LevelDebug, debugHook.Name())
	obj.PreProcess(enum.LevelDebug, []byte(""))
	assert.True(t, unsetHook.isCalled)
	assert.False(t, debugHook.isCalled)
}

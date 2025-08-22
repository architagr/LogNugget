package pipelineStage

import (
	"testing"

	"github.com/architagr/lognugget/enum"
	"github.com/stretchr/testify/assert"
)

type mockEntry struct {
	data  map[string]any
	level enum.LogLevel
}

func (e *mockEntry) ToMap() map[string]any {
	return e.data
}
func (e *mockEntry) Level() enum.LogLevel {
	return e.level
}

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
	obj := NewEventPreProcessingObserver()
	obj.RegisterHook(enum.LevelUnSet, unsetHook)
	obj.RegisterHook(enum.LevelDebug, debugHook)

	obj.PreProcess(&mockEntry{
		data:  map[string]any{},
		level: enum.LevelError,
	})

	assert.True(t, unsetHook.isCalled)
	assert.False(t, debugHook.isCalled)
}

func TestPublishMessage(t *testing.T) {
	unsetHook := &mockUnsetHook{}
	debugHook := &mockDebugHook{}
	obj := NewEventPreProcessingObserver()
	obj.RegisterHook(enum.LevelUnSet, unsetHook)
	obj.RegisterHook(enum.LevelDebug, debugHook)

	obj.PreProcess(&mockEntry{
		data:  map[string]any{},
		level: enum.LevelDebug,
	})

	assert.True(t, unsetHook.isCalled)
	assert.True(t, debugHook.isCalled)
}

func TestDeregister(t *testing.T) {
	unsetHook := &mockUnsetHook{}
	debugHook := &mockDebugHook{}
	obj := NewEventPreProcessingObserver()
	obj.RegisterHook(enum.LevelUnSet, unsetHook)
	obj.RegisterHook(enum.LevelDebug, debugHook)
	obj.DeRegisterHook(enum.LevelDebug, debugHook.Name())
	obj.PreProcess(&mockEntry{
		data:  map[string]any{},
		level: enum.LevelDebug,
	})

	assert.True(t, unsetHook.isCalled)
	assert.False(t, debugHook.isCalled)
}

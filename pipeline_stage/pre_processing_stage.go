package pipelineStage

import (
	"sync"

	"github.com/architagr/lognugget/enum"
)

var EventPreProcessorObj *eventPreProcessorObserver

func init() {
	(&sync.Once{}).Do(func() {
		EventPreProcessorObj = newEventPreProcessingObserver()
	})
}

type publishLogMessageHookContract interface {
	PublishLogMessage(entry []byte)
	Name() string
}

type eventPreProcessorObserver struct {
	hooks map[enum.LogLevel]map[string]publishLogMessageHookContract
}

func newEventPreProcessingObserver() *eventPreProcessorObserver {
	return &eventPreProcessorObserver{
		hooks: make(map[enum.LogLevel]map[string]publishLogMessageHookContract),
	}
}

func (e *eventPreProcessorObserver) RegisterHook(level enum.LogLevel, hook publishLogMessageHookContract) {
	levelHooks, exists := e.hooks[level]
	if !exists {
		levelHooks = make(map[string]publishLogMessageHookContract)
	}
	levelHooks[hook.Name()] = hook
	e.hooks[level] = levelHooks
}

func (e *eventPreProcessorObserver) DeRegisterHook(level enum.LogLevel, hookName string) {
	levelHooks, exists := e.hooks[level]
	if !exists {
		return
	}
	delete(levelHooks, hookName)
	e.hooks[level] = levelHooks
}

func (e *eventPreProcessorObserver) PreProcess(level enum.LogLevel, logMsg []byte) {
	publish(logMsg, e.hooks[enum.LevelUnSet])
	publish(logMsg, e.hooks[level])

}

func publish(byteData []byte, hooks map[string]publishLogMessageHookContract /*, wg *sync.WaitGroup*/) {
	// defer wg.Done()
	for _, unsetHook := range hooks {
		unsetHook.PublishLogMessage(byteData)
	}
}

func (e *eventPreProcessorObserver) Name() string {
	return "EventPreProcessorObserver"
}

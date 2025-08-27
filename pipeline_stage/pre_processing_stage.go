package pipelineStage

import (
	"sync"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

var EventPreProcessorObj *eventPreProcessorObserver

func init() {
	(&sync.Once{}).Do(func() {
		EventPreProcessorObj = newEventPreProcessingObserver()
	})
}

type logEntryContract interface {
	ToMap() map[string]any
	Level() enum.LogLevel
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

func (e *eventPreProcessorObserver) PreProcess(entryObj logEntryContract) {
	byteData, err := config.GetConfig().Encoder().Write(entryObj.ToMap())
	if err != nil {
		return
	}
	wg := sync.WaitGroup{}
	wg.Add(2)
	go publish(byteData, e.hooks[enum.LevelUnSet], &wg)
	go publish(byteData, e.hooks[entryObj.Level()], &wg)
	wg.Wait()
}

func publish(byteData []byte, hooks map[string]publishLogMessageHookContract, wg *sync.WaitGroup) {
	defer wg.Done()
	for _, unsetHook := range hooks {
		unsetHook.PublishLogMessage(byteData)
	}
}

func (e *eventPreProcessorObserver) Name() string {
	return "EventPreProcessorObserver"
}

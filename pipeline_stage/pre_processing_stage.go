// Package pipelineStage implements the pre-processing stage of the log
// pipeline. Its sole responsibility is fan-out: each incoming log event is
// delivered to every registered publishLogMessageHookContract whose level
// subscription matches. It does not encode, buffer, or persist events.
//
// Entry point: EventPreProcessorObj — a package-level singleton initialised
// once by init. Callers use RegisterHook / DeRegisterHook to manage
// subscriptions and PreProcess to dispatch an event.
package pipelineStage

import (
	"sync"

	"github.com/architagr/lognugget/enum"
)

// EventPreProcessorObj is the package-level singleton event fan-out observer.
// It is safe to use after package init completes; callers must not replace or
// nil this variable.
//
// why: assigned directly in init — no sync.Once wrapper needed because init
// runs exactly once per process; the (&sync.Once{}).Do pattern previously used
// created a transient Once on every call, providing no actual guard (D-3).
var EventPreProcessorObj *eventPreProcessorObserver

func init() {
	EventPreProcessorObj = newEventPreProcessingObserver()
}

// publishLogMessageHookContract is the consumer-side interface for hooks that
// receive serialised log events. Implementors must be safe to call from a
// single goroutine; concurrent calls are not made by this package.
//
// Name must return a stable, non-empty string that is unique within a given
// log level — it is used as the map key for RegisterHook / DeRegisterHook.
// Registering a second hook with the same Name at the same level silently
// overwrites the first.
type publishLogMessageHookContract interface {
	// PublishLogMessage delivers a serialised log event to the hook.
	// entry must not be retained after the call returns; make a copy if
	// the hook needs it beyond the call frame.
	PublishLogMessage(entry []byte)
	// Name returns the unique identifier for this hook within its level.
	Name() string
}

// eventPreProcessorObserver fans out log events to registered hooks keyed by
// log level. mu guards hooks for concurrent Register/DeRegister and PreProcess.
type eventPreProcessorObserver struct {
	mu    sync.RWMutex
	hooks map[enum.LogLevel]map[string]publishLogMessageHookContract
}

// newEventPreProcessingObserver returns a zero-state observer with no hooks.
func newEventPreProcessingObserver() *eventPreProcessorObserver {
	return &eventPreProcessorObserver{
		hooks: make(map[enum.LogLevel]map[string]publishLogMessageHookContract),
	}
}

// RegisterHook adds hook to the set of callbacks invoked when PreProcess is
// called with a matching level. If a hook with the same Name is already
// registered at level, it is silently overwritten. Safe for concurrent use.
func (e *eventPreProcessorObserver) RegisterHook(level enum.LogLevel, hook publishLogMessageHookContract) {
	e.mu.Lock()
	defer e.mu.Unlock()
	levelHooks, exists := e.hooks[level]
	if !exists {
		levelHooks = make(map[string]publishLogMessageHookContract)
	}
	levelHooks[hook.Name()] = hook
	e.hooks[level] = levelHooks
}

// DeRegisterHook removes the hook identified by hookName from level. It is a
// no-op if the level has no registered hooks or hookName is not present.
// Safe for concurrent use.
func (e *eventPreProcessorObserver) DeRegisterHook(level enum.LogLevel, hookName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	levelHooks, exists := e.hooks[level]
	if !exists {
		return
	}
	delete(levelHooks, hookName)
	e.hooks[level] = levelHooks
}

// PreProcess delivers logMsg to all hooks registered at enum.LevelUnSet and
// then to all hooks registered at the given level.
//
// ARCH-11: hook invocation order within a level is undefined (Go map
// iteration is randomised). Do not rely on any particular delivery order
// across hooks at the same level.
func (e *eventPreProcessorObserver) PreProcess(level enum.LogLevel, logMsg []byte) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	publish(logMsg, e.hooks[enum.LevelUnSet])
	publish(logMsg, e.hooks[level])
}

// publish calls PublishLogMessage on every hook in the map. Iteration order
// is undefined — callers must not assume any ordering guarantee.
func publish(byteData []byte, hooks map[string]publishLogMessageHookContract) {
	for _, hook := range hooks {
		hook.PublishLogMessage(byteData)
	}
}

// Name returns the observer's stable identifier used by the pipeline registry.
func (e *eventPreProcessorObserver) Name() string {
	return "EventPreProcessorObserver"
}

//go:build testing

// Package race contains tests that exercise concurrent access patterns across
// the full LogNugget pipeline. Every test in this package must pass with
// `go test -race` to satisfy NF9 (race-freedom).
//
// NF7 contract: hook registration via config.RegisterHook / config.DeRegisterHook
// is concurrency-safe (protected by configMu), but the eventPreProcessorObserver
// hooks map is mutated only at program start-up (single-threaded init path)
// and is thereafter read-only. The tests here verify that the two thread-safe
// surfaces (config.RegisterHook and unsetLogEventPostProcessor.PublishLogMessage)
// are race-free under concurrent load.
package race

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/stretchr/testify/assert"
)

// discardWriter is a minimal thread-safe io.Writer used to absorb output
// from the post-processor without allocating per-write.
type discardWriter struct{}

// Write satisfies io.Writer by discarding p.
func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// hookStub is a no-op PublishLogMessageHookContract used in registration tests.
type hookStub struct{ name string }

// PublishLogMessage is a no-op to satisfy PublishLogMessageHookContract.
func (h *hookStub) PublishLogMessage(_ []byte) {}

// Name returns the hook's identifier for registration keying.
func (h *hookStub) Name() string { return h.name }

// Test_Race_HookRegisterUnderLog verifies that config.RegisterHook and
// config.DeRegisterHook are race-free when called concurrently with active
// log publishers that drive the pipeline through config.PublishLog.
//
// Setup:
//   - A fresh config state is created via TestResetConfig so EventPreProcessors
//     is clean for this test.
//   - A post-processor flushing to a discardWriter is registered via
//     config.RegisterHook at LevelUnSet.
//   - 100 goroutines publish log messages continuously for 50 ms.
//   - 1 goroutine repeatedly registers and de-registers a stub hook at
//     LevelDebug for the same 50 ms window.
//
// NF7 note: concurrent register/deregister is exercised here to confirm that
// the RWMutex semantics in config are sufficient for race-freedom. This is
// NOT an endorsement of the pattern — hook registration should be done at
// start-up in production code (NF7). The test documents the boundary: the
// configMu write-lock protects the map write; it does not guarantee in-flight
// PreProcess calls observe the updated map atomically.
//
// The eventPreProcessorObserver singleton (EventPreProcessorObj) is intentionally
// NOT used in this test because its hooks map has no mutex — it is designed
// for mutation at program start-up only (NF7 startup-only contract).
//
// NF9: the -race detector must report no data races on this path.
func Test_Race_HookRegisterUnderLog(t *testing.T) {
	// Isolate config state for this test.
	// why: TestResetConfig is the build-tagged shim that reaches the unexported
	// resetConfig — the production API has no exported reset to prevent misuse
	// at runtime (D-9 / ARCH-15). After this call defaultConfig.hooks is
	// initialised to an empty map (fix landed alongside TS-22).
	config.TestResetConfig()

	// Use a fresh local observer (not the global EventPreProcessorObj) so
	// that this test's pre-processor mutations are isolated from the global
	// singleton used by other tests.
	//
	// why: EventPreProcessorObj.hooks has no mutex. Calling RegisterHook or
	// DeRegisterHook on the singleton concurrently with ProcessLogEvent
	// (which calls PreProcess → iterates hooks) would be a data race.
	// The NF7 startup-only contract means the singleton's map is mutated
	// only before any goroutines start reading it — this test verifies the
	// config-level RWMutex, not the observer-level map.
	localObserver := newLocalObserver()
	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, 10_000, discardWriter{})
	t.Cleanup(func() { proc.Stop() })

	// Register proc into the local observer (single-threaded, before
	// concurrent goroutines start — satisfies NF7 for the observer level).
	localObserver.addHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(localObserver)

	const duration = 50 * time.Millisecond
	deadline := time.Now().Add(duration)

	var wg sync.WaitGroup

	// 100 goroutines publish log events via config.PublishLog for the
	// duration. This exercises the read-lock path inside ProcessLogEvent
	// concurrently with the write-lock path in Register/DeRegisterHook.
	const logGoroutines = 100
	for g := 0; g < logGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				config.PublishLog(enum.LevelDebug, []byte(`{"msg":"concurrent log"}`))
			}
		}()
	}

	// 1 goroutine repeatedly registers and de-registers a stub hook at
	// LevelDebug via config. This exercises the configMu write-lock path
	// concurrently with the RLock held by ProcessLogEvent.
	//
	// why: NF7 says registration is startup-only, but the config-level lock
	// must still be correct if a caller violates that contract (e.g. during
	// graceful config reload). This test confirms race-freedom of the config
	// layer, not correctness of event delivery ordering.
	wg.Add(1)
	go func() {
		defer wg.Done()
		stub := &hookStub{name: "race-test-debug-hook"}
		for time.Now().Before(deadline) {
			config.RegisterHook(enum.LevelDebug, stub)
			config.DeRegisterHook(enum.LevelDebug, stub.Name())
		}
	}()

	wg.Wait()

	// No assertion on counts — the test goal is race-freedom. A passing
	// -race run with no reported data races is the success criterion.
	assert.True(t, true, "reached here without race or panic")
}

// Test_Race_StopWhileLoggingIntegration validates that calling Stop on a
// post-processor while the config pipeline is actively dispatching events
// neither panics nor deadlocks.
//
// This complements Test_Race_StopWhileLogging in pipeline_stage/ by driving
// the full pipeline (config.PublishLog → ProcessLogEvent → PreProcess →
// PublishLogMessage) rather than calling PublishLogMessage directly.
//
// NF9: the -race detector must report no data races on this path.
func Test_Race_StopWhileLoggingIntegration(t *testing.T) {
	config.TestResetConfig()

	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, 64, discardWriter{})
	t.Cleanup(func() { proc.Stop() })

	// Use a local observer, not the global EventPreProcessorObj singleton,
	// to avoid touching the singleton's unprotected hooks map after goroutines
	// start. The hook is registered once here (NF7 startup-only contract for
	// the observer layer).
	localObserver := newLocalObserver()
	localObserver.addHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(localObserver)

	var stopped atomic.Bool

	var wg sync.WaitGroup
	const publishers = 20
	for g := 0; g < publishers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stopped.Load() {
				config.PublishLog(enum.LevelInfo, []byte(`{"msg":"stop-race"}`))
			}
		}()
	}

	// Give publishers a head start, then stop the processor.
	time.Sleep(10 * time.Millisecond)
	stopped.Store(true)

	// Stop blocks until the drain is complete. If it hangs, the test
	// fails via the -timeout flag.
	proc.Stop()

	wg.Wait()
}

// localObserver is a minimal preProcessingObserverContract implementation used
// in race tests to avoid touching the shared EventPreProcessorObj singleton
// whose hooks map has no mutex (NF7 startup-only constraint).
//
// It holds a fixed, read-only snapshot of hooks registered before any goroutines
// start; it never mutates the hooks map after AddHook returns.
type localObserver struct {
	hooks []config.PublishLogMessageHookContract
}

// newLocalObserver returns an empty observer ready for hook registration.
func newLocalObserver() *localObserver { return &localObserver{} }

// addHook appends hook to the delivery list. Must be called before any
// goroutines start (NF7 startup-only contract).
func (o *localObserver) addHook(level enum.LogLevel, hook config.PublishLogMessageHookContract) {
	// level is accepted to satisfy the caller signature but is ignored:
	// this observer delivers every event to every registered hook regardless
	// of level, which is sufficient for the race-test use case.
	o.hooks = append(o.hooks, hook)
}

// PreProcess delivers logMsg to every registered hook.
func (o *localObserver) PreProcess(_ enum.LogLevel, logMsg []byte) {
	for _, h := range o.hooks {
		h.PublishLogMessage(logMsg)
	}
}

// Name returns a stable identifier for the config pre-processor registry.
func (o *localObserver) Name() string { return "localObserver" }

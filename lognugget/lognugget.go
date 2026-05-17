// Package lognugget is the top-level façade for the LogNugget structured
// logging library. It wires together the pipeline_stage post-processor and
// exposes a single package-level Shutdown function for graceful drain-on-exit.
//
// Entry points:
//   - [Shutdown] — block until the default post-processor has flushed all
//     buffered log messages and shut down its background goroutine.
//
// This package does NOT own the logger configuration, entry construction, or
// encoder selection; those responsibilities live in the config, entry, and
// encoder packages respectively.
package lognugget

import (
	"os"
	"sync"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// postProcessor is the narrow interface consumed by this package.
// It is satisfied by *pipeline_stage.unsetLogEventPostProcessor (and any
// future implementations) without importing the concrete type, keeping the
// coupling to a minimum.
//
// Contract:
//   - PublishLogMessage must be safe to call concurrently.
//   - Stop must block until all buffered messages have been written and must
//     be idempotent; a second call must return without panicking.
//   - Name returns a human-readable identifier used in diagnostics.
type postProcessor interface {
	PublishLogMessage([]byte)
	Name() string
	Stop()
}

var (
	// defaultPostProc is the package-level post-processor instance.
	// Set by init() and may be overridden in tests via direct assignment.
	defaultPostProc postProcessor

	// shutdownOnce ensures Shutdown is executed at most once regardless of
	// how many goroutines call it concurrently.
	// why: sync.Once is the idiomatic, race-free way to guarantee one-shot
	// execution without requiring callers to coordinate.
	shutdownOnce sync.Once
)

// init wires the zero-config default pipeline:
//   - one unsetLogEventPostProcessor (rate=1s, max=20, stdout)
//   - registered at LevelUnSet so every log level is captured
//   - EventPreProcessorObj registered as the sole pre-processor
//
// No user code is required to activate logging; importing this package
// is sufficient (SC1 / ARCH-5).
func init() {
	proc := pipelineStage.NewUnsetLogEventPostProcessor(time.Second, 20, os.Stdout)
	defaultPostProc = proc
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
}

// Shutdown drains the default post-processor and blocks until all buffered
// log messages have been written to the underlying output. It is safe to call
// from multiple goroutines; only the first call performs the drain — subsequent
// calls return immediately without blocking.
//
// Callers should invoke Shutdown at the end of main() or in a signal handler
// to ensure in-flight log events reach their destination before the process
// exits.
//
// N-5 loss window: Fatal and Panic log events that have been dispatched to the
// post-processor's internal channel but not yet drained when Shutdown is called
// will be flushed by the drain. However, events that are still in-flight inside
// the caller's goroutine (e.g. not yet passed to PublishLogMessage) at the
// moment Shutdown completes may be lost. Ensure all logging goroutines have
// finished publishing before calling Shutdown to close this window.
func Shutdown() {
	shutdownOnce.Do(func() {
		if defaultPostProc != nil {
			defaultPostProc.Stop()
		}
	})
}

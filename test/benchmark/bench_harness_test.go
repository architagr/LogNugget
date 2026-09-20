package benchmark

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// benchOpts describes the per-benchmark deviations from the clean baseline
// configuration installed by setupBench. The zero value is "defaults only".
type benchOpts struct {
	// minLevel is the gate level. Defaults to enum.LevelDebug so the hot path
	// is exercised; set it explicitly to benchmark the filtered path.
	minLevel enum.LogLevel
	// ctxAppender / ctxFields / ctxParser select at most one context-field
	// strategy. Leaving all three nil benchmarks the context-free path.
	ctxAppender config.ContextFieldsAppender
	ctxFields   config.ContextFieldsFunc
	ctxParser   config.ContextFieldsParser
	// staticParser installs once-evaluated static fields (hostname, version).
	staticParser config.StaticEnvFieldsParser
	// syncMode routes delivery onto the calling goroutine (V4-P1).
	syncMode bool
	// poolEntries is the pre-warmed LogEntry pool size. Defaults to
	// GOMAXPROCS*64, which matches a production process rather than the
	// 1M-entry pool that used to mask GC pressure.
	poolEntries int
}

// setupBench installs a known-clean global configuration for one benchmark and
// tears it down afterwards.
//
// why: every knob in package config is a process-global singleton. Benchmarks
// that set a parser or register a hook and never clear it leak that state into
// every benchmark that runs later in the same binary — Benchmark_Log's legacy
// map context parser alone added ~340 B/op and 2 allocs/op to every subsequent
// benchmark, which is how the published "4 allocs" hot-path number was produced.
// Routing all setup through this helper (with b.Cleanup teardown) makes each
// benchmark's result independent of the order the suite happens to run in.
func setupBench(b *testing.B, opts benchOpts) {
	b.Helper()

	resetGlobalConfig()

	minLevel := opts.minLevel
	if minLevel == enum.LogLevel(0) {
		minLevel = enum.LevelDebug
	}
	config.SetMinLevel(minLevel)
	config.SetEncoderType(enum.EncoderJSON)

	if opts.staticParser != nil {
		config.SetStaticEnvFieldsParser(opts.staticParser)
	}
	if opts.ctxAppender != nil {
		config.SetContextFieldsAppender(opts.ctxAppender)
	}
	if opts.ctxFields != nil {
		config.SetContextFields(opts.ctxFields)
	}
	if opts.ctxParser != nil {
		config.SetContextFieldsParser(opts.ctxParser)
	}

	config.SetSyncMode(opts.syncMode)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, &MockWriter{})
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)

	poolEntries := opts.poolEntries
	if poolEntries == 0 {
		poolEntries = runtime.GOMAXPROCS(0) * 64
	}
	entry.GenerateInitialPool(poolEntries)

	b.Cleanup(func() {
		proc.Stop()
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())
		resetGlobalConfig()
	})
}

// resetGlobalConfig clears every global knob a benchmark can set, using only
// the public setters so the helper compiles without the `testing` build tag.
func resetGlobalConfig() {
	config.SetStaticEnvFieldsParser(nil)
	config.SetContextFieldsParser(nil)
	config.SetContextFieldsAppender(nil)
	config.SetContextFields(nil)
	config.SetAddSource(false)
	config.SetTimeFormat(time.RFC3339)
	config.SetMinLevel(enum.LevelInfo)
	config.SetSyncMode(false)
}

// benchCtx returns a background context carrying the two demo context values
// used by the legacy map-parser benchmarks.
func benchCtx() context.Context {
	return context.WithValue(
		context.WithValue(context.Background(), ctxKeyRequestID, "req-bench"),
		ctxKeyUserID, "user-bench",
	)
}

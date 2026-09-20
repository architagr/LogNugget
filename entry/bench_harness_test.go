package entry_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	pipelineStage "github.com/architagr/lognugget/v4/pipeline_stage"
)

// benchSink is the shared discard writer for every benchmark in this package.
type benchSink struct{}

func (benchSink) Write(p []byte) (int, error) { return len(p), nil }

// entryBenchOpts describes one benchmark's deviations from the clean baseline
// configuration installed by setupEntryBench. The zero value is defaults only.
type entryBenchOpts struct {
	minLevel    enum.LogLevel
	addSource   bool
	ctxAppender config.ContextFieldsAppender
	ctxFields   config.ContextFieldsFunc
	ctxParser   config.ContextFieldsParser
	// poolEntries defaults to GOMAXPROCS*64 — a production-shaped pool. Do not
	// scale it with b.N: an over-sized pool hides both cold-miss cost and the
	// GC pressure a real service would see.
	poolEntries int
	// bucketSize is the post-processor flush threshold (messages).
	bucketSize int
}

// setupEntryBench installs a clean global configuration for one benchmark and
// restores it afterwards.
//
// why: package config is a process-global singleton. A benchmark that installs
// a context parser or registers a hook without tearing it down changes the
// measurement of every benchmark that runs after it in the same binary, which
// silently inflated this package's published alloc counts.
func setupEntryBench(b *testing.B, opts entryBenchOpts) {
	b.Helper()

	resetEntryConfig()

	minLevel := opts.minLevel
	if minLevel == enum.LogLevel(0) {
		minLevel = enum.LevelDebug
	}
	config.SetMinLevel(minLevel)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetAddSource(opts.addSource)

	if opts.ctxAppender != nil {
		config.SetContextFieldsAppender(opts.ctxAppender)
	}
	if opts.ctxFields != nil {
		config.SetContextFields(opts.ctxFields)
	}
	if opts.ctxParser != nil {
		config.SetContextFieldsParser(opts.ctxParser)
	}

	bucket := opts.bucketSize
	if bucket == 0 {
		bucket = 500
	}
	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, bucket, benchSink{})
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
		resetEntryConfig()
	})
}

// resetEntryConfig clears every global knob these benchmarks touch, using only
// public setters so the helper needs no build tag.
func resetEntryConfig() {
	config.SetStaticEnvFieldsParser(nil)
	config.SetContextFieldsParser(nil)
	config.SetContextFieldsAppender(nil)
	config.SetContextFields(nil)
	config.SetAddSource(false)
	config.SetTimeFormat(time.RFC3339)
	config.SetMinLevel(enum.LevelInfo)
}

// tenFieldAppender is the shared 10-context-field payload used by the
// appender-path benchmarks, so their numbers are directly comparable.
func tenFieldAppender(_ context.Context, dst []byte) []byte {
	return append(dst, `,"k1":"v1","k2":"v2","k3":"v3","k4":"v4","k5":"v5","k6":"v6","k7":"v7","k8":"v8","k9":"v9","k10":"v10"`...)
}

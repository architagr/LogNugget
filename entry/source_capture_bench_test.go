package entry_test

import (
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// Benchmark_Log_AddSourceTrue measures the hot path through entry.Log when
// addSource=true. Input shape: single Info call with no extra fields, addSource
// enabled. Budget: 250 ns mean (bench gate ceiling is < 1 µs = 1,000 ns/op).
//
// why: runtime.Caller adds one frame-lookup per log call. This benchmark
// validates that the overhead remains negligible relative to the gate.
func Benchmark_Log_AddSourceTrue(b *testing.B) {
	b.StopTimer()

	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetAddSource(true)

	discard := &discardWriter{}
	unsetPostProcessor := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, discard)
	defer unsetPostProcessor.Stop()
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unsetPostProcessor)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(b.N + 1)

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Log(enum.LevelInfo, ctx, "bench-msg", nil)
	}
}

// discardWriter is an io.Writer that discards all input.
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }

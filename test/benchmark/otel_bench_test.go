//go:build otel_bench

package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// Benchmark_Log_Parallel_OtelCtx measures the hot path when a
// ContextFieldsAppender extracts OTel trace/span IDs from context.
// Build with: go test -tags otel_bench -bench=Benchmark_Log_Parallel_OtelCtx
//
// The appender simulates the OTel pattern (fixed-length hex IDs, no allocs)
// without importing the OTel SDK into the core module.
// A full OTel example lives in examples/otel-appender/.
func Benchmark_Log_Parallel_OtelCtx(b *testing.B) {
	out := &MockWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		// Simulate OTel trace/span ID extraction: fixed 32/16 hex chars, 0 allocs.
		dst = append(dst, `,"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`...)
		dst = append(dst, `,"span_id":"00f067aa0ba902b7"`...)
		return dst
	})

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
	defer proc.Stop()
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(1_000_000)

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(ctx, "otel-ctx bench")
		}
	})
}

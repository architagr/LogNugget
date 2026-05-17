package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// Benchmark_Log_Parallel measures throughput when b.N goroutines call
// the hot path concurrently. This is the primary parallel-throughput bench:
// it shows how LogNugget scales under concurrent load.
//
// Context threshold note: the context has 2 keys (requestID + userID).
// Recommended max is 10 values (including trace/span) — see README §Context.
func Benchmark_Log_Parallel(b *testing.B) {
	out := &MockWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
	defer proc.Stop()
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(1_000_000)

	ctx := context.WithValue(
		context.WithValue(context.Background(), ctxKeyRequestID, "req-bench"),
		ctxKeyUserID, "user-bench",
	)
	field := model.LogAttr{Key: "iter", Value: 0}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Debug(ctx, "parallel bench message", field)
		}
	})
}

// Benchmark_Log_Parallel_NoCtx is the parallel hot path with no context
// parser — isolates the context-free cost baseline.
func Benchmark_Log_Parallel_NoCtx(b *testing.B) {
	out := &MockWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
	defer proc.Stop()
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(1_000_000)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(context.Background(), "no-ctx bench")
		}
	})
}

// Benchmark_Log_Parallel_10CtxFields measures the recommended-maximum
// context load: 10 key-value pairs (including 2 tracing fields).
// This establishes the performance cost at the documented threshold.
func Benchmark_Log_Parallel_10CtxFields(b *testing.B) {
	out := &MockWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		return map[string]any{
			"trace_id":   "abc123def456",
			"span_id":    "span789",
			"request_id": "req-001",
			"user_id":    "usr-42",
			"session_id": "sess-x",
			"region":     "us-east-1",
			"service":    "api-gateway",
			"version":    "1.2.3",
			"env":        "production",
			"pod":        "pod-abc",
		}
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
			entry.NewLogEntry().Info(ctx, "10-ctx-fields bench")
		}
	})
}

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

type ctxBenchWriter struct{}

func (w *ctxBenchWriter) Write(p []byte) (int, error) { return len(p), nil }

type ctxBenchKey string

func setupCtxBenchLogger(b *testing.B) func() {
	b.Helper()
	out := &ctxBenchWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	unset := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unset)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(10_000)
	return unset.Stop
}

// BenchmarkLogEntry_CtxAppender_10 uses the zero-alloc ContextFieldsAppender
// path (P3). Target: ≥ 20 fewer allocs/op than the parser path.
func BenchmarkLogEntry_CtxAppender_10(b *testing.B) {
	b.StopTimer()
	stop := setupCtxBenchLogger(b)
	defer stop()

	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		reqID, _ := ctx.Value(ctxBenchKey("req")).(string)
		dst = append(dst, `,"req_id":"`...)
		dst = append(dst, reqID...)
		dst = append(dst, '"')
		dst = append(dst, `,"user_id":"bench-user","k1":"v1","k2":"v2","k3":"v3","k4":"v4","k5":"v5","k6":"v6","k7":"v7","k8":"v8","k9":"v9"`...)
		return dst
	})

	ctx := context.WithValue(context.Background(), ctxBenchKey("req"), "req-abc")
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "bench")
	}
}

// BenchmarkLogEntry_CtxParser_10 uses the legacy map[string]any parser path.
// Regression guard: alloc count must not increase vs P1 baseline.
func BenchmarkLogEntry_CtxParser_10(b *testing.B) {
	b.StopTimer()
	stop := setupCtxBenchLogger(b)
	defer stop()

	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		return map[string]any{
			"req_id":  "req-abc",
			"user_id": "bench-user",
			"k1": "v1", "k2": "v2", "k3": "v3",
			"k4": "v4", "k5": "v5", "k6": "v6",
			"k7": "v7", "k8": "v8", "k9": "v9",
		}
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "bench")
	}
}

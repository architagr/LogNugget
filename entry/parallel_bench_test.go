package entry_test

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

type parallelBenchWriter struct{}

func (w *parallelBenchWriter) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkLognugget_Parallel_10CtxFields is the final integration gate for
// the full V2 stack (P1-P5 combined). At GOMAXPROCS=8 the channel buffer of
// 1000 eliminates back-pressure; target ≤ 1,000 ns/op.
func BenchmarkLognugget_Parallel_10CtxFields(b *testing.B) {
	b.StopTimer()
	prev := runtime.GOMAXPROCS(8)
	b.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	out := &parallelBenchWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		dst = append(dst, `,"k1":"v1","k2":"v2","k3":"v3","k4":"v4","k5":"v5"`...)
		dst = append(dst, `,"k6":"v6","k7":"v7","k8":"v8","k9":"v9","k10":"v10"`...)
		return dst
	})
	unset := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 1000, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unset)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(100_000)
	b.Cleanup(unset.Stop)

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Debug(ctx, "parallel bench")
		}
	})
}

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

type inlineFramingWriter struct{}

func (w *inlineFramingWriter) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkLogEntry_InlineFraming_10 measures the hot path after P4 inline
// framing: OpenBytes prepend + CloseBytes append + ownership transfer,
// eliminating the en.Append intermediate buffer and the PublishLog defensive
// copy. Target: allocs/op drops by ≥ 1 vs the P3 baseline.
func BenchmarkLogEntry_InlineFraming_10(b *testing.B) {
	b.StopTimer()
	out := &inlineFramingWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"k1":"v1","k2":"v2","k3":"v3","k4":"v4","k5":"v5","k6":"v6","k7":"v7","k8":"v8","k9":"v9","k10":"v10"`...)
	})
	unset := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 1000, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unset)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(10_000)
	b.Cleanup(unset.Stop)

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Info(ctx, "bench p4")
	}
}

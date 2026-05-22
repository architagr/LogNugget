// Package entry_test: V3-P8 exact-size buffer severance benchmark.
//
// Measures B/op before and after changing the post-publish buffer reset from
// make([]byte, 0, initBufCap) (1024 B) to make([]byte, 0, max(64, len(data))).
// Target: B/op drops by ≥ 800 B relative to the initBufCap baseline.
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

type bufferSizeBenchWriter struct{}

func (w *bufferSizeBenchWriter) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkBufferSeverance measures the allocation bytes per call on the hot
// path after the P8 change: the replacement buffer created at severance is
// sized to max(64, len(data)) instead of the fixed initBufCap (1024 B).
//
// Input shape: single-goroutine, no context fields, JSON encoder, Debug level,
// typical short structured message (~60–80 B rendered).
// Budget defended: B/op must be ≤ 200 B (≥ 800 B saving vs 1024-byte baseline);
// overall ns/op must remain < 1000 (sub-1 µs SLO per CLAUDE.md §Performance Gate).
func BenchmarkBufferSeverance(b *testing.B) {
	b.StopTimer()
	out := &bufferSizeBenchWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
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
		entry.NewLogEntry().Debug(ctx, "typical structured log message")
	}
}

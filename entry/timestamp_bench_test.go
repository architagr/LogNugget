// Package entry_test: V3-P4 timestamp benchmark.
//
// Measures the allocation count and latency of the timestamp path after the
// direct AppendFormat change. The target is 0 allocs/op attributable to the
// timestamp formatting step, and an overall p99 < 1 µs.
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

type timestampBenchWriter struct{}

func (w *timestampBenchWriter) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkTimestamp_Direct measures the hot path with the direct
// now.AppendFormat(e.buf, snap.TimeFormat) timestamp encoding, replacing the
// former customTime.Format + AppendQuotedString (2 allocs). The target is
// 0 allocs attributable to timestamp formatting; overall p99 must remain < 1 µs.
//
// Input shape: single-goroutine, no context fields, JSON encoder, Debug level.
// Budget defended: sub-1 µs p99 per the LogNugget SLO (CLAUDE.md §Performance Gate).
func BenchmarkTimestamp_Direct(b *testing.B) {
	b.StopTimer()
	out := &timestampBenchWriter{}
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
		entry.NewLogEntry().Debug(ctx, "bench timestamp")
	}
}

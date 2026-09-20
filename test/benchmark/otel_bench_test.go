//go:build otel_bench

package benchmark

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/v4/entry"
)

// Benchmark_Log_Parallel_OtelCtx measures the hot path when a
// ContextFieldsAppender extracts OTel trace/span IDs from context.
// Build with: go test -tags otel_bench -bench=Benchmark_Log_Parallel_OtelCtx
//
// The appender simulates the OTel pattern (fixed-length hex IDs, no allocs)
// without importing the OTel SDK into the core module.
// A full OTel example lives in examples/otel-appender/.
func Benchmark_Log_Parallel_OtelCtx(b *testing.B) {
	setupBench(b, benchOpts{
		ctxAppender: func(ctx context.Context, dst []byte) []byte {
			// Simulate OTel trace/span ID extraction: fixed 32/16 hex chars, 0 allocs.
			dst = append(dst, `,"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`...)
			dst = append(dst, `,"span_id":"00f067aa0ba902b7"`...)
			return dst
		},
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(ctx, "otel-ctx bench")
		}
	})
}

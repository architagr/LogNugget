package entry_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/architagr/lognugget/v4/entry"
)

// BenchmarkLognugget_Parallel_10CtxFields is the package-level integration
// gate: full stack, 10 context fields via the appender, GOMAXPROCS=8.
// Target ≤ 1,000 ns/op (sub-1 µs SLO).
func BenchmarkLognugget_Parallel_10CtxFields(b *testing.B) {
	prev := runtime.GOMAXPROCS(8)
	b.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	setupEntryBench(b, entryBenchOpts{
		ctxAppender: tenFieldAppender,
		bucketSize:  1000,
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Debug(ctx, "parallel bench")
		}
	})
}

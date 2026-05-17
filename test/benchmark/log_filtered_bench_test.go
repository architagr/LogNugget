package benchmark

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
)

// Benchmark_Log_Filtered_BelowMinLevel measures the fast-reject path (F6):
// when the event level is below the configured minimum, the entry is returned
// to the pool before any encoding work is done.
//
// Target: < 80 ns, 0 allocs (see LLD §3.2).
func Benchmark_Log_Filtered_BelowMinLevel(b *testing.B) {
	config.SetMinLevel(enum.LevelError) // only Error+ passes
	entry.GenerateInitialPool(1_000_000)

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Debug is below Error: gate fires immediately, entry is Put back.
		entry.NewLogEntry().Debug(ctx, "filtered-out message")
	}
}

// Benchmark_Log_Filtered_BelowMinLevel_Parallel is the parallel variant of
// the filtered-path bench.
func Benchmark_Log_Filtered_BelowMinLevel_Parallel(b *testing.B) {
	config.SetMinLevel(enum.LevelError)
	entry.GenerateInitialPool(1_000_000)

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Debug(ctx, "filtered-parallel")
		}
	})
}

package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
)

// Benchmark_Log_AddSourceTrue measures the hot path through entry.Log when
// addSource=true. Input shape: single Info call with no extra fields, addSource
// enabled.
//
// why: runtime.Callers adds one frame lookup per log call, which is inherently
// above the 1 µs ceiling — bench-check.sh exempts this benchmark from the hard
// ceiling and tracks it for regressions only (see BENCH_EXCLUDE_RE).
func Benchmark_Log_AddSourceTrue(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{addSource: true})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Log(enum.LevelInfo, ctx, "bench-msg", nil)
	}
}

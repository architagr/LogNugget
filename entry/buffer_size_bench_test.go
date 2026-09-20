// Package entry_test: V4-P2 dispatch buffer pool benchmark.
//
// Measures B/op after replacing the post-publish make([]byte, 0, newCap) with
// a pool.Get from dispatchBufPool. Target: B/op = 0 on the hot path (warm pool,
// no make() per call); previously ~121 B/op from the P8 make([]byte, 0, newCap).
package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/entry"
)

// BenchmarkBufferSeverance measures the allocation bytes per call on the hot
// path after the P8 change: the replacement buffer created at severance is
// sized to max(64, len(data)) instead of the fixed initBufCap (1024 B).
//
// Input shape: single-goroutine, no context fields, JSON encoder, Debug level,
// typical short structured message (~60–80 B rendered).
// Budget defended: B/op must be ≤ 64 B (pool.Get amortised to 0 on warm path);
// overall ns/op must remain < 1000 (sub-1 µs SLO per CLAUDE.md §Performance Gate).
func BenchmarkBufferSeverance(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{bucketSize: 1000})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "typical structured log message")
	}
}

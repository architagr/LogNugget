// Package entry_test: V3-P4 timestamp benchmark.
//
// Measures the allocation count and latency of the timestamp path after the
// direct AppendFormat change. The target is 0 allocs/op attributable to the
// timestamp formatting step, and an overall p99 < 1 µs.
package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/v4/entry"
)

// BenchmarkTimestamp_Direct measures the hot path with the direct
// now.AppendFormat(e.buf, snap.TimeFormat) timestamp encoding, replacing the
// former customTime.Format + AppendQuotedString (2 allocs). The target is
// 0 allocs attributable to timestamp formatting; overall p99 must remain < 1 µs.
//
// Input shape: single-goroutine, no context fields, JSON encoder, Debug level.
// Budget defended: sub-1 µs p99 per the LogNugget SLO (CLAUDE.md §Performance Gate).
func BenchmarkTimestamp_Direct(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{bucketSize: 1000})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "bench timestamp")
	}
}

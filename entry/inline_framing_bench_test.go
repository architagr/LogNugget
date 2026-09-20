package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/entry"
)

// BenchmarkLogEntry_InlineFraming_10 measures the hot path after V2-P4 inline
// framing: OpenBytes prepend + CloseBytes append + ownership transfer,
// eliminating the en.Append intermediate buffer and the PublishLog defensive
// copy. Target: allocs/op drops by ≥ 1 vs the P3 baseline.
func BenchmarkLogEntry_InlineFraming_10(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{
		ctxAppender: tenFieldAppender,
		bucketSize:  1000,
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Info(ctx, "bench p4")
	}
}

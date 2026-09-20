package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
)

type ctxBenchKey string

// BenchmarkLogEntry_CtxAppender_10 uses the zero-alloc ContextFieldsAppender
// path (V3-P7). Target: ≥ 20 fewer allocs/op than the parser path.
func BenchmarkLogEntry_CtxAppender_10(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{
		ctxAppender: func(ctx context.Context, dst []byte) []byte {
			reqID, _ := ctx.Value(ctxBenchKey("req")).(string)
			dst = append(dst, `,"req_id":"`...)
			dst = append(dst, reqID...)
			dst = append(dst, '"')
			dst = append(dst, `,"user_id":"bench-user","k1":"v1","k2":"v2","k3":"v3","k4":"v4","k5":"v5","k6":"v6","k7":"v7","k8":"v8","k9":"v9"`...)
			return dst
		},
	})

	ctx := context.WithValue(context.Background(), ctxBenchKey("req"), "req-abc")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "bench")
	}
}

// BenchmarkLogEntry_CtxFields_10 uses the V4 typed SetContextFields API — the
// API new code should use. Directly comparable to the appender benchmark above.
func BenchmarkLogEntry_CtxFields_10(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{
		ctxFields: func(ctx context.Context, f *config.CtxFields) {
			reqID, _ := ctx.Value(ctxBenchKey("req")).(string)
			f.Str("req_id", reqID)
			f.Str("user_id", "bench-user")
			f.Str("k1", "v1")
			f.Str("k2", "v2")
			f.Str("k3", "v3")
			f.Str("k4", "v4")
			f.Str("k5", "v5")
			f.Str("k6", "v6")
			f.Str("k7", "v7")
			f.Str("k8", "v8")
		},
	})

	ctx := context.WithValue(context.Background(), ctxBenchKey("req"), "req-abc")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "bench")
	}
}

// BenchmarkLogEntry_CtxParser_10 uses the legacy map[string]any parser path.
// Regression guard: alloc count must not increase, and the gap to the two
// benchmarks above is the cost the README tells callers they are paying.
func BenchmarkLogEntry_CtxParser_10(b *testing.B) {
	setupEntryBench(b, entryBenchOpts{
		ctxParser: func(ctx context.Context) map[string]any {
			return map[string]any{
				"req_id":  "req-abc",
				"user_id": "bench-user",
				"k1":      "v1", "k2": "v2", "k3": "v3",
				"k4": "v4", "k5": "v5", "k6": "v6",
				"k7": "v7", "k8": "v8", "k9": "v9",
			}
		},
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "bench")
	}
}

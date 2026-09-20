package benchmark

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/model"
)

// Benchmark_Log_Parallel measures throughput when b.N goroutines call
// the hot path concurrently with the legacy map context parser installed.
// This is the "worst supported configuration under load" number.
//
// Context threshold note: the context has 2 keys (requestID + userID).
// Recommended max is 10 values (including trace/span) — see README §Context.
func Benchmark_Log_Parallel(b *testing.B) {
	setupBench(b, benchOpts{
		ctxParser: func(ctx context.Context) map[string]any {
			return map[string]any{
				"request_id": ctx.Value(ctxKeyRequestID),
				"user_id":    ctx.Value(ctxKeyUserID),
			}
		},
	})

	ctx := benchCtx()
	field := model.LogAttr{Key: "iter", Value: 0}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Debug(ctx, "parallel bench message", field)
		}
	})
}

// Benchmark_Log_Parallel_NoCtx is the parallel hot path with no context
// fields at all — the context-free cost baseline.
func Benchmark_Log_Parallel_NoCtx(b *testing.B) {
	setupBench(b, benchOpts{})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(context.Background(), "no-ctx bench")
		}
	})
}

// Benchmark_Log_Parallel_10CtxFields measures the recommended-maximum
// context load — 10 key-value pairs (including 2 tracing fields) — through
// the raw-bytes appender API.
func Benchmark_Log_Parallel_10CtxFields(b *testing.B) {
	setupBench(b, benchOpts{
		ctxAppender: func(ctx context.Context, dst []byte) []byte {
			dst = append(dst, `,"trace_id":"abc123def456","span_id":"span789","request_id":"req-001"`...)
			dst = append(dst, `,"user_id":"usr-42","session_id":"sess-x","region":"us-east-1"`...)
			dst = append(dst, `,"service":"api-gateway","version":"1.2.3","env":"production","pod":"pod-abc"`...)
			return dst
		},
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(ctx, "10-ctx-fields bench")
		}
	})
}

// Benchmark_Log_Parallel_10CtxFields_Typed is the same 10-field context load
// expressed through the V4 typed SetContextFields API. It is the apples-to-
// apples comparison for the raw-appender benchmark above and the configuration
// the README recommends.
func Benchmark_Log_Parallel_10CtxFields_Typed(b *testing.B) {
	setupBench(b, benchOpts{
		ctxFields: func(ctx context.Context, f *config.CtxFields) {
			f.Str("trace_id", "abc123def456")
			f.Str("span_id", "span789")
			f.Str("request_id", "req-001")
			f.Str("user_id", "usr-42")
			f.Str("session_id", "sess-x")
			f.Str("region", "us-east-1")
			f.Str("service", "api-gateway")
			f.Str("version", "1.2.3")
			f.Str("env", "production")
			f.Str("pod", "pod-abc")
		},
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(ctx, "10-ctx-fields typed bench")
		}
	})
}

// Benchmark_Log_Parallel_NoCtx_Sync is the V4-P1 comparison: the same hot path
// with the ring bypassed, records delivered on the calling goroutine.
//
// The sink here is a discard writer, which is the configuration sync mode is
// meant for. Against a sink with real latency the caller would pay that
// latency directly — see config.SetSyncMode.
func Benchmark_Log_Parallel_NoCtx_Sync(b *testing.B) {
	setupBench(b, benchOpts{syncMode: true})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			entry.NewLogEntry().Info(context.Background(), "no-ctx sync bench")
		}
	})
}

// Benchmark_Log_Serial_Typed_Sync is the serial counterpart, where removing
// the ring push has no contention to amortise against.
func Benchmark_Log_Serial_Typed_Sync(b *testing.B) {
	setupBench(b, benchOpts{syncMode: true})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().
			Str("method", "GET").
			Int("status", 200).
			Info(ctx, "request handled")
	}
}

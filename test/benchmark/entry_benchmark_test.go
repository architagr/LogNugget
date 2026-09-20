// Package benchmark hosts library hot-path benchmarks.
//
// why: comparison benches against external loggers (e.g. zerolog) live under
// examples/ in nested modules so the library go.mod stays stdlib + testify
// only (NF12). T-9: Benchmark_ZeroLog removed from this file; full bench
// rewrite tracked by story 010.
//
// Every benchmark in this package must call setupBench (see
// bench_harness_test.go) instead of poking package config directly: the
// configuration is a process-global singleton and un-torn-down state leaks
// into whichever benchmark runs next.
package benchmark

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/model"
)

type ctxKey string

const (
	ctxKeyRequestID ctxKey = "requestID"
	ctxKeyUserID    ctxKey = "userID"
)

// MockWriter is a discard io.Writer used by the benchmarks to avoid I/O cost.
type MockWriter struct {
}

// Write discards p and returns its length to satisfy io.Writer.
func (e *MockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// Benchmark_Log is the serial hot path with the legacy map-based context
// parser and static env fields — the heaviest supported configuration.
//
// It is deliberately the slowest LogNugget benchmark: SetContextFieldsParser
// builds a map[string]any per call. Compare against Benchmark_Log_Serial_Typed
// for the configuration new code should use.
func Benchmark_Log(b *testing.B) {
	setupBench(b, benchOpts{
		staticParser: func() map[string]any {
			return map[string]any{
				"app_name": "lognugget",
				"version":  "1.0.0",
			}
		},
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
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Debug(ctx, "debug message that has a log message, from lognugget", field)
	}
}

// Benchmark_Log_Serial_Typed is the serial hot path in the configuration the
// README recommends: typed chain methods, no map parser, no static fields.
func Benchmark_Log_Serial_Typed(b *testing.B) {
	setupBench(b, benchOpts{})

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

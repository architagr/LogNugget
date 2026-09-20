package benchmark

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/model"
)

// Benchmark_Log_JSONEscape_SafeASCII measures the cost of logging a
// message that requires no JSON escaping (pure ASCII, no control chars).
func Benchmark_Log_JSONEscape_SafeASCII(b *testing.B) {
	setupBench(b, benchOpts{})

	ctx := context.Background()
	field := model.LogAttr{Key: "url", Value: "/api/v1/users"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Info(ctx, "safe ASCII message", field)
	}
}

// Benchmark_Log_JSONEscape_Unicode measures the cost when the message
// contains multibyte Unicode characters that must be JSON-escaped.
func Benchmark_Log_JSONEscape_Unicode(b *testing.B) {
	setupBench(b, benchOpts{})

	ctx := context.Background()
	field := model.LogAttr{Key: "user", Value: "日本語テスト "}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Info(ctx, "unicode escape bench: こんにちは", field)
	}
}

// Benchmark_Log_JSONEscape_ControlChars measures the cost when field values
// contain control characters (tab, newline) that require JSON \u-escaping.
func Benchmark_Log_JSONEscape_ControlChars(b *testing.B) {
	setupBench(b, benchOpts{})

	ctx := context.Background()
	field := model.LogAttr{Key: "body", Value: "line1\nline2\ttabbed\r\n"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Info(ctx, "control char escape bench", field)
	}
}

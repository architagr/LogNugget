// Package config_test provides the V3-P5 benchmark for AppendQuotedString.
// No build tag — bench-check.sh runs without -tags and must always pick this up.
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
)

// BenchmarkAppendQuotedString_ASCII measures AppendQuotedString on a plain
// ASCII input with a pre-allocated destination buffer. This simulates the hot
// log path where a caller reuses a buffer across events.
//
// Input shape: 19-byte ASCII string, 64-byte pre-allocated dst.
// Budget: 0 allocs/op after V3-P5 eliminates the []byte(s) conversion;
// p99 must stay < 1 µs (project SLO).
func BenchmarkAppendQuotedString_ASCII(b *testing.B) {
	dst := make([]byte, 0, 64)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst = config.AppendQuotedString(dst[:0], "hello world message")
	}
}

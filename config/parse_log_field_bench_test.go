// Package config_test contains benchmarks for AppendField (D-5).
// These files carry no build tag so that bench-check.sh (which runs
// `go test -bench=. ./...` without -tags) always picks them up.
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
)

// Benchmark_AppendField_Int measures AppendField for an integer key-value pair
// using a pre-allocated destination buffer. Simulates the hot log path where a
// caller reuses a buffer across fields.
//
// Input shape: single int64 value, 1-character key, 64-byte pre-allocated dst.
// Budget: 0 allocations; well under the 1 µs p99 SLO (NF1).
func Benchmark_AppendField_Int(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 0, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = config.AppendField(buf[:0], "n", int64(12345))
	}
}

// Benchmark_AppendField_String measures AppendField for a plain ASCII string
// value with no characters requiring RFC 8259 escaping.
//
// Input shape: 11-byte value, 1-character key, nil dst (allocation on each call).
// Budget: 3 allocations per call (key+value quoting and result []byte);
// well under the 1 µs SLO. Zero-alloc only applies to numeric inputs
// (see Test_AppendField_NoAlloc_Numerics).
func Benchmark_AppendField_String(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.AppendField(nil, "msg", "hello world")
	}
}

// Benchmark_AppendField_Float64 measures AppendField for a float64 value using
// a pre-allocated destination buffer.
//
// Input shape: float64 with 3 significant digits, 1-character key, 64-byte dst.
// Budget: 0 allocations; well under the 1 µs p99 SLO (NF1).
func Benchmark_AppendField_Float64(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 0, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = config.AppendField(buf[:0], "r", float64(3.14))
	}
}

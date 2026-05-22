// Package config_test provides benchmarks for AppendAttr and the legacy
// AppendField, defending the < 1 µs p99 SLO for Epic V2 Story P2.
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/model"
)

// BenchmarkAppendAttr_Str measures AppendAttr with a KindStr attr on a
// pre-allocated destination buffer. Input: single short key/value pair.
// Budget: p99 must stay < 1 µs (project SLO); 0 allocs expected.
func BenchmarkAppendAttr_Str(b *testing.B) {
	attr := model.Str("key", "value")
	dst := make([]byte, 0, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = config.AppendAttr(dst[:0], "key", attr)
	}
}

// BenchmarkAppendAttr_Int measures AppendAttr with a KindInt attr on a
// pre-allocated destination buffer. Input: single numeric key/value pair.
// Budget: p99 must stay < 1 µs; 0 allocs expected.
func BenchmarkAppendAttr_Int(b *testing.B) {
	attr := model.Int("n", 42)
	dst := make([]byte, 0, 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = config.AppendAttr(dst[:0], "n", attr)
	}
}

// BenchmarkAppendField_IntLegacy measures the legacy AppendField path for an
// int value (interface{} boxing). Used as the baseline comparison for
// BenchmarkAppendAttr_Int to quantify the P2 boxing-elimination gain.
// Input: single int passed as interface{}. Budget: < 1 µs.
func BenchmarkAppendField_IntLegacy(b *testing.B) {
	dst := make([]byte, 0, 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = config.AppendField(dst[:0], "n", 42)
	}
}

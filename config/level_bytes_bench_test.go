//go:build testing

package config

import (
	"testing"

	"github.com/architagr/lognugget/enum"
)

// BenchmarkAppendQuotedLevel_Named measures the pre-rendered fast path (no
// level.String() call, no allocation).
func BenchmarkAppendQuotedLevel_Named(b *testing.B) {
	dst := make([]byte, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = AppendQuotedLevel(dst[:0], enum.LevelInfo)
	}
}

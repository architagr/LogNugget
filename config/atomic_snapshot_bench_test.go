//go:build testing

package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// BenchmarkLogEntry_FilteredPath measures the atomic level-gate cost when
// the entry is below the configured minimum. Input: repeated GetAtomicMinLevel
// calls with minLevel=Error (i.e., Debug/Info/Warn are all filtered). p99
// must stay < 1 µs to satisfy the Epic V2 SLO (LLD §2.1).
func BenchmarkLogEntry_FilteredPath(b *testing.B) {
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelError)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.GetAtomicMinLevel()
	}
}

// BenchmarkLogEntry_HotPath_NoCtx measures the cost of capturing a full
// HotSnapshot with no context appender set. Input: default config after
// reset with minLevel=Debug (all events pass the gate). p99 must stay
// < 1 µs to satisfy the Epic V2 SLO (LLD §2.2).
func BenchmarkLogEntry_HotPath_NoCtx(b *testing.B) {
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.GetHotSnapshot()
	}
}

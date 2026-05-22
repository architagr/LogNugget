//go:build testing

// Package config_test provides benchmarks for the atomic preprocessor gate (V3-P1).
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/test/support"
)

// BenchmarkHasEventPreProcessors measures the hot-path cost of HasEventPreProcessors
// after the V3-P1 atomic.Bool optimisation. Input: one registered processor (the
// realistic steady state). p99 must stay <= 5 ns/op with 0 allocs/op;
// the sub-1 µs budget from CLAUDE.md gives us ample headroom.
func BenchmarkHasEventPreProcessors(b *testing.B) {
	config.TestResetConfig()
	fp := support.NewFakePreProc("p1")
	config.InitPreProcessors(fp)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = config.HasEventPreProcessors()
	}
}

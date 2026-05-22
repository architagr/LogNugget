//go:build testing

// Package config provides the V3-P3 benchmark for the atomic.Value channel
// pointer in PublishLog. The benchmark defends the < 1 µs p99 SLO for the
// hot dispatch path and specifically targets the ~50 ns saving over the prior
// configMu.RLock-based channel read (Epic V3 LLD §5.1).
package config

import (
	"testing"

	"github.com/architagr/lognugget/enum"
)

// BenchmarkPublishLog measures the end-to-end hot-path cost of PublishLog after
// the V3-P3 atomic.Value optimisation. Input: default config after TestResetConfig
// with a 100_000-capacity channel so the send never blocks during the benchmark.
// p99 budget: < 1000 ns/op (the project-wide SLO); the atomic channel load alone
// should cost < 10 ns, and the buffered channel send < 50 ns under no contention.
//
// why: the former configMu.RLock + RUnlock pair cost ~50 ns per PublishLog call.
// atomic.Value.Load is a single memory barrier with no scheduler interaction,
// bringing the load to < 5 ns/op and keeping the full send path well under
// the sub-1 µs budget (V3-P3 / LLD §5.1 / see bench-baseline.txt).
func BenchmarkPublishLog(b *testing.B) {
	// Large channel so the b.N sends never back-pressure during the run.
	SetChannelCapacity(channelCapacityMax)
	TestResetConfig()
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			PublishLog(enum.LevelInfo, []byte(`{"level":"INFO"}`))
		}
	})
}

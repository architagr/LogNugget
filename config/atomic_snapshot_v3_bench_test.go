//go:build testing

// Package config contains the V3-P2 benchmark for the atomic.Pointer[HotSnapshot]
// copy-on-write path. The benchmark defends the < 1 µs p99 SLO for the hot log
// path and specifically targets the ~120 ns saving over the prior
// configMu.RLock-based GetHotSnapshot (Epic V3 LLD §4.1).
package config

import "testing"

// BenchmarkGetHotSnapshot measures the cost of a single GetHotSnapshot call via
// the atomic.Pointer[HotSnapshot] path. Input: default config after TestResetConfig
// (no context appender, JSON encoder). The p99 budget for this call is ≤ 20 ns/op
// with 0 allocs/op — matching a single atomic.Pointer.Load plus struct dereference.
//
// why: the former configMu.RLock + struct copy path cost ~120-200 ns under
// contention. atomic.Pointer.Load is a single memory barrier, eliminating lock
// contention entirely on the read path (V3-P2 / LLD §4.1).
func BenchmarkGetHotSnapshot(b *testing.B) {
	TestResetConfig()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GetHotSnapshot()
	}
}

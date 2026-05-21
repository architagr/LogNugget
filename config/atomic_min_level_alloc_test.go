//go:build testing

// Package config_test contains the zero-allocation assertion for the atomic
// level gate introduced in Epic V2 story P1. The test is a separate file so
// it can be read in isolation during performance audits.
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// Test_GetAtomicMinLevel_ZeroAlloc asserts that GetAtomicMinLevel performs
// zero heap allocations. A non-zero allocation count means the atomic load
// has accidentally escaped to the heap, breaking the < 1 µs p99 SLO for
// the level-gate fast path.
func Test_GetAtomicMinLevel_ZeroAlloc(t *testing.T) {
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelInfo)
	allocs := testing.AllocsPerRun(100, func() {
		_ = config.GetAtomicMinLevel()
	})
	if allocs != 0 {
		t.Errorf("GetAtomicMinLevel() allocated %.0f times, want 0", allocs)
	}
}

//go:build testing

// Package entry_test provides TS-05 tests for the level-gate ordering
// and zero-alloc filtered path. These tests verify that Log() checks the
// minimum level before performing any allocations or touching the
// EventPreProcessors map.
package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// Test_LogEntry_MinLevelGate is a table-driven test covering every
// (event-level × min-level) combination. It asserts that the registered
// pre-processor hook is called if and only if event-level ≥ min-level.
//
// TS-05 acceptance criterion: level gate runs before any other work.
//
// why: not parallel — every sub-test mutates the process-wide config
// singleton. Running sub-tests concurrently would race on minLevel and
// EventPreProcessors (same rationale as Test_LogEntry_Methods in
// levels_test.go / issue #54 / story 039).
func Test_LogEntry_MinLevelGate(t *testing.T) {
	levels := []enum.LogLevel{
		enum.LevelDebug,
		enum.LevelInfo,
		enum.LevelWarn,
		enum.LevelError,
	}

	for _, minLevel := range levels {
		for _, eventLevel := range levels {
			minLevel := minLevel
			eventLevel := eventLevel

			wantCalled := eventLevel >= minLevel

			t.Run(eventLevel.String()+"_vs_min_"+minLevel.String(), func(t *testing.T) {
				// why: explicit reset before applying test-specific config
				// prevents state leakage from a preceding test that may have
				// set a different minLevel via ConfigBuilder (T-13 hazard).
				config.TestResetConfig()
				spy := support.NewFakePreProc("gate-spy")
				support.NewConfigBuilder(t).
					MinLevel(minLevel).
					Encoder(enum.EncoderJSON).
					Build()
				config.InitPreProcessors(spy)

				entry.NewLogEntry().Log(eventLevel, context.Background(), "gate-test", nil)

				// Drain: give the async dispatch goroutine time to deliver.
				// Use the same busy-loop strategy as drainSpy in levels_test.go.
				var gotCalled bool
				for i := 0; i < 500; i++ {
					if len(spy.Records()) > 0 {
						gotCalled = true
						break
					}
					done := make(chan struct{})
					go func() { close(done) }()
					<-done
				}

				if gotCalled != wantCalled {
					t.Errorf("eventLevel=%s minLevel=%s: hook called=%v, want %v",
						eventLevel, minLevel, gotCalled, wantCalled)
				}
			})
		}
	}
}

// Test_LogEntry_FilteredPath_ZeroAlloc asserts that a Log() call whose
// event level is below the configured minimum level performs zero heap
// allocations. This protects the < 100 ns / 0-alloc acceptance criterion
// for filtered events (TS-05, acceptance #4).
//
// why: every allocation on the filtered path is wasted work. The level
// gate must short-circuit before make([]string, ...) or any other
// escaping allocation.
func Test_LogEntry_FilteredPath_ZeroAlloc(t *testing.T) {
	// why: not parallel — AllocsPerRun is unreliable under concurrent GC
	// pressure; sequential execution gives the allocator a stable baseline.

	support.NewConfigBuilder(t).
		MinLevel(enum.LevelError).
		Encoder(enum.EncoderJSON).
		Build()

	spy := support.NewFakePreProc("alloc-spy")
	config.InitPreProcessors(spy)

	ctx := context.Background()

	allocs := testing.AllocsPerRun(100, func() {
		// Debug < Error: the level gate must return before any allocation.
		e := entry.NewLogEntry()
		e.Log(enum.LevelDebug, ctx, "filtered", nil)
	})

	if allocs != 0 {
		t.Errorf("filtered Log() path: got %.0f allocs, want 0 — level gate must run before any allocation", allocs)
	}
}

// Test_DefaultAddSource_False verifies that the package-level default for
// addSource is false (TS-05, acceptance #1 / D-7). The constant DefaultAddSource
// drives resetConfig, so checking it is equivalent to checking a fresh singleton.
//
// why: TestResetConfig is intentionally NOT called here. Test_DefaultAddSource_False
// runs in parallel with Test_LogEntry_Methods. If TestResetConfig ran concurrently
// with a Test_LogEntry_Methods sub-test it would wipe EventPreProcessors after
// the sub-test's spy was installed, causing drainSpy to hang (T-13 hazard / #54).
// Checking the constant avoids any config mutation during the parallel phase.
func Test_DefaultAddSource_False(t *testing.T) {
	t.Parallel()

	if config.DefaultAddSource != false {
		t.Errorf("DefaultAddSource package constant = %v, want false — zero-config deployments must not pay runtime.Callers overhead (D-7)", config.DefaultAddSource)
	}
}

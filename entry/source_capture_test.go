//go:build testing

// Package entry_test provides TS-06 tests for source-capture wiring (D-1 / F17 / SC3 / T-3).
// These tests verify that runtime.Caller is invoked when addSource=true, that the
// resulting function name appears in the "caller" JSON field, and that no caller field
// is emitted when addSource=false.
//
// Parallel-safety note: these tests mutate the process-wide config singleton and are
// therefore NOT parallel at the sub-test level. The parent does not call t.Parallel()
// either because each sub-test depends on a clean singleton state set by ConfigBuilder.
package entry_test

import (
	"context"
	"strings"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// resetAndSpyWithAddSource resets the singleton to JSON/Debug defaults with
// addSource set to v, installs a fresh spy, and returns it.
func resetAndSpyWithAddSource(t *testing.T, name string, addSource bool) *support.FakePreProc {
	t.Helper()
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		AddSource(addSource).
		Build()
	spy := support.NewFakePreProc("spy-" + name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_LogEntry_CallerCapture_WhenAddSource_True verifies SC3: when addSource=true,
// the emitted JSON contains a "caller" field whose value names this test function.
// This also validates the skip depth (TS-06 acceptance #1, T-3, ARCH-3).
//
// This test is NOT parallel — it mutates the global config singleton.
func Test_LogEntry_CallerCapture_WhenAddSource_True(t *testing.T) {
	config.TestResetConfig()
	spy := resetAndSpyWithAddSource(t, "caller-true", true)

	// The Info call is the direct caller; runtime.Caller must report this function.
	entry.NewLogEntry().Info(context.Background(), "caller-capture-test")

	raw := drainSpy(t, spy)
	got := parseLogJSON(t, raw)

	callerVal, hasCaller := got["caller"]
	if !hasCaller {
		t.Fatalf("SC3: addSource=true must produce a 'caller' field in JSON; got: %s", raw)
	}
	if callerVal == "" {
		t.Errorf("SC3: 'caller' field must be non-empty; got empty string; payload: %s", raw)
	}
	// Verify skip depth: the reported function must be this test function.
	// runtime.Caller returns the fully-qualified name, e.g.
	// "github.com/architagr/lognugget/entry_test.Test_LogEntry_CallerCapture_WhenAddSource_True".
	if !strings.Contains(callerVal, "Test_LogEntry_CallerCapture_WhenAddSource_True") {
		t.Errorf("SC3: 'caller' field %q must contain the calling function name; want 'Test_LogEntry_CallerCapture_WhenAddSource_True'; payload: %s", callerVal, raw)
	}
}

// Test_LogEntry_NoCallerWhenAddSourceFalse verifies T-3 (false branch): when
// addSource=false, no "caller" field appears in the emitted JSON (TS-06
// acceptance #2).
//
// This test is NOT parallel — it mutates the global config singleton.
func Test_LogEntry_NoCallerWhenAddSourceFalse(t *testing.T) {
	config.TestResetConfig()
	spy := resetAndSpyWithAddSource(t, "caller-false", false)

	entry.NewLogEntry().Info(context.Background(), "no-caller-test")

	raw := drainSpy(t, spy)
	got := parseLogJSON(t, raw)

	if _, hasCaller := got["caller"]; hasCaller {
		t.Errorf("T-3: addSource=false must NOT produce a 'caller' field; got: %s", raw)
	}
}

// Test_LogEntry_CallerOnUnknown verifies that when runtime.Caller returns ok=false
// the "caller" field is set to "unknown" rather than an empty string or omitted
// (TS-06 acceptance #3).
//
// This test uses the exported hook entry.LogWithSkip (if present) to force a
// pathological skip value that exhausts the call stack. Since that hook does not
// exist yet, this test calls the unexported captureCallerForTest helper via the
// white-box entry_test package — which means it must live in a file without
// external test package suffix. Because TS-06 calls for a black-box test file,
// the simplest approach is to observe the field on a log call at a skip depth
// deep enough that runtime.Caller returns ok=false. The entry package does not
// expose that parameter, so we verify the "unknown" sentinel via a compile-time
// constant: the test calls a package-level function that triggers the unknown path.
//
// Practical approach: we expose a test-only helper entry.LogWithBadSkip (build
// tag testing) that calls Log with an artificially large skip so runtime.Caller
// returns ok=false. If that function does not exist, this test fails with a
// compile error guiding the implementer to add it.
//
// This test is NOT parallel — it mutates the global config singleton.
func Test_LogEntry_CallerOnUnknown(t *testing.T) {
	config.TestResetConfig()
	spy := resetAndSpyWithAddSource(t, "caller-unknown", true)

	// LogWithBadSkip is a test-only entry-point (build tag `testing`) that
	// calls entry.NewLogEntry().Log(...) after setting an artificially large
	// runtime.Caller skip depth, causing ok=false and the "unknown" sentinel.
	entry.LogWithBadSkip(context.Background(), "unknown-caller-test")

	raw := drainSpy(t, spy)
	got := parseLogJSON(t, raw)

	callerVal, hasCaller := got["caller"]
	if !hasCaller {
		t.Fatalf("addSource=true with ok=false must still emit 'caller' field; got: %s", raw)
	}
	if callerVal != "unknown" {
		t.Errorf("caller field = %q, want %q when runtime.Caller returns ok=false; payload: %s", callerVal, "unknown", raw)
	}
}

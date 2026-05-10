//go:build testing

package entry

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/architagr/lognugget/config"
)

// nonZeroFrame returns a pointer to a runtime.Frame with at least one
// non-zero field so that the LogEntry.caller field is provably non-zero
// before reset() is called.
func nonZeroFrame() *runtime.Frame {
	return &runtime.Frame{Function: "pkg.SomeFunc"}
}

// Test_LogEntry_ResetExhaustive verifies that reset() zeroes every field of
// LogEntry by inspecting every struct field via reflect after a call to reset().
// Any field that remains non-zero after reset() causes the test to fail.
//
// why: D-18 — manually listing fields in reset() means that new fields added
// later silently survive pool reuse, leaking state between callers. This
// reflect-based check catches any future field that is not cleared without
// requiring a manual update to the test.
func Test_LogEntry_ResetExhaustive(t *testing.T) {
	t.Parallel()

	t.Cleanup(func() { config.TestResetConfig() })

	// Construct an entry directly (white-box: same package) and force all
	// known fields to non-zero values so that a no-op reset() is caught.
	e := &LogEntry{
		caller: nonZeroFrame(),
	}

	e.reset()

	v := reflect.ValueOf(*e)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.IsZero() {
			t.Errorf("field %q is non-zero after reset(); reset() must zero every field of LogEntry", typ.Field(i).Name)
		}
	}
}

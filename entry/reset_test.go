//go:build testing

package entry

import (
	"reflect"
	"runtime"
	"testing"
)

// nonZeroFrame returns a pointer to a runtime.Frame with at least one
// non-zero field so that the LogEntry.caller field is provably non-zero
// before reset() is called.
func nonZeroFrame() *runtime.Frame {
	return &runtime.Frame{Function: "pkg.SomeFunc"}
}

// bufField is the name of the intentionally-retained field in LogEntry.reset().
// Listed here so the exhaustive test can skip it by name without hard-coding
// an index that could silently drift if the struct layout changes.
const bufField = "buf"

// Test_LogEntry_ResetExhaustive verifies that reset() zeroes every field of
// LogEntry by inspecting every struct field via reflect after a call to reset().
// Any field that remains non-zero after reset() causes the test to fail.
//
// Exception: the "buf" field is intentionally retained with len=0 (capacity
// preserved) so the backing array survives pool cycles (D-6 / F27). This test
// verifies buf is empty (len==0) rather than nil (IsZero), and all other fields
// are fully zeroed.
func Test_LogEntry_ResetExhaustive(t *testing.T) {
	t.Parallel()

	// No config.TestResetConfig cleanup here: this test does not modify the
	// config singleton, and registering a cleanup that resets it races with
	// concurrent parallel tests that rely on the singleton (issue #54).

	// Construct an entry directly (white-box: same package) and force all
	// known fields to non-zero values so that a no-op reset() is caught.
	e := &LogEntry{
		caller: nonZeroFrame(),
		buf:    make([]byte, 10, 1024),
	}

	e.reset()

	v := reflect.ValueOf(*e)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		name := typ.Field(i).Name
		if name == bufField {
			// buf must be empty (len=0) but may retain backing capacity.
			if field.Len() != 0 {
				t.Errorf("field %q has len=%d after reset(); must be 0", name, field.Len())
			}
			continue
		}
		if !field.IsZero() {
			t.Errorf("field %q is non-zero after reset(); reset() must zero every field of LogEntry", name)
		}
	}
}

// Test_LogEntry_BufResetSizeZero verifies that reset() sets buf to len=0
// while preserving the original backing capacity (D-6 / F27).
func Test_LogEntry_BufResetSizeZero(t *testing.T) {
	t.Parallel()

	e := &LogEntry{buf: make([]byte, 0, initBufCap)}
	e.buf = append(e.buf, []byte("hello world")...)
	prevCap := cap(e.buf)

	e.reset()

	if len(e.buf) != 0 {
		t.Errorf("buf len = %d after reset; want 0", len(e.buf))
	}
	if cap(e.buf) != prevCap {
		t.Errorf("buf cap = %d after reset; want %d (capacity must be preserved)", cap(e.buf), prevCap)
	}
}

// Test_LogEntry_BufRetainedAcrossPoolCycle verifies that Get/Put/Get reuses
// the same backing array (cap preserved). This is a best-effort check: the
// runtime may GC the first entry between Put and Get on a lightly-loaded
// machine, so we assert cap >= initBufCap rather than pointer equality.
func Test_LogEntry_BufRetainedAcrossPoolCycle(t *testing.T) {
	t.Parallel()

	e1 := NewLogEntry()
	// Grow buf so the cap is observable.
	e1.buf = append(e1.buf, make([]byte, 128)...)
	capBefore := cap(e1.buf)
	e1.Put()

	e2 := NewLogEntry()
	defer e2.Put()

	if cap(e2.buf) < capBefore {
		t.Errorf("buf cap = %d after pool cycle; want >= %d (backing array not retained)", cap(e2.buf), capBefore)
	}
	if len(e2.buf) != 0 {
		t.Errorf("buf len = %d after pool Get; want 0 (reset not applied)", len(e2.buf))
	}
}

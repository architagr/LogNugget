// Package customTime provides time utilities for LogNugget log formatting.
package customTime

import (
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
)

// Test_TimeNow_UTC verifies that Now() always returns a time value in
// the UTC location, regardless of the local system timezone.
func Test_TimeNow_UTC(t *testing.T) {
	t.Parallel()

	got := TimeNow()
	if got.Location() != time.UTC {
		t.Errorf("TimeNow() location = %v, want UTC", got.Location())
	}
}

// Test_AppendFormat_NoAlloc verifies that AppendFormat performs zero heap
// allocations per call, which is the key property that makes it safe for
// the hot log-formatting path.
func Test_AppendFormat_NoAlloc(t *testing.T) {
	t.Parallel()

	dst := make([]byte, 0, 64)
	fixed := time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)

	allocs := testing.AllocsPerRun(100, func() {
		dst = AppendFormat(dst[:0], fixed, time.RFC3339)
	})

	if allocs != 0 {
		t.Errorf("AppendFormat allocs = %v, want 0", allocs)
	}
}

// Test_DefaultTimeFormat_RFC3339 verifies that the package-level default
// time format constant is set to time.RFC3339, not RFC3339Nano or RFC822.
func Test_DefaultTimeFormat_RFC3339(t *testing.T) {
	t.Parallel()

	if config.DefaultTimeFormat != time.RFC3339 {
		t.Errorf("config.DefaultTimeFormat = %q, want %q (time.RFC3339)", config.DefaultTimeFormat, time.RFC3339)
	}
}

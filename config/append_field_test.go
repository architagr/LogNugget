// Package config_test: TS-09 — verifies AppendField incurs zero allocations
// when called on a pre-grown []byte buffer.
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
)

// Test_AppendField_NoAllocOnGrownBuf verifies that AppendField does not
// allocate when the destination slice already has sufficient capacity.
// This guards the hot-path invariant from D-6 / F27 / story 021: the
// pooled LogEntry.buf must grow once (at pool construction) and then
// sustain zero allocations for all common field types on subsequent calls.
func Test_AppendField_NoAllocOnGrownBuf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		key   string
		value any
	}{
		{"string", "env", "production"},
		{"int", "code", 200},
		{"int64", "ts", int64(1_700_000_000)},
		{"float64", "lat", 3.14},
		{"bool_true", "ok", true},
		{"bool_false", "ok", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			buf := make([]byte, 0, 512)
			allocs := testing.AllocsPerRun(100, func() {
				buf = config.AppendField(buf[:0], tc.key, tc.value)
			})
			if allocs != 0 {
				t.Errorf("AppendField(%q, %T) = %.0f allocs; want 0", tc.key, tc.value, allocs)
			}
		})
	}
}

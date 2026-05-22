//go:build testing

package config

import (
	"testing"

	"github.com/architagr/lognugget/enum"
)

func Test_AppendQuotedLevel_NamedLevels(t *testing.T) {
	cases := []struct {
		level enum.LogLevel
		want  string
	}{
		{enum.LevelDebug, `"DEBUG"`},
		{enum.LevelInfo, `"INFO"`},
		{enum.LevelWarn, `"WARN"`},
		{enum.LevelError, `"ERROR"`},
		{enum.LevelFatal, `"FATAL"`},
	}
	for _, c := range cases {
		got := string(AppendQuotedLevel(nil, c.level))
		if got != c.want {
			t.Errorf("level %v: got %s, want %s", c.level, got, c.want)
		}
	}
}

// Test_AppendQuotedLevel_NoAlloc verifies named levels produce no heap allocations
// (they append pre-quoted byte literals, bypassing level.String()).
func Test_AppendQuotedLevel_NoAlloc(t *testing.T) {
	named := []enum.LogLevel{
		enum.LevelDebug, enum.LevelInfo, enum.LevelWarn, enum.LevelError, enum.LevelFatal,
	}
	dst := make([]byte, 0, 16)
	for _, l := range named {
		allocs := testing.AllocsPerRun(100, func() {
			dst = AppendQuotedLevel(dst[:0], l)
		})
		if allocs != 0 {
			t.Errorf("level %v: got %.0f allocs, want 0", l, allocs)
		}
	}
}

// Test_AppendQuotedLevel_UnknownLevel verifies that a level value not in the
// switch falls back to AppendQuotedString without panicking and produces a
// valid quoted JSON string.
func Test_AppendQuotedLevel_UnknownLevel(t *testing.T) {
	unknown := enum.LogLevel(99)
	got := string(AppendQuotedLevel(nil, unknown))
	// Must be a quoted, non-empty string — exact value depends on LogLevel.String().
	if len(got) < 2 || got[0] != '"' || got[len(got)-1] != '"' {
		t.Errorf("unknown level fallback produced invalid JSON string: %s", got)
	}
}

// Test_AppendQuotedLevel_AppendToExisting verifies AppendQuotedLevel correctly
// appends to a non-nil dst rather than overwriting it.
func Test_AppendQuotedLevel_AppendToExisting(t *testing.T) {
	prefix := []byte(`"level":`)
	got := string(AppendQuotedLevel(append([]byte(nil), prefix...), enum.LevelInfo))
	want := `"level":"INFO"`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

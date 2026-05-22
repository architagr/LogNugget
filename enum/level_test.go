//go:build testing

package enum

import "testing"

func Test_LogLevel_String(t *testing.T) {
	cases := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LevelError + 1, "ERROR+1"}, // default branch
	}
	for _, tc := range cases {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("LogLevel(%d).String() = %q; want %q", tc.level, got, tc.want)
		}
	}
}

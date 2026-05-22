//go:build testing

// Package config: unit tests for the string-native appendJSONStringStr helper
// (V3-P5). These tests are in package config (white-box) because they access
// the unexported appendJSONStringStr and appendJSONString functions directly to
// assert identical output without duplicating the escape logic.
package config

import (
	"testing"
)

// Test_AppendJSONStringStr_MatchesByteVariant verifies that appendJSONStringStr
// produces identical output to appendJSONString for every input category:
// empty, plain ASCII, quote/backslash, tab/newline, multi-byte Unicode, low
// control characters, and invalid UTF-8 bytes. This is the primary correctness
// contract — the string variant must be a zero-allocation equivalent of the
// []byte variant.
func Test_AppendJSONStringStr_MatchesByteVariant(t *testing.T) {
	inputs := []string{
		"",
		"hello",
		"with \"quotes\"",
		"with\\backslash",
		"tab\there",
		"newline\nhere",
		"unicode: é中文",
		"control\x01\x02\x1f",
		string([]byte{0xff, 0xfe}), // invalid UTF-8
	}
	for _, s := range inputs {
		got := appendJSONStringStr(nil, s)
		want := appendJSONString(nil, []byte(s))
		if string(got) != string(want) {
			t.Errorf("input %q: got %q, want %q", s, got, want)
		}
	}
}

// Test_AppendJSONStringStr_EmptyString verifies that the empty string produces
// exactly two double-quote characters — the minimal valid JSON string.
func Test_AppendJSONStringStr_EmptyString(t *testing.T) {
	got := appendJSONStringStr(nil, "")
	if string(got) != `""` {
		t.Errorf("got %q, want %q", got, `""`)
	}
}

// Test_AppendJSONStringStr_ControlChars verifies RFC 8259 single-character
// escape sequences: \t, \n, \r, \", and \\. These are the most frequently
// occurring escapes in log strings and must be produced exactly.
func Test_AppendJSONStringStr_ControlChars(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\t", `"\t"`},
		{"\n", `"\n"`},
		{"\r", `"\r"`},
		{"\"", `"\""`},
		{"\\", `"\\"`},
	}
	for _, c := range cases {
		got := string(appendJSONStringStr(nil, c.in))
		if got != c.want {
			t.Errorf("input %q: got %s, want %s", c.in, got, c.want)
		}
	}
}

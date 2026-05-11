package encoder

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func Test_JSONEncoder_NoEmbeddedNilEncoder(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	v := reflect.ValueOf(enc)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.NumField() != 0 {
		t.Fatalf("JSONEncoder must have zero fields after D-17; got %d", v.NumField())
	}
}

func Test_JSONEncoder_Append_WrapsInBraces(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	got := enc.Append(nil, []byte("key:value"))
	if string(got) != "{key:value}\n" {
		t.Fatalf("got %q, want %q", string(got), "{key:value}\n")
	}
}

// Test_JSONEncoder_Escape covers TS-16: escape table handles every RFC 8259 case.
func Test_JSONEncoder_Escape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"double quote", []byte(`"`), `\"`},
		{"backslash", []byte(`\`), `\\`},
		{"backspace", []byte{'\b'}, `\b`},
		{"formfeed", []byte{'\f'}, `\f`},
		{"newline", []byte{'\n'}, `\n`},
		{"carriage return", []byte{'\r'}, `\r`},
		{"tab", []byte{'\t'}, `\t`},
		{"null byte U+0000", []byte{0x00}, "\\u0000"},
		{"control U+0001", []byte{0x01}, "\\u0001"},
		{"control U+001F", []byte{0x1f}, "\\u001f"},
		{"ascii printable", []byte("hello"), "hello"},
		{"valid multibyte UTF-8", []byte("\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e"), "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e"},
		{"invalid UTF-8 single byte", []byte{0xff}, "\xef\xbf\xbd"},
		{"mixed", []byte("say \"hi\"\n"), "say \\\"hi\\\"\\n"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := escapeJSON(nil, tc.in)
			if string(got) != tc.want {
				t.Fatalf("escapeJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Test_JSONEncoder_Numerics_Unquoted verifies digit bytes pass through unchanged.
func Test_JSONEncoder_Numerics_Unquoted(t *testing.T) {
	t.Parallel()

	cases := []string{"42", "3.14", "-1", "1e10", "0"}
	for _, tc := range cases {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			got := escapeJSON(nil, []byte(tc))
			if string(got) != tc {
				t.Fatalf("escapeJSON(%q) = %q; numerics must pass through unchanged", tc, got)
			}
		})
	}
}

// Test_JSONEncoder_UnicodeGolden compares escapeJSON output against the golden file.
func Test_JSONEncoder_UnicodeGolden(t *testing.T) {
	t.Parallel()

	// Explicit byte slice avoids NUL in source literal.
	input := []byte{'"', 'h', 'e', 'l', 'l', 'o', '"', '\\', '\n', '\t', '\b', '\f', '\r', 0x00, 0x1f}
	got := escapeJSON(nil, input)

	golden := filepath.Join("..", "testdata", "golden", "unicode_escape.json")
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

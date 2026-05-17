//go:build testing

package encoder

import (
	"testing"
)

// Test_Encoder_Iface_Compliance verifies that both JSONEncoder and TextEncoder
// satisfy the Encoder interface at compile time (via the var _ checks at the
// bottom of their respective files) and that Name() returns a non-empty string
// for each.
func Test_Encoder_Iface_Compliance(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		enc  Encoder
	}{
		{"JSONEncoder", NewJSONEncoder()},
		{"TextEncoder", NewTextEncoder()},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.enc.Name(); got == "" {
				t.Fatalf("%s.Name() returned empty string", tc.name)
			}
		})
	}
}

// Test_TextEncoder_Append_AddsNewline verifies that TextEncoder.Append returns
// body with a trailing newline appended to dst — ARCH-14 newline-framing
// contract.
func Test_TextEncoder_Append_AddsNewline(t *testing.T) {
	t.Parallel()

	enc := NewTextEncoder()
	got := enc.Append(nil, []byte("hello"))
	want := "hello\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", string(got), want)
	}
}

// Test_JSONEncoder_Append_WrapsAndNewline verifies that JSONEncoder.Append
// wraps body in JSON object braces and appends a newline — ARCH-14
// newline-framing contract.
func Test_JSONEncoder_Append_WrapsAndNewline(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	got := enc.Append(nil, []byte(`"key":"val"`))
	want := "{\"key\":\"val\"}\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", string(got), want)
	}
}

package encoder

import (
	"errors"
	"testing"

	"github.com/architagr/lognugget/enum"
)

// Test_DefaultEncoderFactory_KnownTypes covers TS-15: JSON and Text
// resolve to non-nil encoders that honor the F7 Write contract.
func Test_DefaultEncoderFactory_KnownTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   enum.LogEncodeType
		want string
	}{
		{"json wraps", enum.EncoderJSON, "{a:1}"},
		{"text passthrough", enum.EncoderText, "a:1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enc, err := DefaultEncoderFactory(tc.in)
			if err != nil {
				t.Fatalf("factory err: %v", err)
			}
			if enc == nil {
				t.Fatal("factory returned nil encoder")
			}
			out, err := enc.Write("a:1")
			if err != nil {
				t.Fatalf("write err: %v", err)
			}
			if string(out) != tc.want {
				t.Fatalf("got %q want %q", string(out), tc.want)
			}
		})
	}
}

// Test_DefaultEncoderFactory_UnknownReturnsErr covers TS-18 partial:
// the current factory rejects unknown types with ErrUnsupportedEncoderType.
// why: story 008 spec line references "fallback to JSON" (D-10 doc ack) but
// the actual code surface returns an error. ARCH-2/policy change is out of
// scope here; this test pins current behavior so a future fallback flip is
// a deliberate, reviewed change.
func Test_DefaultEncoderFactory_UnknownReturnsErr(t *testing.T) {
	t.Parallel()

	enc, err := DefaultEncoderFactory(enum.LogEncodeType("bogus"))
	if !errors.Is(err, ErrUnsupportedEncoderType) {
		t.Fatalf("want ErrUnsupportedEncoderType, got %v", err)
	}
	if enc != nil {
		t.Fatalf("want nil encoder on unknown, got %T", enc)
	}
}

package encoder

import (
	"errors"
	"testing"

	"github.com/architagr/lognugget/enum"
)

func Test_DefaultEncoderFactory_KnownTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   enum.LogEncodeType
		want string
	}{
		{"json wraps", enum.EncoderJSON, "{a:1}\n"},
		{"text passthrough", enum.EncoderText, "a:1\n"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enc := DefaultEncoderFactory(tc.in)
			if enc == nil {
				t.Fatal("factory returned nil encoder")
			}
			out := enc.Append(nil, []byte("a:1"))
			if string(out) != tc.want {
				t.Fatalf("got %q want %q", string(out), tc.want)
			}
		})
	}
}

// Test_DefaultEncoderFactory_GracefulFallback_DocCheck verifies that the
// graceful variant returns a JSON encoder for unknown types (D-10 fallback).
func Test_DefaultEncoderFactory_GracefulFallback_DocCheck(t *testing.T) {
	t.Parallel()

	enc := DefaultEncoderFactory(enum.LogEncodeType("bogus"))
	if enc == nil {
		t.Fatal("graceful factory must not return nil")
	}
	if enc.Name() != "json" {
		t.Fatalf("graceful fallback must return JSON encoder, got %q", enc.Name())
	}
}

// Test_DefaultEncoderFactoryE_UnknownType_ReturnsError verifies that the
// error variant surfaces ErrUnknownEncoder for unrecognised types.
func Test_DefaultEncoderFactoryE_UnknownType_ReturnsError(t *testing.T) {
	t.Parallel()

	enc, err := DefaultEncoderFactoryE(enum.LogEncodeType("bogus"))
	if !errors.Is(err, ErrUnknownEncoder) {
		t.Fatalf("want ErrUnknownEncoder, got %v", err)
	}
	if enc != nil {
		t.Fatalf("want nil encoder on unknown, got %T", enc)
	}
}

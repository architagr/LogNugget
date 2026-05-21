package encoder_test

import (
	"testing"

	"github.com/architagr/lognugget/encoder"
	"github.com/stretchr/testify/assert"
)

func Test_JSONEncoder_OpenCloseBytes(t *testing.T) {
	t.Parallel()
	enc := encoder.NewJSONEncoder()
	assert.Equal(t, []byte{'{'}, enc.OpenBytes(), "OpenBytes must return opening brace")
	assert.Equal(t, []byte{'}', '\n'}, enc.CloseBytes(), "CloseBytes must return closing brace + newline")
}

func Test_TextEncoder_OpenCloseBytes(t *testing.T) {
	t.Parallel()
	enc := encoder.NewTextEncoder()
	assert.Nil(t, enc.OpenBytes(), "TextEncoder OpenBytes must return nil")
	assert.Equal(t, []byte{'\n'}, enc.CloseBytes(), "TextEncoder CloseBytes must return newline")
}

// Test_NoopFramer_Defaults verifies the NoopFramer embeddable struct satisfies
// the framing contract: OpenBytes nil (no opener), CloseBytes "\n" (newline terminator).
// Third-party encoder implementations can embed NoopFramer to satisfy the
// Encoder interface without writing OpenBytes/CloseBytes from scratch.
func Test_NoopFramer_Defaults(t *testing.T) {
	t.Parallel()
	var nf encoder.NoopFramer
	assert.Nil(t, nf.OpenBytes(), "NoopFramer.OpenBytes must return nil")
	assert.Equal(t, []byte{'\n'}, nf.CloseBytes(), "NoopFramer.CloseBytes must return newline")
}

func Test_Append_BackwardCompat(t *testing.T) {
	t.Parallel()
	t.Run("json", func(t *testing.T) {
		t.Parallel()
		enc := encoder.NewJSONEncoder()
		got := enc.Append(nil, []byte("k:v"))
		assert.Equal(t, "{k:v}\n", string(got), "JSON Append must wrap body in braces + newline")
	})
	t.Run("text", func(t *testing.T) {
		t.Parallel()
		enc := encoder.NewTextEncoder()
		got := enc.Append(nil, []byte("k=v"))
		assert.Equal(t, "k=v\n", string(got), "Text Append must append newline only")
	})
}

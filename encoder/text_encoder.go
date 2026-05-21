package encoder

// compile-time proof that TextEncoder satisfies the Encoder interface.
var _ Encoder = (*TextEncoder)(nil)

// TextEncoder passes the pre-rendered log body through unchanged and appends a
// trailing newline (ARCH-14). It holds no state; all methods are safe for
// concurrent use.
type TextEncoder struct{}

// NewTextEncoder returns a new TextEncoder that satisfies the Encoder
// interface.
func NewTextEncoder() Encoder {
	return &TextEncoder{}
}

// Append appends body followed by a newline to dst and returns the result.
// dst may be nil; a new slice is allocated in that case.
func (e *TextEncoder) Append(dst, body []byte) []byte {
	dst = append(dst, body...)
	dst = append(dst, '\n')
	return dst
}

// Name returns "text", the stable identifier for this encoder.
func (e *TextEncoder) Name() string {
	return "text"
}

// closeBytesText is the constant line-terminator for a text log record.
// why: package-level var avoids allocating a new slice on every CloseBytes call.
var closeBytesText = []byte{'\n'}

// OpenBytes returns nil for the text encoder because text records have no
// opening delimiter.
func (e *TextEncoder) OpenBytes() []byte { return nil }

// CloseBytes returns `\n`, the newline that terminates a text log record.
// The returned slice is immutable; callers must not modify it.
func (e *TextEncoder) CloseBytes() []byte { return closeBytesText }

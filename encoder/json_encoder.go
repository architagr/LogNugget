package encoder

import "unicode/utf8"

// compile-time proof that JSONEncoder satisfies the Encoder interface.
var _ Encoder = (*JSONEncoder)(nil)

// jsonEscapeTable maps each byte value to an escape class.
// 0 = no escape; values 1-8 map to specific RFC 8259 sequences.
var jsonEscapeTable [256]uint8

const (
	escHex  uint8 = 1 // \uXXXX (control chars U+0000–U+001F)
	escQuot uint8 = 2 // \"
	escBksl uint8 = 3 // \\
	escB    uint8 = 4 // \b
	escF    uint8 = 5 // \f
	escN    uint8 = 6 // \n
	escR    uint8 = 7 // \r
	escT    uint8 = 8 // \t
)

var hexDigits = "0123456789abcdef"

func init() {
	for i := 0; i <= 0x1f; i++ {
		jsonEscapeTable[i] = escHex
	}
	jsonEscapeTable['"'] = escQuot
	jsonEscapeTable['\\'] = escBksl
	jsonEscapeTable['\b'] = escB
	jsonEscapeTable['\f'] = escF
	jsonEscapeTable['\n'] = escN
	jsonEscapeTable['\r'] = escR
	jsonEscapeTable['\t'] = escT
}

// escapeJSON appends src to dst with RFC 8259 string escaping applied.
// Invalid UTF-8 bytes are replaced with the Unicode replacement character (U+FFFD).
// dst may be nil; a new slice is allocated as needed.
func escapeJSON(dst, src []byte) []byte {
	for i := 0; i < len(src); {
		b := src[i]
		if b >= utf8.RuneSelf {
			r, size := utf8.DecodeRune(src[i:])
			if r == utf8.RuneError && size == 1 {
				dst = append(dst, '\xef', '\xbf', '\xbd') // U+FFFD replacement char
				i++
				continue
			}
			dst = append(dst, src[i:i+size]...)
			i += size
			continue
		}
		switch jsonEscapeTable[b] {
		case 0:
			dst = append(dst, b)
		case escHex:
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
		case escQuot:
			dst = append(dst, '\\', '"')
		case escBksl:
			dst = append(dst, '\\', '\\')
		case escB:
			dst = append(dst, '\\', 'b')
		case escF:
			dst = append(dst, '\\', 'f')
		case escN:
			dst = append(dst, '\\', 'n')
		case escR:
			dst = append(dst, '\\', 'r')
		case escT:
			dst = append(dst, '\\', 't')
		}
		i++
	}
	return dst
}

// JSONEncoder wraps a pre-rendered log body in JSON object braces and appends
// a trailing newline. It holds no state; all methods are safe for concurrent use.
type JSONEncoder struct{}

// NewJSONEncoder returns a new JSONEncoder that satisfies the Encoder interface.
func NewJSONEncoder() Encoder {
	return &JSONEncoder{}
}

// Append wraps body as `{<body>}\n` and appends the result to dst.
// dst may be nil; a new slice is allocated in that case.
func (e *JSONEncoder) Append(dst, body []byte) []byte {
	dst = append(dst, '{')
	dst = append(dst, body...)
	dst = append(dst, '}', '\n')
	return dst
}

// Name returns "json", the stable identifier for this encoder.
func (e *JSONEncoder) Name() string {
	return "json"
}

// openBytesJSON is the constant opening byte for a JSON log record.
// why: package-level var avoids allocating a new slice on every OpenBytes call.
var openBytesJSON = []byte{'{'}

// closeBytesJSON is the constant closing bytes for a JSON log record.
// why: package-level var avoids allocating a new slice on every CloseBytes call.
var closeBytesJSON = []byte{'}', '\n'}

// OpenBytes returns the single `{` byte that opens a JSON object.
// The returned slice is immutable; callers must not modify it.
func (e *JSONEncoder) OpenBytes() []byte { return openBytesJSON }

// CloseBytes returns `}\n`, the bytes that close a JSON object and terminate
// the log line. The returned slice is immutable; callers must not modify it.
func (e *JSONEncoder) CloseBytes() []byte { return closeBytesJSON }

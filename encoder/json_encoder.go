package encoder

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

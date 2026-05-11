package encoder

// compile-time proof that JSONEncoder satisfies the Encoder interface.
var _ Encoder = (*JSONEncoder)(nil)

// JSONEncoder wraps a pre-rendered log body in JSON object braces and appends
// a trailing newline. It holds no state; all methods are safe for concurrent
// use.
type JSONEncoder struct{}

// NewJSONEncoder returns a new JSONEncoder that satisfies the Encoder
// interface.
func NewJSONEncoder() Encoder {
	return &JSONEncoder{}
}

// Append wraps body as `{<body>}\n` and appends the result to dst.
// dst may be nil; a new slice is allocated in that case.
//
// why: braces are prepended/appended inline rather than via fmt.Sprintf to
// avoid an extra allocation on the hot path (F10 field escaping arrives in
// story 016 — only framing is handled here).
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

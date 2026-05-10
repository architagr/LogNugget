package encoder

// NewJSONEncoder returns a JSON encoder that satisfies the F7 Write contract.
// why: D-17 removed the unused *json.Encoder field; struct is now empty so
// allocation cost on the hot path is zero.
func NewJSONEncoder() Encoder {
	return &JSONEncoder{}
}

// JSONEncoder wraps an entry's pre-formatted body in JSON object braces.
// It holds no state; methods are safe for concurrent use.
type JSONEncoder struct{}

// Write returns entryData wrapped in "{...}". The F7 contract guarantees a
// nil error for the in-process path; the signature retains error for ARCH-2
// (story 015) when sinks may surface I/O failures.
func (e *JSONEncoder) Write(entryData string) ([]byte, error) {
	return []byte("{" + entryData + "}"), nil
}

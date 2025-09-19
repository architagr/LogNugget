package encoder

func NewTextEncoder() Encoder {
	// Implementation of Text encoder
	return &TextEncoder{}
}

type TextEncoder struct{}

func (e *TextEncoder) Write(entryData string) ([]byte, error) {
	// Implementation of Text encoding logic
	return []byte(entryData), nil
}

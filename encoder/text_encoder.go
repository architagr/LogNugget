package encoder

func NewTextEncoder() Encoder {
	// Implementation of Text encoder
	return &TextEncoder{}
}

type TextEncoder struct{}

func (e *TextEncoder) Write(entryData map[string]any) ([]byte, error) {
	// Implementation of Text encoding logic
	return nil, nil
}

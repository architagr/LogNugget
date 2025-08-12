package encoder

import "github.com/architagr/lognugget/entry"

func NewTextEncoder() Encoder {
	// Implementation of Text encoder
	return &TextEncoder{}
}

type TextEncoder struct{}

func (e *TextEncoder) Write(entry entry.LogEntry) ([]byte, error) {
	// Implementation of Text encoding logic
	return nil, nil
}

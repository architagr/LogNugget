package encoder

import (
	"encoding/json"

	"github.com/architagr/lognugget/entry"
)

func NewJSONEncoder() Encoder {
	// Implementation of JSON encoder
	return &JSONEncoder{
		jsonFormatter: json.NewEncoder(nil), // Replace nil with actual output writer
	}
}

type JSONEncoder struct {
	jsonFormatter *json.Encoder
}

func (e *JSONEncoder) Write(entry entry.LogEntry) ([]byte, error) {
	// Implementation of JSON encoding logic
	d, err := json.Marshal(entry.ToMap())
	if err != nil {
		return nil, err
	}
	return d, nil
}

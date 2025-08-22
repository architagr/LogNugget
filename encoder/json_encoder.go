package encoder

import (
	"encoding/json"
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

func (e *JSONEncoder) Write(entryData map[string]any) ([]byte, error) {
	// Implementation of JSON encoding logic
	d, err := json.Marshal(entryData)
	if err != nil {
		return nil, err
	}
	return d, nil
}

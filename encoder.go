package lognugget

import (
	"encoding/json"
	"errors"
)

type LogEncodeType string

const (
	EncoderJSON LogEncodeType = "json"
	EncoderText LogEncodeType = "text"
)

var (
	ErrUnsupportedEncoderType = errors.New("unsupported encoder type")
)

type Encoder interface {
	Write(LogEntry)
}

type EncoderFactory interface {
	CreateEncoder(encoderType LogEncodeType) (Encoder, error)
}
type EncoderFactoryFunc func(encoderType LogEncodeType) (Encoder, error)

func (f EncoderFactoryFunc) CreateEncoder(encoderType LogEncodeType) (Encoder, error) {
	return f(encoderType)
}
func NewEncoderFactory(f EncoderFactoryFunc) EncoderFactory {
	return f
}
func DefaultEncoderFactory(encoderType LogEncodeType) (Encoder, error) {
	switch encoderType {
	case EncoderJSON:
		return NewJSONEncoder(), nil
	case EncoderText:
		return NewTextEncoder(), nil
	default:
		return nil, ErrUnsupportedEncoderType
	}
}

func NewJSONEncoder() Encoder {
	// Implementation of JSON encoder
	return &JSONEncoder{
		jsonFormatter: json.NewEncoder(nil), // Replace nil with actual output writer
	}
}
func NewTextEncoder() Encoder {
	// Implementation of Text encoder
	return &TextEncoder{}
}

type JSONEncoder struct {
	jsonFormatter *json.Encoder
}

func (e *JSONEncoder) Write(entry LogEntry) {
	// Implementation of JSON encoding logic
	d, err := json.Marshal(entry.ToMap())
	if err != nil {
		return
	}

	defaultConfig.output.Write(d)
}

type TextEncoder struct{}

func (e *TextEncoder) Write(entry LogEntry) {
	// Implementation of Text encoding logic
}

func NewEncoderFactoryWithDefault() EncoderFactory {
	return NewEncoderFactory(DefaultEncoderFactory)
}
func NewEncoderFactoryWithFunc(f EncoderFactoryFunc) EncoderFactory {
	return NewEncoderFactory(f)
}
func NewEncoderFactoryWithDefaultFunc() EncoderFactory {
	return NewEncoderFactory(DefaultEncoderFactory)
}
func NewEncoderFactoryWithDefaultEncoderType(encoderType LogEncodeType) EncoderFactory {
	return NewEncoderFactory(func(et LogEncodeType) (Encoder, error) {
		if et == "" {
			et = encoderType
		}
		return DefaultEncoderFactory(et)
	})
}
func NewEncoderFactoryWithDefaultEncoderTypeFunc(encoderType LogEncodeType, f EncoderFactoryFunc) EncoderFactory {
	return NewEncoderFactory(func(et LogEncodeType) (Encoder, error) {
		if et == "" {
			et = encoderType
		}
		return f(et)
	})
}
func NewEncoderFactoryWithDefaultEncoderTypeAndFunc(encoderType LogEncodeType, f EncoderFactoryFunc) EncoderFactory {
	return NewEncoderFactory(func(et LogEncodeType) (Encoder, error) {
		if et == "" {
			et = encoderType
		}
		return f(et)
	})
}

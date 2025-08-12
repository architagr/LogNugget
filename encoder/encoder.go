package encoder

import (
	"errors"

	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
)

var (
	ErrUnsupportedEncoderType = errors.New("unsupported encoder type")
)

type Encoder interface {
	Write(*entry.LogEntry) ([]byte, error)
}

type EncoderFactory interface {
	CreateEncoder(encoderType enum.LogEncodeType) (Encoder, error)
}
type EncoderFactoryFunc func(encoderType enum.LogEncodeType) (Encoder, error)

func (f EncoderFactoryFunc) CreateEncoder(encoderType enum.LogEncodeType) (Encoder, error) {
	return f(encoderType)
}
func NewEncoderFactory(f EncoderFactoryFunc) EncoderFactory {
	return f
}
func DefaultEncoderFactory(encoderType enum.LogEncodeType) (Encoder, error) {
	switch encoderType {
	case enum.EncoderJSON:
		return NewJSONEncoder(), nil
	case enum.EncoderText:
		return NewTextEncoder(), nil
	default:
		return nil, ErrUnsupportedEncoderType
	}
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
func NewEncoderFactoryWithDefaultEncoderType(encoderType enum.LogEncodeType) EncoderFactory {
	return NewEncoderFactory(func(et enum.LogEncodeType) (Encoder, error) {
		if et == "" {
			et = encoderType
		}
		return DefaultEncoderFactory(et)
	})
}
func NewEncoderFactoryWithDefaultEncoderTypeFunc(encoderType enum.LogEncodeType, f EncoderFactoryFunc) EncoderFactory {
	return NewEncoderFactory(func(et enum.LogEncodeType) (Encoder, error) {
		if et == "" {
			et = encoderType
		}
		return f(et)
	})
}
func NewEncoderFactoryWithDefaultEncoderTypeAndFunc(encoderType enum.LogEncodeType, f EncoderFactoryFunc) EncoderFactory {
	return NewEncoderFactory(func(et enum.LogEncodeType) (Encoder, error) {
		if et == "" {
			et = encoderType
		}
		return f(et)
	})
}

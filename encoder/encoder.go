package encoder

import (
	"errors"

	"github.com/architagr/lognugget/enum"
)

var ErrUnsupportedEncoderType = errors.New("unsupported encoder type")

type Encoder interface {
	Write(map[string]any) ([]byte, error)
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

// Package encoder provides the Encoder interface and its implementations for
// formatting structured log entries. Each encoder is responsible for wrapping
// the pre-rendered body bytes and appending a trailing newline (ARCH-14) so
// callers never need to frame output themselves. The package exposes
// DefaultEncoderFactory as the primary entry point; direct construction via
// NewJSONEncoder and NewTextEncoder is available for testing and advanced use.
package encoder

import (
	"errors"

	"github.com/architagr/lognugget/enum"
)

// ErrUnknownEncoder is returned by DefaultEncoderFactoryE when the requested
// LogEncodeType is not recognised. Callers should use errors.Is to detect it.
var ErrUnknownEncoder = errors.New("unknown encoder type")

// Encoder formats a pre-rendered log body and appends it to dst.
//
// Append appends the formatted representation of body to dst and returns the
// extended slice. If dst is nil the implementation allocates a new slice.
// Implementations MUST append a trailing newline (ARCH-14) so callers do not
// need to frame output. Append is safe for concurrent use on a single Encoder
// value; it must not retain references to dst or body after returning.
//
// Name returns a stable, non-empty identifier for the encoder (e.g. "json",
// "text") suitable for logging and metrics labels.
//
// OpenBytes returns the constant bytes that open an encoded log record (e.g.
// `{` for JSON). The returned slice is immutable; callers must not modify it.
// P4 will use OpenBytes and CloseBytes to build streaming encoders; P1 adds
// these stubs so GetHotSnapshot can pre-fetch them once per call, outside the
// configMu lock.
//
// CloseBytes returns the constant bytes that close an encoded log record (e.g.
// `}\n` for JSON). The returned slice is immutable; callers must not modify it.
type Encoder interface {
	Append(dst, body []byte) []byte
	Name() string
	OpenBytes() []byte
	CloseBytes() []byte
}

// DefaultEncoderFactory returns the Encoder for the given LogEncodeType.
// For unknown types it falls back to the JSON encoder.
// Use DefaultEncoderFactoryE to surface an error instead.
func DefaultEncoderFactory(encoderType enum.LogEncodeType) Encoder {
	enc, err := DefaultEncoderFactoryE(encoderType)
	if err != nil {
		enc, _ = DefaultEncoderFactoryE(enum.EncoderJSON)
	}
	return enc
}

// DefaultEncoderFactoryE returns the Encoder for the given LogEncodeType.
// It returns ErrUnknownEncoder for any type not explicitly handled.
func DefaultEncoderFactoryE(encoderType enum.LogEncodeType) (Encoder, error) {
	switch encoderType {
	case enum.EncoderJSON:
		return NewJSONEncoder(), nil
	case enum.EncoderText:
		return NewTextEncoder(), nil
	default:
		return nil, ErrUnknownEncoder
	}
}

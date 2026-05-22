package enum

// LogEncodeType identifies the wire format used to serialise log entries.
// Pass it to config.SetEncoderType to switch the active encoder.
type LogEncodeType string

const (
	// EncoderJSON serialises log entries as single-line JSON objects.
	EncoderJSON LogEncodeType = "json"
	// EncoderText serialises log entries as plain text lines without JSON framing.
	EncoderText LogEncodeType = "text"
)

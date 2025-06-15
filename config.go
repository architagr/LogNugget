package lognugget

import (
	"context"
	"io"
	"os"
	"time"
)

var (
	DafaultLevel       LogLevel      = LevelInfo   // Default log level
	DafaultEncoderType LogEncodeType = EncoderJSON // Default encoder type
	DafaultAddSource   bool          = true        // Default to add source information
	DefaultOutput      io.Writer     = os.Stdout   // Default output writer
	DefaultTimeFormat  string        = time.RFC822 // Default time format for log entries
	DafaultLogBuffer   int           = 20          // Default buffer size for logs
)

type StaticEnvFieldsParser = func() map[string]any
type ContextFieldsParser = func(ctx context.Context) map[string]any

func init() {
	ResetConfig()
}

type Config struct {
	minLevel            LogLevel                 // Minimum log level to log
	encoderType         LogEncodeType            // Encoder type to use for logging
	encoder             Encoder                  // Encoder to format log entries
	addSource           bool                     // Whether to add source information to logs
	output              io.Writer                // Output writer for logs
	logBuffer           int                      // max Buffer size for logs
	rate                time.Duration            // Rate to push logs to output
	extractStaticFields StaticEnvFieldsParser    // Function to extract static environment fields
	staticEnvFields     map[string]any           // Static environment fields to log
	contextParser       ContextFieldsParser      // Function to extract context fields
	defaultFields       map[DefaultLogKey]string // Default fields to log with every entry
	timeFormat          string                   // Time format for log entries
}

var defaultConfig *Config

// SetMinLevel sets the minimum log level for the logger
func SetMinLevel(level LogLevel) {
	defaultConfig.minLevel = level
}

// SetMinLevel sets the minimum log level for the logger
func SetTimeFormat(format string) {
	defaultConfig.timeFormat = format
}

// SetEncoderType sets the encoder type for the logger
func SetEncoderType(encoderType LogEncodeType) {
	defaultConfig.encoderType = encoderType
	defaultConfig.encoder, _ = DefaultEncoderFactory(defaultConfig.encoderType)
}

// SetAddSource sets whether to add source information to logs
func SetAddSource(addSource bool) {
	defaultConfig.addSource = addSource

}

// SetOutput sets the output writer for the logger
func SetOutput(output io.Writer) {
	if output == nil {
		output = DefaultOutput
	}
	defaultConfig.output = output

}

// SetLogBuffer sets the maximum buffer size for logs
func SetLogBuffer(size int) {
	if size <= 0 {
		size = 20 // Default buffer size
	}
	defaultConfig.logBuffer = size

}

// SetRate sets the rate at which logs are pushed to output
func SetRate(rate time.Duration) {
	if rate <= 0 {
		rate = 1 * time.Second // Default rate is 1 sec
	}
	defaultConfig.rate = rate

}

// SetStaticEnvFieldsParser sets the function to extract static environment fields
func SetStaticEnvFieldsParser(parser StaticEnvFieldsParser) {
	defaultConfig.extractStaticFields = parser
	if parser != nil {
		defaultConfig.staticEnvFields = defaultConfig.extractStaticFields()
	} else {
		defaultConfig.staticEnvFields = nil
	}

}

// SetContextFieldsParser sets the function to extract context fields
func SetContextFieldsParser(parser ContextFieldsParser) {
	defaultConfig.contextParser = parser

}

// SetDefaultFields sets the default fields to log with every entry
func SetDefaultFields(fields map[DefaultLogKey]string) {
	if fields == nil {

	}
	for key, value := range fields {
		if len(string(key)) == 0 || len(value) == 0 {
			continue // Skip empty keys
		}
		defaultConfig.defaultFields[key] = value
	}

}

// GetConfig returns the current logger configuration
func GetConfig() *Config {
	return defaultConfig
}

// ResetConfig resets the logger configuration to default values
func ResetConfig() {
	defaultConfig = &Config{
		minLevel:            DafaultLevel,
		encoderType:         DafaultEncoderType,
		addSource:           DafaultAddSource,
		output:              DefaultOutput,
		logBuffer:           DafaultLogBuffer, // Default buffer size
		rate:                1 * time.Second,  // Default rate is 1 sec
		extractStaticFields: nil,
		staticEnvFields:     nil,
		contextParser:       nil,
		timeFormat:          DefaultTimeFormat,
		defaultFields: map[DefaultLogKey]string{
			DefaultLogKeyTime:          string(DefaultLogKeyTime),
			DefaultLogKeyLevel:         string(DefaultLogKeyLevel),
			DefaultLogKeyMessage:       string(DefaultLogKeyMessage),
			DefaultLogKeyError:         string(DefaultLogKeyError),
			DefaultLogKeyCaller:        string(DefaultLogKeyCaller),
			DefaultLogKeyContext:       string(DefaultLogKeyContext),
			DefaultLogKeyDuration:      string(DefaultLogKeyDuration),
			DefaultLogKeyFields:        string(DefaultLogKeyFields),
			DefaultLogKeySource:        string(DefaultLogKeySource),
			DefaultLogKeyStatic:        string(DefaultLogKeyStatic),
			DefaultLogKeyEnv:           string(DefaultLogKeyEnv),
			DefaultLogKeyHost:          string(DefaultLogKeyHost),
			DefaultLogKeyService:       string(DefaultLogKeyService),
			DefaultLogKeyVersion:       string(DefaultLogKeyVersion),
			DefaultLogKeyRequest:       string(DefaultLogKeyRequest),
			DefaultLogKeyResponse:      string(DefaultLogKeyResponse),
			DefaultLogKeyUser:          string(DefaultLogKeyUser),
			DefaultLogKeySession:       string(DefaultLogKeySession),
			DefaultLogKeyTraceID:       string(DefaultLogKeyTraceID),
			DefaultLogKeySpanID:        string(DefaultLogKeySpanID),
			DefaultLogKeyCorrelationID: string(DefaultLogKeyCorrelationID),
			DefaultLogKeyComponent:     string(DefaultLogKeyComponent),
			DefaultLogKeyOperation:     string(DefaultLogKeyOperation),
			DefaultLogKeyStatus:        string(DefaultLogKeyStatus),
			DefaultLogKeyLatency:       string(DefaultLogKeyLatency),
			DefaultLogKeyRequestID:     string(DefaultLogKeyRequestID),
			DefaultLogKeyResponseTime:  string(DefaultLogKeyResponseTime),
			DefaultLogKeyClientIP:      string(DefaultLogKeyClientIP),
			DefaultLogKeyServerIP:      string(DefaultLogKeyServerIP),
			DefaultLogKeyProtocol:      string(DefaultLogKeyProtocol),
			DefaultLogKeyMethod:        string(DefaultLogKeyMethod),
			DefaultLogKeyURL:           string(DefaultLogKeyURL),
			DefaultLogKeyStatusCode:    string(DefaultLogKeyStatusCode),
			DefaultLogKeyContentType:   string(DefaultLogKeyContentType),
			DefaultLogKeyContentLength: string(DefaultLogKeyContentLength),
			DefaultLogKeyResponseSize:  string(DefaultLogKeyResponseSize),
			DefaultLogKeyRequestSize:   string(DefaultLogKeyRequestSize),
			DefaultLogKeyUserAgent:     string(DefaultLogKeyUserAgent),
			DefaultLogKeyReferer:       string(DefaultLogKeyReferer),
			DefaultLogKeyForwardedFor:  string(DefaultLogKeyForwardedFor),
			DefaultLogKeyCustom:        string(DefaultLogKeyCustom),
		},
	}
	defaultConfig.encoder, _ = DefaultEncoderFactory(defaultConfig.encoderType)
}

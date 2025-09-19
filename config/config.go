package config

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/architagr/lognugget/encoder"
	"github.com/architagr/lognugget/enum"
)

var (
	DafaultLevel       enum.LogLevel      = enum.LevelInfo   // Default log level
	DafaultEncoderType enum.LogEncodeType = enum.EncoderJSON // Default encoder type
	DafaultAddSource   bool               = true             // Default to add source information
	DefaultOutput      io.Writer          = os.Stdout        // Default output writer
	DefaultTimeFormat  string             = time.RFC822      // Default time format for log entries
	DafaultLogBuffer   int                = 20               // Default buffer size for logs
)

type StaticEnvFieldsParser = func() map[string]any
type ContextFieldsParser = func(ctx context.Context) map[string]any

func init() {
	ResetConfig()
}

type Config struct {
	minLevel           enum.LogLevel                 // Minimum log level to log
	encoderType        enum.LogEncodeType            // Encoder type to use for logging
	encoderObj         encoder.Encoder               // encoder for the data
	addSource          bool                          // Whether to add source information to logs
	output             io.Writer                     // Output writer for logs
	logBufferMaxSize   int                           // max Buffer size for logs
	rate               time.Duration                 // Rate to push logs to output
	parsedStaticFields string                        // this is the satic fields
	contextParser      ContextFieldsParser           // Function to extract context fields
	defaultFields      map[enum.DefaultLogKey]string // Default fields to log with every entry
	timeFormat         string                        // Time format for log entries
}

var (
	defaultConfig      *Config
	EventPreProcessors map[string]preProcessingObserverContract
)

type LogEntryContract interface {
	ToMap() string
	Level() enum.LogLevel
}
type preProcessingObserverContract interface {
	PreProcess(entry LogEntryContract)
	Name() string
}

func InitPreProcessors(observers ...preProcessingObserverContract) {
	EventPreProcessors = make(map[string]preProcessingObserverContract)
	AddPreProcessors(observers...)
}

func AddPreProcessors(observers ...preProcessingObserverContract) {
	for _, observer := range observers {
		EventPreProcessors[observer.Name()] = observer
	}
}

func RemovePreProcessor(name string) {
	delete(EventPreProcessors, name)
}

// SetMinLevel sets the minimum log level for the logger
func SetMinLevel(level enum.LogLevel) {
	defaultConfig.minLevel = level
}

// SetMinLevel sets the minimum log level for the logger
func SetTimeFormat(format string) {
	defaultConfig.timeFormat = format
}

// SetEncoderType sets the encoder type for the logger
func SetEncoderType(encoderType enum.LogEncodeType) {
	var err error

	defaultConfig.encoderObj, err = encoder.DefaultEncoderFactory(encoderType)
	if err != nil {
		encoderType = enum.EncoderJSON
		defaultConfig.encoderObj, _ = encoder.DefaultEncoderFactory(encoderType)
	}

	defaultConfig.encoderType = encoderType
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

// SetLogBufferMaxSize sets the maximum buffer size for logs
func SetLogBufferMaxSize(size int) {
	if size <= 0 {
		size = 20 // Default buffer size
	}
	defaultConfig.logBufferMaxSize = size

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
	if parser != nil {
		list := []string{}
		for key, value := range parser() {
			list = Foo(list, key, value)
		}
		if len(list) > 0 {
			defaultConfig.parsedStaticFields = strings.Join(list, ", ")
		}

	} else {
		defaultConfig.parsedStaticFields = ""
	}

}

func Foo(data []string, key string, value any) []string {
	defaultFields := GetConfig().DefaultFields()
	if slices.Contains([]string{
		defaultFields[enum.DefaultLogKeyCaller],
		defaultFields[enum.DefaultLogKeyError],
		defaultFields[enum.DefaultLogKeyMessage],
		defaultFields[enum.DefaultLogKeyLevel],
		defaultFields[enum.DefaultLogKeyTime],
	}, key) {
		key = "custon." + key
	}

	data = append(data, Bar(key, value))
	return data
}

func Bar(key string, value any) string {
	return fmt.Sprintf("\"%s\": %+v", key, value)
}

// SetContextFieldsParser sets the function to extract context fields
func SetContextFieldsParser(parser ContextFieldsParser) {
	defaultConfig.contextParser = parser

}

// SetDefaultFields sets the default fields to log with every entry
func SetDefaultFields(fields map[enum.DefaultLogKey]string) {
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
	encoderObj, _ := encoder.DefaultEncoderFactory(enum.EncoderJSON)
	defaultConfig = &Config{
		minLevel:           DafaultLevel,
		encoderType:        DafaultEncoderType,
		encoderObj:         encoderObj,
		addSource:          DafaultAddSource,
		output:             DefaultOutput,
		logBufferMaxSize:   DafaultLogBuffer, // Default buffer size
		rate:               1 * time.Second,  // Default rate is 1 sec
		parsedStaticFields: "",
		contextParser:      nil,
		timeFormat:         DefaultTimeFormat,
		defaultFields: map[enum.DefaultLogKey]string{
			enum.DefaultLogKeyTime:          string(enum.DefaultLogKeyTime),
			enum.DefaultLogKeyLevel:         string(enum.DefaultLogKeyLevel),
			enum.DefaultLogKeyMessage:       string(enum.DefaultLogKeyMessage),
			enum.DefaultLogKeyError:         string(enum.DefaultLogKeyError),
			enum.DefaultLogKeyCaller:        string(enum.DefaultLogKeyCaller),
			enum.DefaultLogKeyContext:       string(enum.DefaultLogKeyContext),
			enum.DefaultLogKeyDuration:      string(enum.DefaultLogKeyDuration),
			enum.DefaultLogKeyFields:        string(enum.DefaultLogKeyFields),
			enum.DefaultLogKeySource:        string(enum.DefaultLogKeySource),
			enum.DefaultLogKeyStatic:        string(enum.DefaultLogKeyStatic),
			enum.DefaultLogKeyEnv:           string(enum.DefaultLogKeyEnv),
			enum.DefaultLogKeyHost:          string(enum.DefaultLogKeyHost),
			enum.DefaultLogKeyService:       string(enum.DefaultLogKeyService),
			enum.DefaultLogKeyVersion:       string(enum.DefaultLogKeyVersion),
			enum.DefaultLogKeyRequest:       string(enum.DefaultLogKeyRequest),
			enum.DefaultLogKeyResponse:      string(enum.DefaultLogKeyResponse),
			enum.DefaultLogKeyUser:          string(enum.DefaultLogKeyUser),
			enum.DefaultLogKeySession:       string(enum.DefaultLogKeySession),
			enum.DefaultLogKeyTraceID:       string(enum.DefaultLogKeyTraceID),
			enum.DefaultLogKeySpanID:        string(enum.DefaultLogKeySpanID),
			enum.DefaultLogKeyCorrelationID: string(enum.DefaultLogKeyCorrelationID),
			enum.DefaultLogKeyComponent:     string(enum.DefaultLogKeyComponent),
			enum.DefaultLogKeyOperation:     string(enum.DefaultLogKeyOperation),
			enum.DefaultLogKeyStatus:        string(enum.DefaultLogKeyStatus),
			enum.DefaultLogKeyLatency:       string(enum.DefaultLogKeyLatency),
			enum.DefaultLogKeyRequestID:     string(enum.DefaultLogKeyRequestID),
			enum.DefaultLogKeyResponseTime:  string(enum.DefaultLogKeyResponseTime),
			enum.DefaultLogKeyClientIP:      string(enum.DefaultLogKeyClientIP),
			enum.DefaultLogKeyServerIP:      string(enum.DefaultLogKeyServerIP),
			enum.DefaultLogKeyProtocol:      string(enum.DefaultLogKeyProtocol),
			enum.DefaultLogKeyMethod:        string(enum.DefaultLogKeyMethod),
			enum.DefaultLogKeyURL:           string(enum.DefaultLogKeyURL),
			enum.DefaultLogKeyStatusCode:    string(enum.DefaultLogKeyStatusCode),
			enum.DefaultLogKeyContentType:   string(enum.DefaultLogKeyContentType),
			enum.DefaultLogKeyContentLength: string(enum.DefaultLogKeyContentLength),
			enum.DefaultLogKeyResponseSize:  string(enum.DefaultLogKeyResponseSize),
			enum.DefaultLogKeyRequestSize:   string(enum.DefaultLogKeyRequestSize),
			enum.DefaultLogKeyUserAgent:     string(enum.DefaultLogKeyUserAgent),
			enum.DefaultLogKeyReferer:       string(enum.DefaultLogKeyReferer),
			enum.DefaultLogKeyForwardedFor:  string(enum.DefaultLogKeyForwardedFor),
			enum.DefaultLogKeyCustom:        string(enum.DefaultLogKeyCustom),
		},
	}
}

func (c *Config) MinLevel() enum.LogLevel {
	return c.minLevel
}

func (c *Config) EncoderType() enum.LogEncodeType {
	return c.encoderType
}

func (c *Config) AddSource() bool {
	return c.addSource
}

func (c *Config) Output() io.Writer {
	return c.output
}

func (c *Config) LogBuffer() int {
	return c.logBufferMaxSize
}

func (c *Config) Rate() time.Duration {
	return c.rate
}

func (c *Config) StaticFields() string {
	return c.parsedStaticFields
}

func (c *Config) ContextParser() ContextFieldsParser {
	return c.contextParser
}

func (c *Config) DefaultFields() map[enum.DefaultLogKey]string {
	return c.defaultFields
}

func (c *Config) TimeFormat() string {
	return c.timeFormat
}
func (c *Config) Encoder() encoder.Encoder {
	return c.encoderObj
}

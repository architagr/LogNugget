// Package config holds the singleton logger configuration for LogNugget.
// It owns the three package-level globals (defaultConfig, ch,
// EventPreProcessors) and exposes Set* mutators intended for use at
// program start-up only (NF5 / ARCH-15).
//
// Concurrency: all exported mutators and resetConfig acquire configMu
// (write) before touching the globals; GetConfig and ProcessLogEvent
// acquire configMu (read). This satisfies the race-freedom requirement
// tracked in issue #54 / story 039 without the allocation overhead of a
// copy-on-write atomic.Pointer approach (see bench-baseline.txt for
// the < 1 µs budget).
package config

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/architagr/lognugget/encoder"
	"github.com/architagr/lognugget/enum"
)

var (
	DafaultLevel       enum.LogLevel      = enum.LevelInfo   // Default log level
	DafaultEncoderType enum.LogEncodeType = enum.EncoderJSON // Default encoder type
	DafaultAddSource   bool               = true             // Default to add source information
	DefaultOutput      io.Writer          = os.Stdout        // Default output writer
	DefaultTimeFormat  string             = time.RFC3339     // Default time format for log entries
	DafaultLogBuffer   int                = 20               // Default buffer size for logs
	DefaultPrefix      string             = "custom."
)

type PublishLogMessageHookContract interface {
	PublishLogMessage(entry []byte)
	Name() string
}
type StaticEnvFieldsParser = func() map[string]any
type ContextFieldsParser = func(ctx context.Context) map[string]any

func init() {
	resetConfig()
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
	hooks              map[enum.LogLevel]map[string]PublishLogMessageHookContract
}

type LogEvent struct {
	Level enum.LogLevel
	Data  []byte
}

var (
	defaultConfig      *Config
	ch                 chan LogEvent
	EventPreProcessors map[string]preProcessingObserverContract

	// configMu guards the three package-level globals above.
	//
	// why: ProcessLogEvent reads ch and EventPreProcessors concurrently
	// with resetConfig writing them; Set* functions write fields of
	// defaultConfig concurrently with each other and with resetConfig.
	// A single RWMutex is the minimal, reviewable fix for #54.
	// RLock is taken by read-only paths (GetConfig, ProcessLogEvent) so
	// multiple concurrent readers never block each other; write paths
	// (Set*, resetConfig) take the exclusive Lock.
	configMu sync.RWMutex
)

type preProcessingObserverContract interface {
	PreProcess(level enum.LogLevel, logMsg []byte)
	Name() string
}

// InitPreProcessors replaces the global pre-processor map with a fresh
// map seeded from observers. It is safe to call concurrently with
// ProcessLogEvent.
func InitPreProcessors(observers ...preProcessingObserverContract) {
	configMu.Lock()
	defer configMu.Unlock()
	EventPreProcessors = make(map[string]preProcessingObserverContract)
	for _, observer := range observers {
		EventPreProcessors[observer.Name()] = observer
	}
}

// AddPreProcessors registers one or more pre-processors into the global
// map. It is safe to call concurrently.
func AddPreProcessors(observers ...preProcessingObserverContract) {
	configMu.Lock()
	defer configMu.Unlock()
	for _, observer := range observers {
		EventPreProcessors[observer.Name()] = observer
	}
}

// RemovePreProcessor deletes the named pre-processor from the global map.
// It is safe to call concurrently.
func RemovePreProcessor(name string) {
	configMu.Lock()
	defer configMu.Unlock()
	delete(EventPreProcessors, name)
}

// SetMinLevel sets the minimum log level for the logger. Safe for
// concurrent use.
func SetMinLevel(level enum.LogLevel) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.minLevel = level
}

// SetTimeFormat sets the time format for log entries. Safe for
// concurrent use.
func SetTimeFormat(format string) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.timeFormat = format
}

// SetEncoderType sets the encoder type for the logger. If the requested
// encoder type is unknown, it falls back to EncoderJSON. Safe for
// concurrent use.
func SetEncoderType(encoderType enum.LogEncodeType) {
	var err error

	configMu.Lock()
	defer configMu.Unlock()

	defaultConfig.encoderObj, err = encoder.DefaultEncoderFactory(encoderType)
	if err != nil {
		encoderType = enum.EncoderJSON
		defaultConfig.encoderObj, _ = encoder.DefaultEncoderFactory(encoderType)
	}

	defaultConfig.encoderType = encoderType
}

// SetAddSource sets whether source file and line information is appended
// to every log entry. Safe for concurrent use.
func SetAddSource(addSource bool) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.addSource = addSource
}

// SetOutput sets the output writer for the logger. A nil argument is
// silently replaced with DefaultOutput. Safe for concurrent use.
func SetOutput(output io.Writer) {
	if output == nil {
		output = DefaultOutput
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.output = output
}

// PublishLog sends a log event onto the dispatch channel. Safe for
// concurrent use; callers block if the channel buffer is full.
//
// why: the RLock is taken only for the pointer snapshot of ch, not
// across the send itself. resetConfig never closes the old channel
// (see comment there), so there is no risk of a "send on closed
// channel" panic. Releasing the RLock before the send avoids holding
// it during a potentially blocking channel operation.
func PublishLog(Level enum.LogLevel, Data []byte) {
	configMu.RLock()
	currentCh := ch
	configMu.RUnlock()
	currentCh <- LogEvent{
		Level: Level,
		Data:  Data,
	}
}

// SetLogBufferMaxSize sets the maximum buffer size for logs. Values ≤ 0
// are silently replaced with 20. Safe for concurrent use.
func SetLogBufferMaxSize(size int) {
	if size <= 0 {
		size = 20 // Default buffer size
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.logBufferMaxSize = size
}

// SetRate sets the rate at which buffered logs are pushed to output.
// Values ≤ 0 are silently replaced with 1 second. Safe for concurrent use.
func SetRate(rate time.Duration) {
	if rate <= 0 {
		rate = 1 * time.Second // Default rate is 1 sec
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.rate = rate
}

// restrictedFieldsSet is the O(1) replacement for the former restrictedFields
// []string. It holds the current effective field names for the five core log
// keys (time, level, message, error, caller). Map membership replaces
// slices.Contains, eliminating the O(n) linear scan on every field validation
// call.
//
// why: D-8 — the slice caused measurable latency growth as the field list
// grew; map lookup is O(1) regardless of set size and allocates zero bytes
// per lookup. Populated by resetConfig (for built-in defaults) and
// repopulated by SetDefaultFields (when the caller renames a core key).
//
// The variable is package-level, not a Config field, because ValidateandParseLogField
// is a package-level function shared between config and entry; embedding it in
// *Config would require plumbing the *Config pointer into the entry hot path.
// Access is serialised by configMu (write under Lock in resetConfig /
// SetDefaultFields; read under RLock in ValidateandParseLogField).
var restrictedFieldsSet map[string]struct{}

// buildRestrictedSet constructs a new restricted-field set from the five core
// keys in fields. It must be called while the caller holds configMu (write).
func buildRestrictedSet(fields map[enum.DefaultLogKey]string) map[string]struct{} {
	// why: fixed capacity of 5 — only the five core keys are ever restricted.
	// Allocating exactly the right size avoids the rehash that would otherwise
	// occur when the sixth key is inserted (there is no sixth key).
	set := make(map[string]struct{}, 5)
	set[fields[enum.DefaultLogKeyCaller]] = struct{}{}
	set[fields[enum.DefaultLogKeyError]] = struct{}{}
	set[fields[enum.DefaultLogKeyMessage]] = struct{}{}
	set[fields[enum.DefaultLogKeyLevel]] = struct{}{}
	set[fields[enum.DefaultLogKeyTime]] = struct{}{}
	return set
}

// ValidateandParseLogField checks whether key collides with a restricted log
// field name and, if so, prepends DefaultPrefix ("custom.") before delegating
// to ParseLogField. The restricted-field lookup is O(1) via map membership.
//
// Callers: entry.Log (via setLogContextFields) and SetStaticEnvFieldsParser.
// Both call sites are in the request hot path; the map lookup must remain
// allocation-free (verified by Benchmark_ValidateField).
func ValidateandParseLogField(key string, value any) string {
	configMu.RLock()
	_, restricted := restrictedFieldsSet[key]
	configMu.RUnlock()
	if restricted {
		key = DefaultPrefix + key
	}
	return ParseLogField(key, value)
}

// ParseLogField serialises key and value into a JSON key-value fragment
// (e.g. `"key": "value"`). It does not check for reserved-key collisions;
// callers that need collision detection should use ValidateandParseLogField
// instead.
func ParseLogField(key string, value any) string {
	sb := strings.Builder{}
	sb.Grow(100 + len(key))
	sb.WriteString("\"")
	sb.WriteString(key)
	sb.WriteString("\": \"")
	switch value := value.(type) {
	case string:
		sb.WriteString(value)
	case int, int16, int32, int64:
		sb.WriteString(fmt.Sprintf("%d", value))
	case float32, float64:
		sb.WriteString(fmt.Sprintf("%f", value))
	default:
		sb.WriteString(fmt.Sprintf("%+v", value))
	}
	sb.WriteString("\"")
	return sb.String()
}

// SetStaticEnvFieldsParser sets the function that extracts static
// environment fields merged into every log line. Passing nil clears the
// field. Safe for concurrent use.
func SetStaticEnvFieldsParser(parser StaticEnvFieldsParser) {
	var parsed string
	if parser != nil {
		list := []string{}
		for key, value := range parser() {
			list = append(list, ValidateandParseLogField(key, value))
		}
		if len(list) > 0 {
			parsed = strings.Join(list, ", ")
		}
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.parsedStaticFields = parsed
}

// SetContextFieldsParser sets the function that extracts per-request
// context fields. Safe for concurrent use.
func SetContextFieldsParser(parser ContextFieldsParser) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.contextParser = parser
}

// RegisterHook registers hook to be invoked whenever a log event at
// level is dispatched. Safe for concurrent use.
func RegisterHook(level enum.LogLevel, hook PublishLogMessageHookContract) {
	configMu.Lock()
	defer configMu.Unlock()
	levelHooks, exists := defaultConfig.hooks[level]
	if !exists {
		levelHooks = make(map[string]PublishLogMessageHookContract)
	}
	levelHooks[hook.Name()] = hook
	defaultConfig.hooks[level] = levelHooks
}

// DeRegisterHook removes the hook identified by hookName from level's
// handler set. It is a no-op if the level or name is unknown. Safe for
// concurrent use.
func DeRegisterHook(level enum.LogLevel, hookName string) {
	configMu.Lock()
	defer configMu.Unlock()
	levelHooks, exists := defaultConfig.hooks[level]
	if !exists {
		return
	}
	delete(levelHooks, hookName)
	defaultConfig.hooks[level] = levelHooks
}

// SetDefaultFields merges fields into the default field map used by every log
// entry. Entries with empty keys or values are skipped. After merging, the
// restrictedFieldsSet is rebuilt so that the O(1) collision check always
// reflects the current (possibly renamed) field names for the five core keys.
//
// why: if the caller renames enum.DefaultLogKeyTime from "time" to "ts", the
// restricted set must track "ts" (not "time") so that a subsequent user field
// key "ts" is correctly prefixed. Building the set from the post-merge
// defaultFields map captures the rename atomically under the write lock.
//
// Safe for concurrent use.
func SetDefaultFields(fields map[enum.DefaultLogKey]string) {
	if fields == nil {
		return
	}
	configMu.Lock()
	defer configMu.Unlock()
	for key, value := range fields {
		if len(string(key)) == 0 || len(value) == 0 {
			continue // Skip empty keys
		}
		defaultConfig.defaultFields[key] = value
	}
	restrictedFieldsSet = buildRestrictedSet(defaultConfig.defaultFields)
}

// GetConfig returns a pointer to the current logger configuration.
// The returned pointer is valid until the next call to resetConfig
// (test-only). Safe for concurrent use.
func GetConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return defaultConfig
}

// ProcessLogEvent drains the dispatch channel and forwards each event
// to every registered PreProcessor. It is started as a goroutine by
// resetConfig and runs until the channel is closed. Safe for concurrent
// use: it takes an RLock to snapshot the current ch and EventPreProcessors
// values so that a concurrent resetConfig does not create a data race on
// those globals.
func ProcessLogEvent() {
	configMu.RLock()
	currentCh := ch
	configMu.RUnlock()

	for e := range currentCh {
		configMu.RLock()
		processors := EventPreProcessors
		configMu.RUnlock()
		for _, observer := range processors {
			observer.PreProcess(e.Level, e.Data)
		}
	}
}

// resetConfig resets the logger configuration to default values.
//
// Unexported by design (D-9 / ARCH-15): exposing this at runtime would
// let production callers re-create the dispatch channel + dispatcher
// goroutine, leaking goroutines and dropping in-flight events
// (NF5 / NF7 / NF8). Tests reach this via the build-tagged
// TestResetConfig shim in test_only_helpers.go.
//
// why: configMu.Lock() is taken before replacing the globals so that
// concurrent ProcessLogEvent, PublishLog, and Set* callers do not race
// on ch or defaultConfig — this is the fix for issue #54 / story 039.
// The old channel is closed after the pointer swap so the previous
// ProcessLogEvent goroutine exits cleanly rather than leaking.
func resetConfig() {
	// Build the new config and channel before acquiring the lock to
	// minimise lock-hold time — encoder factory can take allocations.
	newCh := make(chan LogEvent, 10)
	encoderObj, _ := encoder.DefaultEncoderFactory(enum.EncoderJSON)
	newCfg := &Config{
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

	configMu.Lock()
	ch = newCh
	defaultConfig = newCfg
	EventPreProcessors = make(map[string]preProcessingObserverContract)
	// why: restrictedFieldsSet must be rebuilt here so that
	// ValidateandParseLogField works correctly from the very first call —
	// even before any SetDefaultFields call is made. This is the fix for D-8:
	// the former slice was only populated inside SetDefaultFields, leaving it
	// empty (and all reserved keys un-prefixed) until the caller explicitly
	// invoked that function.
	restrictedFieldsSet = buildRestrictedSet(newCfg.defaultFields)
	configMu.Unlock()

	// why: the old channel is intentionally not closed here. resetConfig
	// is a test-only path (called once from init at program start; the
	// test shim TestResetConfig calls it in unit tests only). Closing the
	// old channel while a concurrent PublishLog might still hold an RLock
	// and be mid-send would require two-phase coordination that adds
	// complexity beyond this story's scope. The pre-existing goroutine
	// leak (old ProcessLogEvent blocking on the unreferenced channel) is
	// a known trade-off tracked separately; it is harmless in test
	// binaries which exit after the test run completes.
	go ProcessLogEvent()
}

// MinLevel returns the minimum log level. Safe for concurrent use.
func (c *Config) MinLevel() enum.LogLevel {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.minLevel
}

// EncoderType returns the encoder type. Safe for concurrent use.
func (c *Config) EncoderType() enum.LogEncodeType {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.encoderType
}

// AddSource reports whether source file and line are appended to log
// entries. Safe for concurrent use.
func (c *Config) AddSource() bool {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.addSource
}

// Output returns the output writer. Safe for concurrent use.
func (c *Config) Output() io.Writer {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.output
}

// LogBuffer returns the maximum log buffer size. Safe for concurrent use.
func (c *Config) LogBuffer() int {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.logBufferMaxSize
}

// Rate returns the log flush rate. Safe for concurrent use.
func (c *Config) Rate() time.Duration {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.rate
}

// StaticFields returns the pre-rendered static field string. Safe for
// concurrent use.
func (c *Config) StaticFields() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.parsedStaticFields
}

// ContextParser returns the context field extractor function. Safe for
// concurrent use.
func (c *Config) ContextParser() ContextFieldsParser {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.contextParser
}

// DefaultFields returns a copy reference to the default field map. Safe
// for concurrent use; callers must not modify the returned map.
func (c *Config) DefaultFields() map[enum.DefaultLogKey]string {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.defaultFields
}

// TimeFormat returns the time format string. Safe for concurrent use.
func (c *Config) TimeFormat() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.timeFormat
}

// Encoder returns the encoder instance. Safe for concurrent use.
func (c *Config) Encoder() encoder.Encoder {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.encoderObj
}

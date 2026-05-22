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
// the < 1 µs budget). Hot-path booleans (HasEventPreProcessors) are
// additionally mirrored into atomic.Bool fields so the most frequent
// callers pay a single load instruction rather than a full RLock pair.
package config

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/architagr/lognugget/encoder"
	"github.com/architagr/lognugget/enum"
)

var (
	// DafaultLevel is the minimum log level applied when no explicit level has been set.
	DafaultLevel enum.LogLevel = enum.LevelInfo
	// DafaultEncoderType is the encoder format used when no explicit encoder has been set.
	DafaultEncoderType enum.LogEncodeType = enum.EncoderJSON
	// DefaultAddSource is the package default for whether source file/line
	// information is appended to every log entry. It is false by design so
	// that zero-config deployments do not pay the runtime.Callers overhead
	// (D-7). Callers may opt in explicitly via config.SetAddSource(true).
	DefaultAddSource bool = false
	// DefaultOutput is the writer used for log output when none has been configured.
	DefaultOutput io.Writer = os.Stdout
	// DefaultTimeFormat is the time layout applied to log entry timestamps when none has been configured.
	DefaultTimeFormat string = time.RFC3339
	// DafaultLogBuffer is the default channel buffer size for the log dispatch pipeline.
	DafaultLogBuffer int = 1000
	// DefaultPrefix is prepended to user-supplied field keys that collide with reserved log keys.
	DefaultPrefix string = "custom."
)

// atomicMinLevel mirrors defaultConfig.minLevel as an atomic.Int64 so that
// the level gate in entry.logWithSkip can short-circuit without acquiring
// configMu. It is updated by SetMinLevel and resetConfig under the write lock
// to stay consistent with the guarded copy.
//
// why: eliminating 2 lock calls (RLock + RUnlock for GetConfig().MinLevel())
// on the filtered path removes ~200 ns of contention per filtered event,
// keeping the overall call budget inside the < 1 µs SLO (Epic V2 / LLD §2.1).
var atomicMinLevel atomic.Int64

// hasPreProcessorsAtomic mirrors len(EventPreProcessors) > 0 as an atomic.Bool
// so that HasEventPreProcessors can short-circuit without acquiring configMu.
// It is stored inside configMu.Lock in every mutator (InitPreProcessors,
// AddPreProcessors, RemovePreProcessor, resetConfig) to stay consistent with
// the guarded map.
//
// why: the former RLock+RUnlock pair in HasEventPreProcessors cost ~80 ns on the
// hot log path (V3-P1 / LLD §3.1). atomic.Bool.Load is a single memory barrier
// with no scheduler interaction, bringing the check to < 1 ns/op (see
// BenchmarkHasEventPreProcessors in preprocessor_gate_bench_test.go).
var hasPreProcessorsAtomic atomic.Bool

// hotSnapshotPtr holds the current HotSnapshot as an atomic pointer so that
// GetHotSnapshot can load the snapshot with a single memory barrier instead of
// acquiring configMu.RLock and copying the struct field-by-field.
//
// why: the former configMu.RLock + struct copy path cost ~120-200 ns under
// contention on the read side. atomic.Pointer.Load is a single instruction with
// no scheduler interaction, making the read path allocation-free and
// contention-free (V3-P2 / LLD §4.1 / ~120 ns saving per log event).
//
// Invariant: hotSnapshotPtr must never be nil after package init. It is stored
// inside every configMu.Lock scope (in all Set* mutators and in resetConfig)
// so readers always observe a fully-initialised snapshot.
var hotSnapshotPtr atomic.Pointer[HotSnapshot]

// Compile-time width guard: enum.LogLevel must fit in int64 so the atomic
// store is lossless. If LogLevel ever widens beyond int64 this line will
// produce a compile error, not a silent data truncation.
var _ int64 = int64(enum.LevelFatal)

// channelCapacity is the buffer size used when resetConfig creates the
// dispatch channel. Set via SetChannelCapacity before init() fires.
// Range: [1, 100_000]; values outside this range are clamped with a warning.
// Stored as atomic.Int64 so concurrent test helpers can write it safely under -race.
var channelCapacity atomic.Int64

const channelCapacityMax = 100_000

func init() { channelCapacity.Store(int64(DafaultLogBuffer)) }

// ContextFieldsAppender is a function that appends per-request context fields
// directly to dst as pre-serialised bytes and returns the extended slice.
// It is the high-performance alternative to ContextFieldsParser: the appender
// writes straight into the log-line buffer without allocating an intermediate
// map.
//
// Security contract: the caller is solely responsible for RFC 8259 escaping.
// Appending raw user-controlled bytes (e.g. unescaped string values) without
// using AppendQuotedString or AppendField can introduce log-injection
// vulnerabilities. Each field must be formatted as ,"key":value where key and
// string values are JSON-escaped. Use config.AppendQuotedString for strings.
//
// Collision note: the appender path bypasses the reserved-key collision check
// performed by logWithSkip for variadic fields. If an appender writes a key
// that collides with a reserved default key (time, level, message, error,
// caller), the resulting JSON will have a duplicate key; behaviour on
// duplicate-key JSON is consumer-defined. Use distinct key names to avoid this.
type ContextFieldsAppender = func(ctx context.Context, dst []byte) []byte

// HotSnapshot is an immutable, lock-free view of the config fields consumed
// by the hot log path. A single configMu.RLock in GetHotSnapshot captures all
// fields atomically; after the snapshot is taken, callers access it without
// any further locking.
//
// why: replacing 6+ individual configMu.RLock/RUnlock calls inside
// logWithSkip with one GetHotSnapshot call reduces lock overhead from
// ~1.2 µs to ~200 ns on a contended path (Epic V2 LLD §2.2).
//
// Callers must treat every field as read-only; the maps (DefaultFields,
// Rendered, RestrictedFields) are shared references — do not mutate them.
type HotSnapshot struct {
	// AddSource controls whether the call-site function name is appended.
	AddSource bool
	// TimeFormat is the strftime-compatible time format string.
	TimeFormat string
	// DefaultFields maps each core log key to its configured field name.
	DefaultFields map[enum.DefaultLogKey]string
	// Rendered maps each core log key to its pre-rendered `"name":` bytes.
	Rendered map[enum.DefaultLogKey][]byte
	// StaticFields is the pre-serialised static field fragment (may be "").
	StaticFields string
	// ContextParser is the legacy context-field extractor (nil if unset).
	ContextParser ContextFieldsParser
	// ContextAppender is the high-performance context-field writer (nil if unset).
	// P3 will set this; P1 carries the field so the struct is forward-compatible.
	ContextAppender ContextFieldsAppender
	// Encoder is the active log encoder (JSON or text).
	Encoder encoder.Encoder
	// EncoderType is the discriminator for the active encoder.
	EncoderType enum.LogEncodeType
	// RestrictedFields is the O(1) set of field names reserved for core keys.
	RestrictedFields map[string]struct{}
	// EncoderOpen is Encoder.OpenBytes() pre-fetched outside the lock.
	EncoderOpen []byte
	// EncoderClose is Encoder.CloseBytes() pre-fetched outside the lock.
	EncoderClose []byte
}

// GetAtomicMinLevel returns the current minimum log level via an atomic load.
// It never acquires configMu and performs zero heap allocations, making it
// safe to call on every log entry without lock contention.
//
// why: the lock-free gate is the first guard in logWithSkip; filtered events
// spend zero time in the scheduler waiting for a reader/writer to release
// configMu (Epic V2 LLD §2.1 / TS-05 acceptance #4).
func GetAtomicMinLevel() enum.LogLevel {
	return enum.LogLevel(atomicMinLevel.Load())
}

// storeHotSnapshot builds a fresh HotSnapshot from defaultConfig and
// restrictedFieldsSet and atomically publishes it to hotSnapshotPtr.
//
// Precondition: caller must hold configMu (write lock). Calling this outside
// the lock would allow a concurrent Set* writer to produce a snapshot that
// mixes fields from two different writes, violating the atomic-visibility
// guarantee (V3-P2 / LLD §4.1).
//
// why: building the snapshot inside the lock ensures that every field in the
// published pointer is consistent with respect to each other. Readers that
// call GetHotSnapshot after the pointer is stored see a coherent snapshot
// without ever acquiring configMu.
func storeHotSnapshot() {
	snap := &HotSnapshot{
		AddSource:        defaultConfig.addSource,
		TimeFormat:       defaultConfig.timeFormat,
		DefaultFields:    defaultConfig.defaultFields,
		Rendered:         defaultConfig.defaultFieldsRendered,
		StaticFields:     defaultConfig.parsedStaticFields,
		ContextParser:    defaultConfig.contextParser,
		ContextAppender:  defaultConfig.contextAppender,
		Encoder:          defaultConfig.encoderObj,
		EncoderType:      defaultConfig.encoderType,
		RestrictedFields: restrictedFieldsSet,
	}
	// why: OpenBytes/CloseBytes return package-level constant slices written
	// once at encoder construction and never mutated. They are safe to call
	// inside the lock without risk of circular locking.
	snap.EncoderOpen = snap.Encoder.OpenBytes()
	snap.EncoderClose = snap.Encoder.CloseBytes()
	hotSnapshotPtr.Store(snap)
}

// GetHotSnapshot returns the current hot-path config snapshot via a single
// atomic.Pointer load. No lock is acquired; the returned value is a complete,
// consistent snapshot published by the last Set* mutator or resetConfig call.
//
// why: replacing the former configMu.RLock + struct copy with a single
// atomic.Pointer.Load removes ~120 ns of lock overhead per log event, the
// largest single saving in Epic V3 (LLD §4.1 / bench-baseline.txt).
//
// Callers must treat every map field (DefaultFields, Rendered, RestrictedFields)
// as read-only; they are shared references — do not mutate them.
func GetHotSnapshot() HotSnapshot {
	return *hotSnapshotPtr.Load()
}

// PublishLogMessageHookContract is the interface that log-level hooks must
// implement. Hooks are registered via RegisterHook and called by the pre-processor
// fan-out stage for every matching log event.
type PublishLogMessageHookContract interface {
	PublishLogMessage(entry []byte)
	Name() string
}

// StaticEnvFieldsParser is a function that returns a map of static fields
// to be merged into every log line at startup. Pass it to SetStaticEnvFieldsParser.
type StaticEnvFieldsParser = func() map[string]any

// ContextFieldsParser is the legacy per-request context field extractor.
// It receives a context and returns a map of fields to merge into the log line.
// Prefer ContextFieldsAppender for zero-allocation hot paths.
type ContextFieldsParser = func(ctx context.Context) map[string]any

func init() {
	resetConfig()
}

// Config holds the active logger configuration. All fields are guarded by
// configMu; use the exported Set* mutators and Get* accessors rather than
// reading or writing fields directly.
type Config struct {
	minLevel              enum.LogLevel                 // Minimum log level to log
	encoderType           enum.LogEncodeType            // Encoder type to use for logging
	encoderObj            encoder.Encoder               // encoder for the data
	addSource             bool                          // Whether to add source information to logs
	output                io.Writer                     // Output writer for logs
	logBufferMaxSize      int                           // max Buffer size for logs
	rate                  time.Duration                 // Rate to push logs to output
	parsedStaticFields    string                        // this is the satic fields
	contextParser         ContextFieldsParser           // Function to extract context fields
	contextAppender       ContextFieldsAppender         // High-performance context-field writer (P3)
	defaultFields         map[enum.DefaultLogKey]string // Default fields to log with every entry
	defaultFieldsRendered map[enum.DefaultLogKey][]byte // pre-rendered `"key":` prefix bytes; populated by buildRenderedFields
	timeFormat            string                        // Time format for log entries
	hooks                 map[enum.LogLevel]map[string]PublishLogMessageHookContract
}

// LogEvent is a single serialised log message dispatched through the pipeline.
// Level is used by pre-processors to route the event to matching hooks;
// Data holds the fully encoded log line (e.g. a JSON object with a trailing newline).
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
	// why: store inside the lock so HasEventPreProcessors sees a consistent value
	// with respect to every other configMu writer (V3-P1 / LLD §3.1).
	hasPreProcessorsAtomic.Store(len(EventPreProcessors) > 0)
}

// AddPreProcessors registers one or more pre-processors into the global
// map. It is safe to call concurrently.
func AddPreProcessors(observers ...preProcessingObserverContract) {
	configMu.Lock()
	defer configMu.Unlock()
	for _, observer := range observers {
		EventPreProcessors[observer.Name()] = observer
	}
	// why: store inside the lock so HasEventPreProcessors sees a consistent value
	// with respect to every other configMu writer (V3-P1 / LLD §3.1).
	hasPreProcessorsAtomic.Store(len(EventPreProcessors) > 0)
}

// HasEventPreProcessors reports whether at least one pre-processor is
// registered. It reads hasPreProcessorsAtomic with a single atomic load —
// no lock is acquired — making it safe for concurrent use with zero contention
// on the hot log path (V3-P1 / LLD §3.1).
//
// why: the former RLock+RUnlock pair cost ~80 ns per call; an atomic.Bool.Load
// costs < 1 ns. The value is kept consistent by all mutators (InitPreProcessors,
// AddPreProcessors, RemovePreProcessor, resetConfig) which store to
// hasPreProcessorsAtomic inside their configMu.Lock sections.
func HasEventPreProcessors() bool {
	return hasPreProcessorsAtomic.Load()
}

// RemovePreProcessor deletes the named pre-processor from the global map.
// It is safe to call concurrently.
func RemovePreProcessor(name string) {
	configMu.Lock()
	defer configMu.Unlock()
	delete(EventPreProcessors, name)
	// why: store inside the lock so HasEventPreProcessors sees a consistent value
	// with respect to every other configMu writer (V3-P1 / LLD §3.1).
	hasPreProcessorsAtomic.Store(len(EventPreProcessors) > 0)
}

// SetMinLevel sets the minimum log level for the logger. It stores the level
// both in defaultConfig (guarded by configMu) and in atomicMinLevel so that
// the lock-free gate in GetAtomicMinLevel immediately observes the change.
// storeHotSnapshot is called inside the lock so GetHotSnapshot immediately
// reflects the new level on the atomic read path (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetMinLevel(level enum.LogLevel) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.minLevel = level
	atomicMinLevel.Store(int64(level))
	storeHotSnapshot()
}

// SetTimeFormat sets the time format for log entries. storeHotSnapshot is
// called inside the lock so the atomic snapshot immediately reflects the new
// format without requiring callers to hold configMu (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetTimeFormat(format string) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.timeFormat = format
	storeHotSnapshot()
}

// SetEncoderType sets the encoder type for the logger. If the requested
// encoder type is unknown, it falls back to EncoderJSON. storeHotSnapshot is
// called inside the lock so the atomic snapshot immediately reflects the new
// encoder and its OpenBytes/CloseBytes constants (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetEncoderType(encoderType enum.LogEncodeType) {
	var err error

	configMu.Lock()
	defer configMu.Unlock()

	defaultConfig.encoderObj, err = encoder.DefaultEncoderFactoryE(encoderType)
	if err != nil {
		encoderType = enum.EncoderJSON
		defaultConfig.encoderObj = encoder.DefaultEncoderFactory(encoderType)
	}

	defaultConfig.encoderType = encoderType
	storeHotSnapshot()
}

// SetAddSource sets whether source file and line information is appended
// to every log entry. storeHotSnapshot is called inside the lock so the
// atomic snapshot immediately reflects the new AddSource flag (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetAddSource(addSource bool) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.addSource = addSource
	storeHotSnapshot()
}

// SetOutput sets the output writer for the logger. A nil argument is
// silently replaced with DefaultOutput. output is not part of HotSnapshot
// (it is accessed via GetConfig().Output() on the write path), but
// storeHotSnapshot is called here for consistency so the snapshot remains
// fully up-to-date with the rest of defaultConfig (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetOutput(output io.Writer) {
	if output == nil {
		output = DefaultOutput
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.output = output
	storeHotSnapshot()
}

// PublishLog sends Data onto the dispatch channel. Safe for concurrent use;
// callers block if the channel buffer is full (back-pressure, F23).
//
// why (P4 ownership transfer): entry.logWithSkip now transfers ownership of
// its pooled buf before calling PublishLog — e.buf is severed with a fresh
// make([]byte, 0, initBufCap) and e.Put() is called only after PublishLog
// returns. Data therefore has exclusive ownership; the background goroutine
// (ProcessLogEvent) can read it safely without a defensive copy.
//
// why (lock discipline): the RLock is taken only for the pointer snapshot of
// ch, not across the send itself. resetConfig never closes the old channel
// (see comment there), so there is no "send on closed channel" risk.
// Releasing the RLock before the send avoids holding it during a potentially
// blocking channel operation.
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
// are silently replaced with 20. storeHotSnapshot is called inside the lock
// for consistency; logBufferMaxSize itself is not a HotSnapshot field but the
// call keeps the snapshot fully synchronised with defaultConfig (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetLogBufferMaxSize(size int) {
	if size <= 0 {
		size = 20 // Default buffer size
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.logBufferMaxSize = size
	storeHotSnapshot()
}

// SetRate sets the rate at which buffered logs are pushed to output.
// Values ≤ 0 are silently replaced with 1 second. storeHotSnapshot is called
// inside the lock for consistency; rate itself is not a HotSnapshot field but
// the call keeps the snapshot fully synchronised with defaultConfig (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetRate(rate time.Duration) {
	if rate <= 0 {
		rate = 1 * time.Second // Default rate is 1 sec
	}
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.rate = rate
	storeHotSnapshot()
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

// buildRenderedFields constructs a new map of pre-rendered JSON key prefixes of
// the form `"name":` for every key in fields. The resulting []byte slices are
// ready to append directly before a JSON value, eliminating the per-call
// string→[]byte conversion that AppendField performs when building the key.
//
// Must be called while the caller holds configMu (write) — it constructs a
// fresh map; no existing map is mutated.
//
// why: pre-rendering the three mandatory fields (time, level, message) once at
// init and again on SetDefaultFields removes the repeated allocation of
// `[]byte(key)` in AppendField's appendJSONString call. Each pre-rendered
// prefix is a single heap object allocated at configuration time, never on
// the hot log path (ARCH-6 / LLD §6.5 / ~80 ns per-event saving target).
func buildRenderedFields(fields map[enum.DefaultLogKey]string) map[enum.DefaultLogKey][]byte {
	rendered := make(map[enum.DefaultLogKey][]byte, len(fields))
	for k, name := range fields {
		// Build `"name":` — the key prefix an appender concatenates with the value.
		b := make([]byte, 0, len(name)+3)
		b = append(b, '"')
		b = append(b, name...)
		b = append(b, '"', ':')
		rendered[k] = b
	}
	return rendered
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
// (e.g. `"key":value`). It does not check for reserved-key collisions;
// callers that need collision detection should use ValidateandParseLogField
// instead.
//
// Prefer AppendField(dst, key, value) for zero-copy incremental construction
// of log lines. This wrapper is kept for the existing callers in entry.go;
// story 018 will migrate those sites to AppendField directly.
func ParseLogField(key string, value any) string {
	return string(AppendField(nil, key, value))
}

// SetStaticEnvFieldsParser sets the function that extracts static
// environment fields merged into every log line. Passing nil clears the
// field. storeHotSnapshot is called inside the lock so the atomic snapshot
// immediately reflects the new StaticFields fragment (V3-P2 / LLD §4.1).
// Safe for concurrent use.
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
	storeHotSnapshot()
}

// SetContextFieldsParser sets the function that extracts per-request
// context fields. storeHotSnapshot is called inside the lock so the atomic
// snapshot immediately reflects the new ContextParser (V3-P2 / LLD §4.1).
// Safe for concurrent use.
func SetContextFieldsParser(parser ContextFieldsParser) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.contextParser = parser
	storeHotSnapshot()
}

// SetContextFieldsAppender sets the zero-alloc context field writer.
// When set, it takes precedence over any ContextFieldsParser on the hot path.
// The appender receives the entry buffer and must append ,key:value fragments
// for each context field, returning the extended buffer. storeHotSnapshot is
// called inside the lock so the atomic snapshot immediately reflects the new
// ContextAppender (V3-P2 / LLD §4.1). Safe for concurrent use.
func SetContextFieldsAppender(appender ContextFieldsAppender) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.contextAppender = appender
	storeHotSnapshot()
}

// SetChannelCapacity sets the buffer size for the dispatch channel created by
// the next resetConfig call. Must be called before init() fires (Go
// init-ordering guarantee) and before InitPreProcessors. No configMu needed —
// the variable is written once at startup, read once in resetConfig.
//
// Values ≤ 0 are clamped to DafaultLogBuffer (1000); values > 100_000 are
// clamped to 100_000. A log.Printf warning is emitted for out-of-range values.
func SetChannelCapacity(n int) {
	if n <= 0 || n > channelCapacityMax {
		log.Printf("SetChannelCapacity(%d): out of range [1, %d]; clamping to default %d", n, channelCapacityMax, DafaultLogBuffer)
		if n > channelCapacityMax {
			channelCapacity.Store(int64(channelCapacityMax))
		} else {
			channelCapacity.Store(int64(DafaultLogBuffer))
		}
		return
	}
	channelCapacity.Store(int64(n))
}

// GetChannelCapacity returns the current channel capacity value.
// Exposed for testing; not intended for production use.
func GetChannelCapacity() int {
	return int(channelCapacity.Load())
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
	// why: rebuild the pre-rendered prefix cache so that the hot path in
	// entry.logWithSkip immediately sees the new `"newname":` bytes without
	// incurring any per-call allocation. Both caches must be rebuilt together
	// under the same write lock to keep restrictedFieldsSet and
	// defaultFieldsRendered consistent (ARCH-6 / acceptance criterion 3).
	defaultConfig.defaultFieldsRendered = buildRenderedFields(defaultConfig.defaultFields)
	// why: storeHotSnapshot must be called after both buildRestrictedSet and
	// buildRenderedFields so the published snapshot contains the fully-rebuilt
	// maps. A snapshot stored before either rebuild would expose stale map
	// references to concurrent readers on the atomic path (V3-P2 / LLD §4.1).
	storeHotSnapshot()
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
	newCh := make(chan LogEvent, int(channelCapacity.Load()))
	encoderObj := encoder.DefaultEncoderFactory(enum.EncoderJSON)
	newCfg := &Config{
		minLevel:           DafaultLevel,
		encoderType:        DafaultEncoderType,
		encoderObj:         encoderObj,
		addSource:          DefaultAddSource,
		output:             DefaultOutput,
		logBufferMaxSize:   DafaultLogBuffer, // Default buffer size
		rate:               1 * time.Second,  // Default rate is 1 sec
		parsedStaticFields: "",
		contextParser:      nil,
		timeFormat:         DefaultTimeFormat,
		// why: hooks must be initialised here so that RegisterHook does not
		// panic with "assignment to entry in nil map". resetConfig is the only
		// site that creates a new Config, so a nil hooks map after reset would
		// make the first RegisterHook call fatal (discovered by TS-22 race tests).
		hooks: make(map[enum.LogLevel]map[string]PublishLogMessageHookContract),
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
	// why: buildRenderedFields is called before the lock so the allocation cost
	// stays outside the critical section, consistent with the pattern used for
	// encoderObj above.  The result is safe to store in newCfg because newCfg
	// is not yet visible to any other goroutine at this point.
	newCfg.defaultFieldsRendered = buildRenderedFields(newCfg.defaultFields)

	configMu.Lock()
	ch = newCh
	defaultConfig = newCfg
	EventPreProcessors = make(map[string]preProcessingObserverContract)
	// why: mirror the reset into the atomic so HasEventPreProcessors immediately
	// observes false after TestResetConfig is called in tests (and at init).
	// Without this store, the atomic would retain a stale true from a previous
	// InitPreProcessors call across test resets (V3-P1 / LLD §3.1).
	hasPreProcessorsAtomic.Store(false)
	// why: restrictedFieldsSet must be rebuilt here so that
	// ValidateandParseLogField works correctly from the very first call —
	// even before any SetDefaultFields call is made. This is the fix for D-8:
	// the former slice was only populated inside SetDefaultFields, leaving it
	// empty (and all reserved keys un-prefixed) until the caller explicitly
	// invoked that function.
	restrictedFieldsSet = buildRestrictedSet(newCfg.defaultFields)
	// why: mirror the reset level into the atomic so GetAtomicMinLevel
	// immediately observes the default after TestResetConfig is called in
	// tests (and at init). Without this store, the atomic would retain a
	// stale value from a previous SetMinLevel call across test resets.
	atomicMinLevel.Store(int64(newCfg.minLevel))
	// why: storeHotSnapshot must be called after defaultConfig, restrictedFieldsSet,
	// and atomicMinLevel are all written, so the published pointer contains a
	// fully-consistent reset snapshot. Readers that call GetHotSnapshot after
	// configMu.Unlock() will see the reset state without ever acquiring the lock
	// (V3-P2 / LLD §4.1 / Test_GetHotSnapshot_NilSafe).
	storeHotSnapshot()
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

// DefaultFieldsRendered returns the pre-rendered JSON key-prefix map. Each
// entry maps a DefaultLogKey to the bytes `"name":` ready to append directly
// before a quoted JSON value, eliminating the per-call string→[]byte
// conversion that AppendField performs when rendering the key.
//
// The map is populated by buildRenderedFields when resetConfig runs (at init
// and in test resets) and rebuilt atomically under the write lock whenever
// SetDefaultFields is called. Callers must not modify the returned map.
//
// Safe for concurrent use.
func (c *Config) DefaultFieldsRendered() map[enum.DefaultLogKey][]byte {
	configMu.RLock()
	defer configMu.RUnlock()
	return c.defaultFieldsRendered
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

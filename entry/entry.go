package entry

import (
	"context"
	"runtime"

	"github.com/architagr/lognugget/config"
	customTime "github.com/architagr/lognugget/custom_time"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
)

// LogEntry is a single structured-log event in flight. Callers obtain a
// *LogEntry from NewLogEntry (pool-backed), invoke exactly one level method
// (Debug, Info, Warn, Error, Fatal, Panic), and must not retain a reference
// after the call — the entry is returned to the pool by Log.
//
// LogEntry is not safe for concurrent use; each goroutine that needs to log
// must obtain its own entry via NewLogEntry.
type LogEntry struct {
	// caller is the call-site frame appended to the log line when cfg.addSource
	// is true. Populated lazily inside logWithSkip via runtime.Caller(skip).
	caller *runtime.Frame
	// buf is the reusable byte accumulator for the rendered log body.
	// It is pre-grown to initBufCap at construction and reset to len=0 (not
	// nil) on each pool cycle so that the backing array survives across reuses.
	// why: eliminates the make([]byte, 0, 256) allocation on every Log call
	// (D-6 / F27). initBufCap = 1 KB covers the vast majority of log lines.
	buf []byte
	// pendingBuf accumulates chain-method fields (Str/Int/Uint/Float64/Bool)
	// before they are flushed into buf during logWithSkip. Each field is
	// written as ",key":value so they can be appended verbatim.
	// Pre-grown to pendingBufCap (256 B) at pool construction; capacity is
	// retained across pool cycles to eliminate per-call allocations.
	pendingBuf []byte
}

// initBufCap is the initial capacity of LogEntry.buf. 1 KB covers ~80% of
// real-world structured log lines without reallocation on the hot path.
const initBufCap = 1024

// p8SeveranceCapHook, when non-nil, is called with the newCap value computed
// by the P8 exact-size buffer severance code. Always nil in production; set
// by white-box tests to verify buffer sizing without depending on sync.Pool
// slot identity (which is non-deterministic under -race / multi-P execution).
var p8SeveranceCapHook func(int)

// pendingBufCap is the initial capacity of LogEntry.pendingBuf. 256 B covers
// the typical chain-method field set (10 typed fields) without reallocation.
const pendingBufCap = 256

// NewLogEntry obtains a *LogEntry from the internal sync.Pool, resets all
// fields to zero, and returns it ready for use. Callers must invoke exactly
// one level method; the entry is returned to the pool automatically.
func NewLogEntry() *LogEntry {
	e := entryPool.Get().(*LogEntry)
	e.reset()
	return e
}

// reset zeroes every field of LogEntry so that entries returned from the
// sync.Pool carry no state from a previous caller.
//
// why: D-18 — using a zero-value assignment instead of manually listing
// fields ensures that new fields added to LogEntry in the future are
// automatically zeroed without requiring a matching update here.
//
// Exception: buf is intentionally retained (len reset to 0, capacity kept)
// so that the backing array survives pool cycles and eliminates the per-call
// make([]byte, 0, 256) allocation (D-6 / F27). The reflect-exhaustive test
// in reset_test.go is updated to allow this exception.
func (e *LogEntry) reset() {
	retained := e.buf[:0]
	retainedPending := e.pendingBuf[:0]
	*e = LogEntry{}
	e.buf = retained
	e.pendingBuf = retainedPending
}

// Put returns e to the internal sync.Pool. It is called automatically by Log
// after publishing; callers should not invoke it directly unless they abandon
// an entry without logging.
//
// Safety after PublishLog (P4 ownership-transfer model): logWithSkip transfers
// ownership of e.buf to config.PublishLog by severing the alias before calling
// Put — the slice sent to the channel is a distinct allocation from the buffer
// retained by the pool. Put therefore cannot race with ProcessLogEvent reading
// the dispatched bytes.
func (e *LogEntry) Put() {
	entryPool.Put(e)
}

// callerSkipDirect is the number of frames to skip when runtime.Caller is
// invoked from inside logWithSkip and logWithSkip was called directly by the
// public entry point (Log or a level method).
//
// Stack when user calls e.Info(ctx, msg):
//
//	skip=0 → logWithSkip  (the function that called runtime.Caller)
//	skip=1 → Info         (called logWithSkip)
//	skip=2 → user code    <- the frame we want
//
// Stack when user calls e.Log(level, ctx, msg, nil):
//
//	skip=0 → logWithSkip
//	skip=1 → Log
//	skip=2 → user code    <- the frame we want
//
// why: both Log and the level methods call logWithSkip directly, so the depth
// to the user's frame is always 2. A single constant covers all public entry points.
const callerSkipDirect = 2

// Log assembles the structured log line from level, ctx, message, err, and
// any extra fields, encodes it, and dispatches it through config.PublishLog.
// It returns immediately if level is below the configured minimum (level gate
// runs first — before any allocation) or if no pre-processors are registered.
// The entry is returned to the pool before Log returns in all code paths,
// including the early-return filtered path, to prevent pool depletion.
func (e *LogEntry) Log(level enum.LogLevel, ctx context.Context, message string, err error, fields ...model.LogAttr) {
	e.logWithSkip(level, ctx, message, err, callerSkipDirect, fields...)
}

// logWithSkip is the internal implementation of Log. The skip parameter is
// forwarded directly to runtime.Caller so callers can adjust the reported
// call-site frame. Use Log for external call sites and level methods for
// convenience wrappers.
//
// why: a separate skip-aware implementation lets test helpers exercise the
// ok=false path of runtime.Caller (by passing a skip value that exceeds the
// stack depth) without exposing that parameter on the public API.
//
// Hot-path lock discipline (Epic V2 P1):
//  1. GetAtomicMinLevel — zero locks, zero allocs on the filtered path.
//  2. GetHotSnapshot — single configMu.RLock replaces the former 6+ individual
//     GetConfig() calls, reducing lock overhead from ~1.2 µs to ~200 ns.
func (e *LogEntry) logWithSkip(level enum.LogLevel, ctx context.Context, message string, err error, skip int, fields ...model.LogAttr) {
	// why: atomic level gate runs BEFORE EventPreProcessors check and before
	// any allocation. A filtered call must spend zero heap allocations.
	// GetAtomicMinLevel never acquires configMu (D-13 / TS-05 / Epic V2 LLD §2.1).
	if config.GetAtomicMinLevel() > level {
		e.Put()
		return
	}
	if !config.HasEventPreProcessors() {
		e.Put()
		return
	}

	// why: one GetHotSnapshot call captures all config fields under a single
	// RLock instead of the former pattern of calling GetConfig() once per
	// field (AddSource, TimeFormat, DefaultFields, etc.). Each individual
	// GetConfig() call took its own RLock; the snapshot collapses them to one
	// (Epic V2 LLD §2.2).
	snap := config.GetHotSnapshot()

	// why: capture caller before any other work so that runtime.Caller sees
	// the correct frame depth. D-1 / F17 / ARCH-3: use singular runtime.Caller,
	// not runtime.Callers + CallersFrames.
	if snap.AddSource {
		if pc, _, _, ok := runtime.Caller(skip); ok {
			fn := runtime.FuncForPC(pc)
			frame := &runtime.Frame{}
			if fn != nil {
				frame.Function = fn.Name()
			}
			e.caller = frame
		} else {
			e.caller = &runtime.Frame{Function: "unknown"}
		}
	}

	// why: prepend the encoder's opening bytes (e.g. `{` for JSON) directly into
	// e.buf instead of allocating a new buffer via Encoder.Append(nil, body).
	// snap.EncoderOpen is pre-fetched in GetHotSnapshot (outside configMu) so
	// this append performs zero locks. D-6 / F27 / ARCH-8 / Epic V2 P4.
	e.buf = append(e.buf[:0], snap.EncoderOpen...)

	// why: append the pre-rendered `"key":` prefix bytes directly instead of
	// calling AppendField, which would re-encode the key string on every call.
	// buildRenderedFields pre-computes these at init and on SetDefaultFields so
	// the hot path performs zero extra allocations for the three mandatory
	// fields (ARCH-6 / LLD §6.5).
	e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyTime]...)
	// why: RFC 3339 chars (digits, T, Z, -, :, +, .) need no JSON escaping, so
	// we write the surrounding quotes directly and let time.AppendFormat fill the
	// value in-place. This eliminates the intermediate string alloc from
	// customTime.Format and the second alloc inside AppendQuotedString — saving
	// ~2 allocs and ~40 ns per log entry (V3-P4 / story #115).
	e.buf = append(e.buf, '"')
	e.buf = customTime.TimeNow().AppendFormat(e.buf, snap.TimeFormat)
	e.buf = append(e.buf, '"')
	e.buf = append(e.buf, ',')
	e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyLevel]...)
	e.buf = config.AppendQuotedLevel(e.buf, level)
	e.buf = append(e.buf, ',')
	e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyMessage]...)
	e.buf = config.AppendQuotedString(e.buf, message)

	for _, field := range fields {
		key := string(field.Key)
		if _, ok := snap.DefaultFields[enum.DefaultLogKey(field.Key)]; ok {
			// why: user-supplied key collides with a reserved default key;
			// prefix it so the reserved key is never shadowed. D-8.
			key = config.DefaultPrefix + key
		}
		e.buf = append(e.buf, ',')
		// why: AppendAttr dispatches by kind — typed attrs (KindStr, KindInt, etc.)
		// are serialised without any interface{} boxing, eliminating a heap alloc
		// per field on the hot path (Epic V2 P2). KindAny falls back to the legacy
		// AppendField-equivalent type switch for backward compatibility.
		e.buf = config.AppendAttr(e.buf, key, field)
	}

	// Flush chain-method fields (Str/Int/Uint/Float64/Bool). These were written
	// to pendingBuf as ",key":value fragments; appending them here is a single
	// slice copy with zero allocations when buf has remaining capacity.
	if len(e.pendingBuf) > 0 {
		e.buf = append(e.buf, e.pendingBuf...)
		e.pendingBuf = e.pendingBuf[:0]
	}

	// Context fields — AppendContextFields dispatches to the zero-alloc
	// ContextAppender when registered, or falls back to the legacy parser path.
	// Both paths use snap.RestrictedFields so no extra configMu lock is needed.
	e.buf = config.AppendContextFields(ctx, e.buf, snap)

	if err != nil {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyError]...)
		e.buf = config.AppendQuotedString(e.buf, err.Error())
	}
	if e.caller != nil {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyCaller]...)
		e.buf = config.AppendQuotedString(e.buf, e.caller.Function)
	}
	if snap.StaticFields != "" {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, snap.StaticFields...)
	}

	// why: append closing bytes (e.g. `}\n` for JSON) directly into e.buf —
	// no intermediate allocation. Then transfer ownership of e.buf to the
	// channel by severing the pool alias: replace e.buf with a fresh
	// make([]byte, 0, initBufCap) so that reset() in e.Put() retains this
	// new backing array (not the one handed to PublishLog). ProcessLogEvent
	// reads Data asynchronously; after the transfer the pool goroutine and the
	// consumer goroutine each own separate backing arrays — no data race.
	// Eliminates 2 allocs per call vs pre-P4 (en.Append + dataCopy). P4.
	e.buf = append(e.buf, snap.EncoderClose...)
	data := e.buf
	newCap := len(data)
	if newCap < 64 {
		newCap = 64
	}
	e.buf = make([]byte, 0, newCap)
	if p8SeveranceCapHook != nil {
		p8SeveranceCapHook(newCap)
	}
	config.PublishLog(level, data)
	e.Put()
}

// Debug logs message at LevelDebug with optional extra fields.
func (e *LogEntry) Debug(ctx context.Context, message string, fields ...model.LogAttr) {
	e.logWithSkip(enum.LevelDebug, ctx, message, nil, callerSkipDirect, fields...)
}

// Info logs message at LevelInfo with optional extra fields.
func (e *LogEntry) Info(ctx context.Context, message string, fields ...model.LogAttr) {
	e.logWithSkip(enum.LevelInfo, ctx, message, nil, callerSkipDirect, fields...)
}

// Warn logs message at LevelWarn with optional extra fields.
func (e *LogEntry) Warn(ctx context.Context, message string, fields ...model.LogAttr) {
	e.logWithSkip(enum.LevelWarn, ctx, message, nil, callerSkipDirect, fields...)
}

// Error logs message at LevelError, attaching err's string to the "error" field.
// err may be nil; if nil, no error field is appended.
func (e *LogEntry) Error(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.logWithSkip(enum.LevelError, ctx, message, err, callerSkipDirect, fields...)
}

// Fatal logs at LevelError then calls runtime.Goexit to terminate the calling
// goroutine. The log event is guaranteed to be dispatched before Goexit fires.
func (e *LogEntry) Fatal(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.logWithSkip(enum.LevelError, ctx, message, err, callerSkipDirect, fields...)
	runtime.Goexit()
}

// Panic logs at LevelError (via Error) then panics with err. The log event
// is guaranteed to be dispatched before the panic propagates.
func (e *LogEntry) Panic(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.logWithSkip(enum.LevelError, ctx, message, err, callerSkipDirect, fields...)
	panic(err)
}

// Str appends a string field to the entry's pending buffer and returns e for
// chaining. The field is written as ,"key":"value" with RFC 8259 escaping.
// Zero heap allocations when pendingBuf has sufficient capacity.
func (e *LogEntry) Str(key, val string) *LogEntry {
	e.pendingBuf = append(e.pendingBuf, ',')
	e.pendingBuf = config.AppendAttr(e.pendingBuf, key, model.Str(key, val))
	return e
}

// Int appends an int64 field to the entry's pending buffer and returns e for
// chaining. The value is written as an unquoted JSON integer.
func (e *LogEntry) Int(key string, val int64) *LogEntry {
	e.pendingBuf = append(e.pendingBuf, ',')
	e.pendingBuf = config.AppendAttr(e.pendingBuf, key, model.Int(key, val))
	return e
}

// Uint appends a uint64 field to the entry's pending buffer and returns e for
// chaining. The value is written as an unquoted JSON integer.
func (e *LogEntry) Uint(key string, val uint64) *LogEntry {
	e.pendingBuf = append(e.pendingBuf, ',')
	e.pendingBuf = config.AppendAttr(e.pendingBuf, key, model.Uint(key, val))
	return e
}

// Float64 appends a float64 field to the entry's pending buffer and returns e
// for chaining. NaN and ±Inf are rendered as JSON null.
func (e *LogEntry) Float64(key string, val float64) *LogEntry {
	e.pendingBuf = append(e.pendingBuf, ',')
	e.pendingBuf = config.AppendAttr(e.pendingBuf, key, model.Float64(key, val))
	return e
}

// Bool appends a bool field to the entry's pending buffer and returns e for
// chaining. The value is written as an unquoted JSON boolean.
func (e *LogEntry) Bool(key string, val bool) *LogEntry {
	e.pendingBuf = append(e.pendingBuf, ',')
	e.pendingBuf = config.AppendAttr(e.pendingBuf, key, model.Bool(key, val))
	return e
}

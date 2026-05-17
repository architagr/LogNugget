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
}

// initBufCap is the initial capacity of LogEntry.buf. 1 KB covers ~80% of
// real-world structured log lines without reallocation on the hot path.
const initBufCap = 1024

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
	*e = LogEntry{}
	e.buf = retained
}

// Put returns e to the internal sync.Pool. It is called automatically by Log
// after publishing; callers should not invoke it directly unless they abandon
// an entry without logging.
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
func (e *LogEntry) logWithSkip(level enum.LogLevel, ctx context.Context, message string, err error, skip int, fields ...model.LogAttr) {
	// why: level gate runs BEFORE EventPreProcessors check and before any
	// allocation (make, AppendField, encoder.Append). A filtered call must
	// spend zero heap allocations. D-13 / TS-05.
	if config.GetConfig().MinLevel() > level {
		e.Put()
		return
	}
	if config.EventPreProcessors == nil {
		e.Put()
		return
	}

	// why: capture caller before any other work so that runtime.Caller sees
	// the correct frame depth. D-1 / F17 / ARCH-3: use singular runtime.Caller,
	// not runtime.Callers + CallersFrames.
	if config.GetConfig().AddSource() {
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

	cfg := config.GetConfig()
	defaultFields := cfg.DefaultFields()
	rendered := cfg.DefaultFieldsRendered()
	ctxData := e.setLogContextFields(ctx)

	// why: reuse pooled buf instead of make([]byte, 0, 256) per call.
	// The backing array was pre-grown to initBufCap (1 KB) at pool construction
	// and is reset to len=0 (not nil) on each pool cycle, so no allocation
	// occurs on the hot path for log lines that fit within the capacity.
	// D-6 / F27 / ARCH-8.
	e.buf = e.buf[:0]

	// why: append the pre-rendered `"key":` prefix bytes directly instead of
	// calling AppendField, which would re-encode the key string on every call.
	// buildRenderedFields pre-computes these at init and on SetDefaultFields so
	// the hot path performs zero extra allocations for the three mandatory
	// fields (ARCH-6 / LLD §6.5). AppendQuotedString handles RFC 8259 escaping
	// of the value without the key-encoding overhead.
	e.buf = append(e.buf, rendered[enum.DefaultLogKeyTime]...)
	e.buf = config.AppendQuotedString(e.buf, customTime.Format(customTime.TimeNow(), cfg.TimeFormat()))
	e.buf = append(e.buf, ',')
	e.buf = append(e.buf, rendered[enum.DefaultLogKeyLevel]...)
	e.buf = config.AppendQuotedString(e.buf, level.String())
	e.buf = append(e.buf, ',')
	e.buf = append(e.buf, rendered[enum.DefaultLogKeyMessage]...)
	e.buf = config.AppendQuotedString(e.buf, message)

	for _, field := range fields {
		key := string(field.Key)
		if _, ok := defaultFields[enum.DefaultLogKey(field.Key)]; ok {
			// why: user-supplied key collides with a reserved default key;
			// prefix it so the reserved key is never shadowed. D-8.
			key = config.DefaultPrefix + key
		}
		e.buf = append(e.buf, ',')
		e.buf = config.AppendField(e.buf, key, field.Value)
	}

	// why: ctxData is still []string from setLogContextFields (a separate
	// refactor out of scope for Story 018). Each string is already a rendered
	// "key":value fragment; we just need to append it with a leading comma.
	for _, d := range ctxData {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, d...)
	}

	if err != nil {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, rendered[enum.DefaultLogKeyError]...)
		e.buf = config.AppendQuotedString(e.buf, err.Error())
	}
	if e.caller != nil {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, rendered[enum.DefaultLogKeyCaller]...)
		e.buf = config.AppendQuotedString(e.buf, e.caller.Function)
	}
	if sf := cfg.StaticFields(); sf != "" {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, sf...)
	}

	en := cfg.Encoder()
	// why: en.Append(nil, e.buf) allocates a fresh []byte for the encoded log
	// line (wraps body in {…}\n). byteData does not share the backing array
	// with e.buf, so it is safe to Put e immediately after (D-6 / story 024
	// explicit-copy step is a belt-and-suspenders guard for future encoders).
	byteData := en.Append(nil, e.buf)
	config.PublishLog(level, byteData)

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

func (e *LogEntry) setLogContextFields(ctx context.Context) []string {
	if ctxParser := config.GetConfig().ContextParser(); ctx != nil && ctxParser != nil {
		data := []string{}
		for key, value := range ctxParser(ctx) {
			data = append(data, config.ValidateandParseLogField(string(key), value))
		}
		return data
	}
	return nil
}

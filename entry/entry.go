package entry

import (
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/architagr/lognugget/config"
	customTime "github.com/architagr/lognugget/custom_time"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
)

var (
	entryPool sync.Pool
)

func init() {
	entryPool = sync.Pool{
		New: func() any {
			return initLogEntry()
		},
	}
}

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
}

// NewLogEntry obtains a *LogEntry from the internal sync.Pool, resets all
// fields to zero, and returns it ready for use. Callers must invoke exactly
// one level method; the entry is returned to the pool automatically.
func NewLogEntry() *LogEntry {
	e := entryPool.Get().(*LogEntry)
	e.reset()
	return e
}

// GenerateInitialPool pre-warms the internal sync.Pool with n ready-to-use
// LogEntry instances. Call once at program startup to reduce allocation
// pressure on the first burst of log calls.
func GenerateInitialPool(n int) {
	for i := 0; i < n; i++ {
		entryPool.Put(initLogEntry())
	}
}

func initLogEntry() *LogEntry {
	return &LogEntry{}
}

// reset zeroes every field of LogEntry so that entries returned from the
// sync.Pool carry no state from a previous caller.
//
// why: D-18 — using a zero-value assignment instead of manually listing
// fields ensures that new fields added to LogEntry in the future are
// automatically zeroed without requiring a matching update here. A manual
// nil/zero list is a maintenance hazard: any new field not added to reset()
// silently leaks state across pooled reuses. The reflect-exhaustive test in
// reset_test.go enforces this guarantee at test time.
func (e *LogEntry) reset() {
	*e = LogEntry{}
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
//   skip=0 → logWithSkip  (the function that called runtime.Caller)
//   skip=1 → Info         (called logWithSkip)
//   skip=2 → user code    <- the frame we want
//
// Stack when user calls e.Log(level, ctx, msg, nil):
//   skip=0 → logWithSkip
//   skip=1 → Log
//   skip=2 → user code    <- the frame we want
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
	// allocation (make, strings.Join, encoder.Append). A filtered call must
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

	defaultFields := config.GetConfig().DefaultFields()
	ctxData := e.setLogContextFields(ctx)
	data := make([]string, 3+len(fields)+len(ctxData))
	data[0] = config.ParseLogField(defaultFields[enum.DefaultLogKeyTime], customTime.Format(customTime.TimeNow(), config.GetConfig().TimeFormat()))
	data[1] = config.ParseLogField(defaultFields[enum.DefaultLogKeyLevel], level.String())
	data[2] = config.ParseLogField(defaultFields[enum.DefaultLogKeyMessage], message)
	for j, field := range fields {
		if _, ok := defaultFields[enum.DefaultLogKey(field.Key)]; ok {
			field.Key = model.LogAttrKey(config.DefaultPrefix) + field.Key
		}
		data[3+j] = config.ParseLogField(string(field.Key), field.Value)
	}
	for j, d := range ctxData {
		data[3+len(fields)+j] = d
	}
	if err != nil {
		data = append(data, config.ParseLogField(defaultFields[enum.DefaultLogKeyError], err.Error()))
	}
	if e.caller != nil {
		data = append(data, config.ParseLogField(defaultFields[enum.DefaultLogKeyCaller], e.caller.Function))
	}
	str := strings.Join(data, ", ")
	if config.GetConfig().StaticFields() != "" {
		str += ", " + config.GetConfig().StaticFields()
	}

	en := config.GetConfig().Encoder()
	byteData := en.Append(nil, []byte(str))
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

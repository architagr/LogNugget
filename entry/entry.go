package entry

import (
	"context"
	"runtime"
	"sync"
	"time"

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

type LogEntry struct {
	// published indicates if the log entry has been published to the output stream
	published bool
	// data custom fields to be logged
	data []model.LogAttr
	// context current context object
	context context.Context
	// level the log entry was logged at: Trace, Debug, Info, Warn, Error, Fatal or Panic
	// This field will be set on entry firing and the value will be equal to the one in Logger struct field.
	level enum.LogLevel
	// time time when the log entry was created
	time time.Time
	// message log message
	message string
	// Error in case of error
	err error
	// caller Calling method, with package name
	caller *runtime.Frame // TODO: add a function to set caller from runtime.Caller
}

// implement a builder to duplicate an existing logEntry having below functions
// WithContext
// WithError
// WithFields
// WithTime

func NewLogEntry() *LogEntry {
	e := entryPool.Get().(*LogEntry)
	e.reset()
	return e
}

func GenerateInitialPool(n int) {
	for i := 0; i < n; i++ {
		entryPool.Put(initLogEntry())
	}
}

func initLogEntry() *LogEntry {
	return &LogEntry{
		data:    make([]model.LogAttr, 0),
		context: context.Background(),
		time:    customTime.TimeNow(),
	}
}
func (e *LogEntry) reset() {
	e.data = make([]model.LogAttr, 0)
	e.context = context.Background()
	e.level = enum.LevelUnSet
	e.time = customTime.TimeNow()
	e.message = ""
	e.err = nil
	e.caller = nil
	e.published = false
}

func (e *LogEntry) Put() {
	entryPool.Put(e)
}

func (e *LogEntry) WithFields(fields ...model.LogAttr) *LogEntry {
	if e.published || len(fields) == 0 {
		return e
	}
	e.data = append(e.data, fields...)
	return e
}

func (e *LogEntry) WithTime(t time.Time) *LogEntry {
	if e.published {
		return e
	}
	if t.IsZero() {
		t = customTime.TimeNow()
	}
	e.time = t
	return e
}

func (e *LogEntry) WithContext(ctx context.Context) *LogEntry {
	if e.published {
		return e
	}
	e.context = ctx
	return e
}

func (e *LogEntry) WithCaller(caller *runtime.Frame) *LogEntry {
	if e.published {
		return e
	}
	if caller == nil {
		return e
	}
	e.caller = caller
	return e
}

func (e *LogEntry) WithMessage(msg string) *LogEntry {
	if e.published {
		return e
	}
	e.message = msg
	return e
}

func (e *LogEntry) WithLevel(level enum.LogLevel) *LogEntry {
	if e.published || level < enum.LevelDebug || level > enum.LevelError {
		return e
	}
	e.level = level
	return e
}

func (e *LogEntry) Clone() *LogEntry {
	clone := NewLogEntry().
		WithFields(e.data...).
		WithTime(e.time).
		WithCaller(e.caller).
		WithLevel(e.level).
		WithContext(e.context).
		WithMessage(e.message)
	return clone
}

func (e *LogEntry) IsEmpty() bool {
	return e == nil || (len(e.data) == 0 && e.context == nil && e.time.IsZero() && e.message == "" && e.err == nil && e.caller == nil)
}

func (e *LogEntry) IsValid() bool {
	return e != nil && !e.time.IsZero() && e.level >= enum.LevelDebug && e.level <= enum.LevelError
}

func (e *LogEntry) IsError() bool {
	return e != nil && e.err != nil
}

func (e *LogEntry) IsDebug() bool {
	return e != nil && e.level == enum.LevelDebug
}

func (e *LogEntry) IsInfo() bool {
	return e != nil && e.level == enum.LevelInfo
}

func (e *LogEntry) IsWarnLevel() bool {
	return e != nil && e.level == enum.LevelWarn
}

func (e *LogEntry) IsTraceLevel() bool {
	return e != nil && e.level == enum.LevelDebug // Assuming Trace is equivalent to Debug in this context
}

func (e *LogEntry) IsErrorLevel() bool {
	return e != nil && e.level == enum.LevelError
}

func (e *LogEntry) IsFatalLevel() bool {
	return e != nil && e.level == enum.LevelFatal && e.err != nil
}

func (e *LogEntry) IsPanicLevel() bool {
	return e != nil && e.level == enum.LevelFatal && e.err != nil && e.caller != nil && e.caller.Function == "runtime.panic"
}

func (e *LogEntry) IsContextEmpty() bool {
	return e == nil || (e.context == nil || e.context == context.Background())
}

func (e *LogEntry) IsDataEmpty() bool {
	return e == nil || (len(e.data) == 0)
}

func (e *LogEntry) IsTimeEmpty() bool {
	return e == nil || (e.time.IsZero())
}

func (e *LogEntry) IsMessageEmpty() bool {
	return e == nil || (e.message == "")
}

func (e *LogEntry) IsCallerEmpty() bool {
	return e == nil || (e.caller == nil)
}

func (e *LogEntry) Level() enum.LogLevel {
	return e.level
}

func (e *LogEntry) Log(level enum.LogLevel, ctx context.Context, message string, fields ...model.LogAttr) {

	if e.published || config.GetConfig().MinLevel() > level || config.EventPreProcessors == nil {
		return
	}

	e.context = ctx
	e.message = message
	e.data = append(e.data, fields...)
	e.level = level

	for _, observer := range config.EventPreProcessors {
		observer.PreProcess(e)
	}
	e.reset()
	e.Put()
}

func (e *LogEntry) Debug(ctx context.Context, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelDebug, ctx, message, fields...)
}

func (e *LogEntry) Info(ctx context.Context, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelInfo, ctx, message, fields...)
}
func (e *LogEntry) Warn(ctx context.Context, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelWarn, ctx, message, fields...)
}
func (e *LogEntry) Error(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.err = err
	e.Log(enum.LevelError, ctx, message, fields...)
}

func (e *LogEntry) Fatal(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	if e.published {
		return
	}
	e.Error(ctx, err, message, fields...)
	runtime.Goexit() // Exit the program after logging fatal error
}
func (e *LogEntry) Panic(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	if e.published {
		return
	}
	e.Error(ctx, err, message, fields...)
	panic(e.err) // Panic with the error
}

func (e *LogEntry) valueKey(data map[string]any, key string) {
	if value, ok := data[key]; ok {
		data["custon."+key] = value
	}
}

func (e *LogEntry) setLogTime(data map[string]any) {
	if e.time.IsZero() {
		e.time = customTime.TimeNow()
	}
	defaultFields := config.GetConfig().DefaultFields()
	e.valueKey(data, defaultFields[enum.DefaultLogKeyTime])
	data[defaultFields[enum.DefaultLogKeyTime]] = customTime.Format(e.time, config.GetConfig().TimeFormat())
}

func (e *LogEntry) setLogLevel(data map[string]any) {
	defaultFields := config.GetConfig().DefaultFields()
	e.valueKey(data, defaultFields[enum.DefaultLogKeyLevel])
	data[defaultFields[enum.DefaultLogKeyLevel]] = e.level.String()
}

func (e *LogEntry) setLogMessage(data map[string]any) {
	defaultFields := config.GetConfig().DefaultFields()
	e.valueKey(data, defaultFields[enum.DefaultLogKeyMessage])
	data[defaultFields[enum.DefaultLogKeyMessage]] = e.message
}

func (e *LogEntry) setLogError(data map[string]any) {
	if e.err == nil {
		return
	}
	defaultFields := config.GetConfig().DefaultFields()
	e.valueKey(data, defaultFields[enum.DefaultLogKeyError])
	data[defaultFields[enum.DefaultLogKeyError]] = e.err.Error()
}

func (e *LogEntry) setLogCaller(data map[string]any) {
	if e.caller == nil {
		return
	}
	defaultFields := config.GetConfig().DefaultFields()
	e.valueKey(data, defaultFields[enum.DefaultLogKeyCaller])
	data[defaultFields[enum.DefaultLogKeyCaller]] = e.caller.Function
}

func (e *LogEntry) setLogContextFields(data map[string]any) {
	if e.context == nil {
		return
	}
	if ctxParser := config.GetConfig().ContextParser(); ctxParser != nil {
		ctxFields := ctxParser(e.context)
		for key, value := range ctxFields {
			e.valueKey(data, string(key))
			data[string(key)] = value
		}
	}
}

func (e *LogEntry) setLogStaticEnvFields(data map[string]any) {
	for key, value := range config.GetConfig().StaticEnvFields() {
		e.valueKey(data, string(key))
		data[string(key)] = value
	}
}
func (e *LogEntry) ToMap() map[string]any {
	if e.IsMessageEmpty() || e.IsTimeEmpty() {
		return nil
	}
	data := make(map[string]any, len(e.data)+len(config.GetConfig().StaticEnvFields()))
	e.setLogContextFields(data)
	for _, field := range e.data {
		data[string(field.Key)] = field.Value
	}
	e.setLogStaticEnvFields(data)
	e.setLogTime(data)
	e.setLogLevel(data)
	e.setLogMessage(data)
	e.setLogError(data)
	e.setLogCaller(data)
	return data
}

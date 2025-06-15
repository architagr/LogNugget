package lognugget

import (
	"context"
	"runtime"
	"time"
)

type LogEntry struct {
	// data custom fields to be logged
	data []LogAttr
	// context current context object
	context context.Context
	// level the log entry was logged at: Trace, Debug, Info, Warn, Error, Fatal or Panic
	// This field will be set on entry firing and the value will be equal to the one in Logger struct field.
	level LogLevel
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

func newLogEntry() *LogEntry {
	return &LogEntry{
		data:    make([]LogAttr, 0),
		context: context.Background(),
		time:    time.Now(),
	}
}

func (e *LogEntry) WithFields(fields ...LogAttr) *LogEntry {
	if len(fields) == 0 {
		return e
	}
	e.data = append(e.data, fields...)
	return e
}
func (e *LogEntry) WithTime(t time.Time) *LogEntry {
	if t.IsZero() {
		t = time.Now()
	}
	e.time = t
	return e
}

func (e *LogEntry) WithCaller(caller *runtime.Frame) *LogEntry {
	if caller == nil {
		return e
	}
	e.caller = caller
	return e
}
func (e *LogEntry) WithLevel(level LogLevel) *LogEntry {
	if level < LevelDebug || level > LevelError {
		return e
	}
	e.level = level
	return e
}

func (e *LogEntry) Clone() *LogEntry {
	clone := newLogEntry().
		WithFields(e.data...).
		WithTime(e.time).
		WithCaller(e.caller).
		WithLevel(e.level)

	return clone
}
func (e *LogEntry) IsEmpty() bool {
	return e == nil || (len(e.data) == 0 && e.context == nil && e.time.IsZero() && e.message == "" && e.err == nil && e.caller == nil)
}
func (e *LogEntry) IsValid() bool {
	return e != nil && !e.time.IsZero() && e.level >= LevelDebug && e.level <= LevelError
}
func (e *LogEntry) IsError() bool {
	return e != nil && e.err != nil
}
func (e *LogEntry) IsDebug() bool {
	return e != nil && e.level == LevelDebug
}
func (e *LogEntry) IsInfo() bool {
	return e != nil && e.level == LevelInfo
}
func (e *LogEntry) IsWarn() bool {
	return e != nil && e.level == LevelWarn
}
func (e *LogEntry) IsTrace() bool {
	return e != nil && e.level == LevelDebug // Assuming Trace is equivalent to Debug in this context
}
func (e *LogEntry) IsErrorLevel() bool {
	return e != nil && e.level == LevelError
}
func (e *LogEntry) IsFatalLevel() bool {
	return e != nil && e.level == LevelFatal && e.err != nil
}
func (e *LogEntry) IsPanicLevel() bool {
	return e != nil && e.level == LevelFatal && e.err != nil && e.caller != nil && e.caller.Function == "runtime.panic"
}

func (e *LogEntry) IsLevelNotIn(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, l := range levels {
		if e.level == l {
			return false
		}
	}
	return true
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
func (e *LogEntry) IsErrorEmpty() bool {
	return e == nil || (e.err == nil)
}
func (e *LogEntry) IsCallerEmpty() bool {
	return e == nil || (e.caller == nil)
}
func (e *LogEntry) IsValidForLevel(level LogLevel) bool {
	if e == nil {
		return false
	}
	if e.level != level || e.time.IsZero() || e.IsError() || e.IsFatalLevel() || e.IsPanicLevel() {
		return false
	}
	return true
}
func (e *LogEntry) IsValidForLevels(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level {
			return e.IsValidForLevel(level)
		}
	}
	return false
}
func (e *LogEntry) IsValidForAnyLevel(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return true
		}
	}
	return false
}
func (e *LogEntry) IsValidForAllLevels(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && !e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}
func (e *LogEntry) IsValidForNoneOfLevels(levels ...LogLevel) bool {
	if e == nil {
		return true
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}
func (e *LogEntry) IsValidForAnyOfLevels(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return true
		}
	}
	return false
}
func (e *LogEntry) IsValidForAllOfLevels(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && !e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}

func (e *LogEntry) IsValidForAllOfLevelsNotIn(levels ...LogLevel) bool {
	if e == nil {
		return true
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}
func (e *LogEntry) IsValidForNoneOfLevelsNotIn(levels ...LogLevel) bool {
	if e == nil {
		return true
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}
func (e *LogEntry) IsValidForAnyOfLevelsIn(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return true
		}
	}
	return false
}
func (e *LogEntry) IsValidForAllOfLevelsIn(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && !e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}
func (e *LogEntry) IsValidForNoneOfLevelsIn(levels ...LogLevel) bool {
	if e == nil {
		return true
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}
func (e *LogEntry) IsValidForAnyOfLevelsNotIn(levels ...LogLevel) bool {
	if e == nil {
		return false
	}
	for _, level := range levels {
		if e.level == level && e.IsValidForLevel(level) {
			return false
		}
	}
	return true
}

func (e *LogEntry) Log(level LogLevel, ctx context.Context, message string, fields ...LogAttr) {
	if defaultConfig.minLevel > level {
		return
	}
	e.context = ctx
	e.message = message
	e.data = append(e.data, fields...)
	e.level = level
	defaultConfig.encoder.Write(*e)
}

func (e *LogEntry) Debug(ctx context.Context, message string, fields ...LogAttr) {
	e.Log(LevelDebug, ctx, message, fields...)
}

func (e *LogEntry) Info(ctx context.Context, message string, fields ...LogAttr) {
	e.Log(LevelInfo, ctx, message, fields...)
}
func (e *LogEntry) Warn(ctx context.Context, message string, fields ...LogAttr) {
	e.Log(LevelWarn, ctx, message, fields...)
}
func (e *LogEntry) Error(ctx context.Context, err error, message string, fields ...LogAttr) {
	e.err = err
	e.Log(LevelError, ctx, message, fields...)
}

func (e *LogEntry) Fatal(ctx context.Context, err error, message string, fields ...LogAttr) {
	e.Error(ctx, err, message, fields...)
	runtime.Goexit() // Exit the program after logging fatal error
}
func (e *LogEntry) Panic(ctx context.Context, err error, message string, fields ...LogAttr) {
	e.Error(ctx, err, message, fields...)
	panic(e.err) // Panic with the error
}

func (e *LogEntry) valueKey(data map[string]any, key string) {
	if value, ok := data[key]; ok {
		data["custon."+key] = value
	}
}
func (e *LogEntry) ToMap() map[string]any {

	ctxFields := make(map[string]any)
	if e.context != nil {
		if ctxParser := defaultConfig.contextParser; ctxParser != nil {
			ctxFields = ctxParser(e.context)
		}
	}
	data := make(map[string]any, len(e.data)+len(defaultConfig.staticEnvFields)+len(ctxFields))
	for _, field := range e.data {
		data[string(field.Key)] = field.Value
	}

	for key, value := range ctxFields {
		e.valueKey(data, string(key))
		data[string(key)] = value
	}

	for key, value := range defaultConfig.staticEnvFields {
		e.valueKey(data, string(key))
		data[string(key)] = value
	}

	if e.time.IsZero() {
		e.time = time.Now()
	}
	e.valueKey(data, defaultConfig.defaultFields[DefaultLogKeyTime])
	e.valueKey(data, defaultConfig.defaultFields[DefaultLogKeyLevel])
	e.valueKey(data, defaultConfig.defaultFields[DefaultLogKeyMessage])

	data[defaultConfig.defaultFields[DefaultLogKeyTime]] = e.time.UTC().Format(defaultConfig.timeFormat)
	data[defaultConfig.defaultFields[DefaultLogKeyLevel]] = e.level.String()
	data[defaultConfig.defaultFields[DefaultLogKeyMessage]] = e.message

	if e.err != nil {
		e.valueKey(data, defaultConfig.defaultFields[DefaultLogKeyError])
		data[defaultConfig.defaultFields[DefaultLogKeyError]] = e.err.Error()
	}

	if e.caller != nil {
		e.valueKey(data, defaultConfig.defaultFields[DefaultLogKeyCaller])
		data[defaultConfig.defaultFields[DefaultLogKeyCaller]] = e.caller.Function
	}
	return data
}

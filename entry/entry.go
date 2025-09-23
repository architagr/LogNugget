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

type LogEntry struct {
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
	return &LogEntry{}
}
func (e *LogEntry) reset() {
	e.caller = nil
}

func (e *LogEntry) Put() {
	entryPool.Put(e)
}

func (e *LogEntry) Log(level enum.LogLevel, ctx context.Context, message string, err error, fields ...model.LogAttr) {
	if config.GetConfig().MinLevel() > level || config.EventPreProcessors == nil {
		return
	}

	defaultFields := config.GetConfig().DefaultFields()
	ctxData := e.setLogContextFields(ctx)
	data := make([]string, 3+len(fields)+len(ctxData), len(fields)+5)
	i := 0
	data[i] = config.ParseLogField(defaultFields[enum.DefaultLogKeyTime], customTime.Format(customTime.TimeNow(), config.GetConfig().TimeFormat()))
	data[i+1] = config.ParseLogField(defaultFields[enum.DefaultLogKeyLevel], level.String())
	data[i+2] = config.ParseLogField(defaultFields[enum.DefaultLogKeyMessage], message)
	i += 2
	for _, field := range fields {
		if _, ok := defaultFields[enum.DefaultLogKey(field.Key)]; ok {
			field.Key = model.LogAttrKey(config.DefaultPrefix) + field.Key
		}
		i++
		data[i] = config.ParseLogField(string(field.Key), field.Value)
	}

	for x, d := range ctxData {
		data[i+x] = d
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
	byteData, _ := en.Write(str)
	config.PublishLog(level, byteData)

	e.Put()
}

func (e *LogEntry) Debug(ctx context.Context, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelDebug, ctx, message, nil, fields...)
}

func (e *LogEntry) Info(ctx context.Context, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelInfo, ctx, message, nil, fields...)
}
func (e *LogEntry) Warn(ctx context.Context, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelWarn, ctx, message, nil, fields...)
}
func (e *LogEntry) Error(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.Log(enum.LevelError, ctx, message, err, fields...)
}

func (e *LogEntry) Fatal(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.Error(ctx, err, message, fields...)
	runtime.Goexit() // Exit the program after logging fatal error
}
func (e *LogEntry) Panic(ctx context.Context, err error, message string, fields ...model.LogAttr) {
	e.Error(ctx, err, message, fields...)
	panic(err) // Panic with the error
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

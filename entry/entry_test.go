package entry

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	"github.com/stretchr/testify/assert"
)

type TestPreProcessorObserver struct {
	logEntry      string
	isExecuted    bool
	timeToProcess time.Duration
}

func (t *TestPreProcessorObserver) PreProcess(entry config.LogEntryContract) {
	t.isExecuted = true
	n := time.Now()
	t.logEntry = entry.ToMap()
	t.timeToProcess = time.Since(n)
}

func (t *TestPreProcessorObserver) Name() string {
	return "TestPreProcessorObserver"
}

// var timeoutForSingleLogProcessing = 200 * time.Microsecond

func TestEntryForDebugWithMinLogLevelAsDebug(t *testing.T) {
	var buf bytes.Buffer
	config.SetOutput(&buf)
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})
	ctx := context.WithValue(context.Background(), "request_id", "12345")

	observer := &TestPreProcessorObserver{}
	config.InitPreProcessors(observer)

	// Create a new log entry
	entry := NewLogEntry()
	entry.Debug(ctx, "This is a debug message", []model.LogAttr{
		{Key: model.LogAttrKey("id"), Value: model.LogAttrValue(1)},
		{Key: model.LogAttrKey("message"), Value: model.LogAttrValue("message")},
	}...)
	defaultFields := config.GetConfig().DefaultFields()

	assert.True(t, observer.isExecuted, "Pre processor should be executed for debug log when min log level is debug")
	assert.Contains(t, observer.logEntry, config.Bar("app_name", "lognugget"), "Static field app_name should be set")
	assert.Contains(t, observer.logEntry, config.Bar("version", "1.0.0"), "Static field version should be set")
	assert.Contains(t, observer.logEntry, config.Bar("request_id", "12345"), "Context field request_id should be set")
	assert.Contains(t, observer.logEntry, config.Bar("id", "1"), "Field id should be set")
	assert.Contains(t, observer.logEntry, config.Bar(config.DefaultPrefix+"message", "message"), "Field message should be set")
	assert.Contains(t, observer.logEntry, config.Bar(defaultFields[enum.DefaultLogKeyMessage], "This is a debug message"), "Log message should match")
	assert.Contains(t, observer.logEntry, defaultFields[enum.DefaultLogKeyTime], "Log entry should have a time field")
	assert.Contains(t, observer.logEntry, config.Bar(defaultFields[enum.DefaultLogKeyLevel], enum.LevelDebug.String()), "Log message should match")
	assert.NotContains(t, observer.logEntry, "caller", "Log entry should have a caller field")
	// assert.LessOrEqual(t, observer.timeToProcess.Microseconds(), int64(timeoutForSingleLogProcessing.Microseconds()), "Pre processor should process log entry within the timeout")
}

func TestEntryForDebugWithMinLogLevelAsError(t *testing.T) {
	var buf bytes.Buffer
	config.SetOutput(&buf)
	config.SetMinLevel(enum.LevelError)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})
	ctx := context.WithValue(context.Background(), "request_id", "12345")
	observer := &TestPreProcessorObserver{}
	config.InitPreProcessors(observer)

	// Create a new log entry
	entry := NewLogEntry()

	// Check if the fields are set correctly
	entry.Debug(ctx, "This is a debug message", []model.LogAttr{
		{Key: model.LogAttrKey("id"), Value: model.LogAttrValue(1)},
		{Key: model.LogAttrKey("message"), Value: model.LogAttrValue("message")},
	}...)
	assert.False(t, observer.isExecuted, "Pre processor should not be executed for debug log when min log level is error")
}

func TestEntryForErrorWithMinLogLevelAsDebug(t *testing.T) {
	var buf bytes.Buffer
	config.SetOutput(&buf)
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})
	ctx := context.WithValue(context.Background(), "request_id", "12345")

	observer := &TestPreProcessorObserver{}
	config.InitPreProcessors(observer)

	entry := NewLogEntry()
	entry.Error(ctx, errors.New("not found"), "This is a error message", []model.LogAttr{
		{Key: model.LogAttrKey("id"), Value: model.LogAttrValue(1)},
		{Key: model.LogAttrKey("message"), Value: model.LogAttrValue("message")},
	}...)

	defaultFields := config.GetConfig().DefaultFields()

	assert.True(t, observer.isExecuted, "Pre processor should be executed for debug log when min log level is debug")

	assert.True(t, observer.isExecuted, "Pre processor should be executed for debug log when min log level is debug")
	assert.Contains(t, observer.logEntry, config.Bar("app_name", "lognugget"), "Static field app_name should be set")
	assert.Contains(t, observer.logEntry, config.Bar("version", "1.0.0"), "Static field version should be set")
	assert.Contains(t, observer.logEntry, config.Bar("request_id", "12345"), "Context field request_id should be set")
	assert.Contains(t, observer.logEntry, config.Bar("id", "1"), "Field id should be set")
	assert.Contains(t, observer.logEntry, config.Bar(config.DefaultPrefix+"message", "message"), "Field message should be set")
	assert.Contains(t, observer.logEntry, config.Bar(defaultFields[enum.DefaultLogKeyMessage], "This is a error message"), "Log message should match")
	assert.Contains(t, observer.logEntry, defaultFields[enum.DefaultLogKeyTime], "Log entry should have a time field")
	assert.Contains(t, observer.logEntry, config.Bar(defaultFields[enum.DefaultLogKeyLevel], enum.LevelError.String()), "Log message should match")
	assert.NotContains(t, observer.logEntry, "caller", "Log entry should have a caller field")
	assert.Contains(t, observer.logEntry, config.Bar(defaultFields[enum.DefaultLogKeyError], "not found"), "Field error should be set")
	// assert.LessOrEqual(t, observer.timeToProcess.Microseconds(), int64(timeoutForSingleLogProcessing.Microseconds()), "Pre processor should process log entry within the timeout")
}

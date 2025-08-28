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
	logEntry      map[string]any
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

	// Close the channel AFTER sending all entries
	assert.True(t, observer.isExecuted, "Pre processor should be executed for debug log when min log level is debug")
	assert.Equal(t, "lognugget", observer.logEntry["app_name"], "Static field app_name should be set")
	assert.Equal(t, "1.0.0", observer.logEntry["version"], "Static field version should be set")
	assert.Equal(t, "12345", observer.logEntry["request_id"], "Context field request_id should be set")
	assert.Equal(t, 1, observer.logEntry["id"], "Field id should be set")
	assert.Equal(t, "message", observer.logEntry["custon.message"], "Field message should be set")
	assert.Equal(t, "This is a debug message", observer.logEntry["message"], "Log message should match")
	assert.NotEmpty(t, observer.logEntry["time"], "Log entry should have a time field")
	assert.Equal(t, enum.LevelDebug.String(), observer.logEntry["level"], "Log level should be debug")
	assert.Empty(t, observer.logEntry["caller"], "Log entry should have a caller field")
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
	assert.Equal(t, "lognugget", observer.logEntry["app_name"], "Static field app_name should be set")
	assert.Equal(t, "1.0.0", observer.logEntry["version"], "Static field version should be set")
	assert.Equal(t, "12345", observer.logEntry["request_id"], "Context field request_id should be set")
	assert.Equal(t, 1, observer.logEntry["id"], "Field id should be set")
	assert.Equal(t, "not found", observer.logEntry[defaultFields[enum.DefaultLogKeyError]], "Field message should be set")
	assert.Equal(t, "message", observer.logEntry["custon.message"], "Field message should be set")
	assert.Equal(t, "This is a error message", observer.logEntry[defaultFields[enum.DefaultLogKeyMessage]], "Log message should match")
	assert.NotEmpty(t, observer.logEntry[defaultFields[enum.DefaultLogKeyTime]], "Log entry should have a time field")
	assert.Equal(t, enum.LevelError.String(), observer.logEntry[defaultFields[enum.DefaultLogKeyLevel]], "Log level should be debug")
	assert.Empty(t, observer.logEntry["caller"], "Log entry should have a caller field")
	// assert.LessOrEqual(t, observer.timeToProcess.Microseconds(), int64(timeoutForSingleLogProcessing.Microseconds()), "Pre processor should process log entry within the timeout")

}

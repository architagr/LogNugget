package lognugget

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntryForDebugWithMinLogLevelAsDebug(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetMinLevel(LevelDebug)
	SetEncoderType(EncoderJSON)
	SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})
	ctx := context.WithValue(context.Background(), "request_id", "12345")
	// Create a new log entry
	entry := newLogEntry()

	// Set some fields
	entry.WithFields([]LogAttr{
		{Key: LogAttrKey("id"), Value: LogAttrValue(1)},
		{Key: LogAttrKey("message"), Value: LogAttrValue("message")},
	}...)
	// Check if the fields are set correctly
	entry.Debug(ctx, "This is a debug message")

	s := buf.String()
	if !assert.NotEmpty(t, s, "Log entry should not be empty") {
		return
	}
	var logEntry map[string]any
	err := json.Unmarshal([]byte(s), &logEntry)
	if !assert.NoError(t, err, "Log entry should be valid JSON") {
		return
	}
	assert.Equal(t, "lognugget", logEntry["app_name"], "Static field app_name should be set")
	assert.Equal(t, "1.0.0", logEntry["version"], "Static field version should be set")
	assert.Equal(t, "12345", logEntry["request_id"], "Context field request_id should be set")
	assert.Equal(t, float64(1), logEntry["id"], "Field id should be set")
	assert.Equal(t, "message", logEntry["custon.message"], "Field message should be set")
	assert.Equal(t, "This is a debug message", logEntry["message"], "Log message should match")
	assert.NotEmpty(t, logEntry["time"], "Log entry should have a time field")
	assert.Equal(t, LevelDebug.String(), logEntry["level"], "Log level should be debug")
	assert.Empty(t, logEntry["caller"], "Log entry should have a caller field")
}

func TestEntryForDebugWithMinLogLevelAsError(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetMinLevel(LevelError)
	SetEncoderType(EncoderJSON)
	SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})
	ctx := context.WithValue(context.Background(), "request_id", "12345")
	// Create a new log entry
	entry := newLogEntry()

	// Set some fields
	entry.WithFields([]LogAttr{
		{Key: LogAttrKey("id"), Value: LogAttrValue(1)},
		{Key: LogAttrKey("message"), Value: LogAttrValue("message")},
	}...)
	// Check if the fields are set correctly
	entry.Debug(ctx, "This is a debug message")

	s := buf.String()
	if !assert.Empty(t, s, "Log entry should be empty") {
		return
	}
}

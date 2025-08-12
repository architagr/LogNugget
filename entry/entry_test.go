package entry

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	"github.com/stretchr/testify/assert"
)

var (
	timeoutForSingleLogProcessing = 300 * time.Microsecond
)

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

	// Create a buffered channel with buffer size 1 (or more if multiple logs)
	stream := make(chan *LogEntry, 1)
	// Create a new log entry
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		runValidation := false
		for le := range stream {
			logEntry := le.ToMap()
			runValidation = true
			assert.Equal(t, "lognugget", logEntry["app_name"], "Static field app_name should be set")
			assert.Equal(t, "1.0.0", logEntry["version"], "Static field version should be set")
			assert.Equal(t, "12345", logEntry["request_id"], "Context field request_id should be set")
			assert.Equal(t, 1, logEntry["id"], "Field id should be set")
			assert.Equal(t, "message", logEntry["custon.message"], "Field message should be set")
			assert.Equal(t, "This is a debug message", logEntry["message"], "Log message should match")
			assert.NotEmpty(t, logEntry["time"], "Log entry should have a time field")
			assert.Equal(t, enum.LevelDebug.String(), logEntry["level"], "Log level should be debug")
			assert.Empty(t, logEntry["caller"], "Log entry should have a caller field")
		}
		assert.True(t, runValidation)
	}()

	entry := newLogEntry(stream)
	entry.Debug(ctx, "This is a debug message", []model.LogAttr{
		{Key: model.LogAttrKey("id"), Value: model.LogAttrValue(1)},
		{Key: model.LogAttrKey("message"), Value: model.LogAttrValue("message")},
	}...)

	// Close the channel AFTER sending all entries
	close(stream)

	// Wait for validation to complete
	wg.Wait()
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
	wg := sync.WaitGroup{}
	stream := func(t *testing.T, wg *sync.WaitGroup) chan<- *LogEntry {
		stream := make(chan *LogEntry)
		wg.Add(1)
		time.AfterFunc(timeoutForSingleLogProcessing, func() {
			close(stream)
			wg.Done()
		})
		go func() {
			runValidation := false
			for range stream {
				runValidation = true
			}
			assert.False(t, runValidation)
		}()
		return stream
	}(t, &wg)
	// Create a new log entry
	entry := newLogEntry(stream)

	// Check if the fields are set correctly
	entry.Debug(ctx, "This is a debug message", []model.LogAttr{
		{Key: model.LogAttrKey("id"), Value: model.LogAttrValue(1)},
		{Key: model.LogAttrKey("message"), Value: model.LogAttrValue("message")},
	}...)
	wg.Wait()
}

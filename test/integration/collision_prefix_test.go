//go:build testing

// Package integration contains end-to-end tests that drive the full log
// pipeline from entry.LogEntry through the config singleton down to the
// pre-processor hooks. Tests here require the `testing` build tag because
// they import test/support builders (D-9 / ARCH-15).
//
// SC7 coverage: a user field key that collides with a reserved log field name
// is prefixed with "custom." in the emitted JSON.
package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	"github.com/architagr/lognugget/test/support"
)


// drainSpyIntegration polls spy.Records() until at least one record is
// available, failing after a time-bounded number of iterations.
//
// why: config.PublishLog is asynchronous; the event is sent on a buffered
// channel consumed by config.ProcessLogEvent in a background goroutine. A
// short yield-loop is necessary before asserting on the emitted payload.
func drainSpyIntegration(t *testing.T, spy *support.FakePreProc) []byte {
	t.Helper()
	for i := 0; i < 500; i++ {
		recs := spy.Records()
		if len(recs) > 0 {
			return recs[0].Data
		}
		// Yield to the scheduler without a fixed sleep duration.
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	t.Fatal("timed out waiting for log record from FakePreProc")
	return nil
}

// Test_Integration_CollisionPrefix_UserFieldTimeEmitsAsCustomTime is the SC7
// integration test. It exercises the full pipeline:
//
//  1. Reset config singleton to known defaults.
//  2. Register a FakePreProc spy as the sole pre-processor.
//  3. Emit a log entry with user field key "time" (reserved).
//  4. Assert that the JSON output contains key "custom.time" and does NOT
//     contain a duplicate plain "time" key that would shadow the system field.
//
// The test is single-threaded (not t.Parallel) because it mutates the
// process-wide config singleton.
func Test_Integration_CollisionPrefix_UserFieldTimeEmitsAsCustomTime(t *testing.T) {
	// Arrange: reset singleton to JSON/Debug defaults; spy receives all events.
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()

	spy := support.NewFakePreProc("sc7-spy")
	config.InitPreProcessors(spy)

	// Act: log a message with user field "time" (reserved collision).
	entry.NewLogEntry().Info(
		context.Background(),
		"sc7 collision test message",
		model.LogAttr{Key: "time", Value: "user-supplied-time-value"},
	)

	// Wait for the async pipeline to deliver the event.
	raw := drainSpyIntegration(t, spy)

	// Assert: parse JSON and verify collision-prefix behaviour.
	var logMap map[string]string
	if err := json.Unmarshal(raw, &logMap); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\npayload: %s", err, raw)
	}

	// The prefixed key must exist with the user-supplied value.
	const prefixedKey = "custom.time"
	if got, ok := logMap[prefixedKey]; !ok {
		t.Errorf("JSON output missing %q; full payload: %s", prefixedKey, raw)
	} else if got != "user-supplied-time-value" {
		t.Errorf("logMap[%q] = %q, want %q", prefixedKey, got, "user-supplied-time-value")
	}

	// The system "time" field must still be present (the reserved field is the
	// timestamp, not the user value).
	if _, ok := logMap["time"]; !ok {
		t.Errorf("system 'time' field missing from JSON output; full payload: %s", raw)
	}

	// Sanity: the plain user-field key "time" must not carry the user value
	// (i.e. the user value must only appear under "custom.time").
	if logMap["time"] == "user-supplied-time-value" {
		t.Errorf("system 'time' field was overwritten by user value; collision prefix not applied")
	}
}

// Test_Integration_CollisionPrefix_MultipleReservedKeysAllPrefixed extends SC7
// to all five core reserved keys. Each user field collision must yield a
// prefixed key in the emitted JSON.
func Test_Integration_CollisionPrefix_MultipleReservedKeysAllPrefixed(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()

	spy := support.NewFakePreProc("sc7-multi-spy")
	config.InitPreProcessors(spy)

	// Emit with all five reserved keys as user fields simultaneously.
	entry.NewLogEntry().Info(
		context.Background(),
		"multi collision test",
		model.LogAttr{Key: "level", Value: "user-level"},
		model.LogAttr{Key: "message", Value: "user-message"},
		model.LogAttr{Key: "error", Value: "user-error"},
		model.LogAttr{Key: "caller", Value: "user-caller"},
	)

	raw := drainSpyIntegration(t, spy)

	var logMap map[string]string
	if err := json.Unmarshal(raw, &logMap); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\npayload: %s", err, raw)
	}

	type checkCase struct {
		prefixedKey string
		wantValue   string
	}
	cases := []checkCase{
		{"custom.level", "user-level"},
		{"custom.message", "user-message"},
		{"custom.error", "user-error"},
		{"custom.caller", "user-caller"},
	}

	for _, c := range cases {
		if got, ok := logMap[c.prefixedKey]; !ok {
			t.Errorf("JSON output missing %q; full payload: %s", c.prefixedKey, raw)
		} else if got != c.wantValue {
			t.Errorf("logMap[%q] = %q, want %q", c.prefixedKey, got, c.wantValue)
		}
	}
}

// Test_Integration_CollisionPrefix_StaticFieldTimeEmitsAsCustomTime verifies
// SC7 for static-env-injected fields: when SetStaticEnvFieldsParser returns a
// map with key "time", ValidateandParseLogField must prefix it with "custom."
// before the field reaches the emitted JSON.
func Test_Integration_CollisionPrefix_StaticFieldTimeEmitsAsCustomTime(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()

	spy := support.NewFakePreProc("sc7-static-spy")
	config.InitPreProcessors(spy)

	// Install a static parser that returns "time" as a static field (reserved collision).
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"time": "static-time-value",
		}
	})

	entry.NewLogEntry().Info(context.Background(), "static collision test")

	// Allow enough time for the background goroutine to deliver.
	var raw []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recs := spy.Records()
		if len(recs) > 0 {
			raw = recs[0].Data
			break
		}
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	if raw == nil {
		t.Fatal("timed out waiting for log record")
	}

	var logMap map[string]string
	if err := json.Unmarshal(raw, &logMap); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\npayload: %s", err, raw)
	}

	const prefixedKey = "custom.time"
	if got, ok := logMap[prefixedKey]; !ok {
		t.Errorf("static field 'time' not prefixed; want %q in JSON: %s", prefixedKey, raw)
	} else if got != "static-time-value" {
		t.Errorf("logMap[%q] = %q, want %q", prefixedKey, got, "static-time-value")
	}
}

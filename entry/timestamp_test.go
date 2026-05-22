//go:build testing

// Package entry_test: V3-P4 timestamp-format correctness tests.
//
// These tests verify that the direct AppendFormat path (replacing
// customTime.Format + AppendQuotedString) produces RFC 3339–valid timestamps
// under the default format and correctly renders custom layouts. They run under
// the `testing` build tag because they import the test/support package.
package entry_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// drainTimestampRec polls spy.Records() until at least one record arrives,
// returning the first record's raw bytes. Fails after 500 goroutine-yield
// iterations — matching the pattern used in levels_test.go.
func drainTimestampRec(t *testing.T, spy *support.FakePreProc) []byte {
	t.Helper()
	for i := 0; i < 500; i++ {
		recs := spy.Records()
		if len(recs) > 0 {
			return recs[0].Data
		}
		// Yield to the scheduler without importing time.
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	t.Fatal("timed out waiting for log record; spy was never invoked — check config.InitPreProcessors")
	return nil
}

// Test_LogEntry_TimestampFormat_RFC3339 verifies that the default time format
// (time.RFC3339) produces a valid RFC 3339 timestamp in the "time" JSON field.
// This is a regression guard: the direct AppendFormat path must produce output
// identical in meaning to the former customTime.Format + AppendQuotedString path.
func Test_LogEntry_TimestampFormat_RFC3339(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("ts-rfc3339")
	config.InitPreProcessors(spy)

	entry.NewLogEntry().Info(context.Background(), "test")

	raw := drainTimestampRec(t, spy)

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v — output: %s", err, raw)
	}
	ts, ok := m["time"].(string)
	if !ok {
		t.Fatalf("no 'time' string field in output: %s", raw)
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Fatalf("timestamp %q is not valid RFC3339: %v", ts, err)
	}
}

// Test_LogEntry_TimestampFormat_Custom verifies that a custom time layout
// (date-only "2006-01-02") is honoured and the resulting value parses correctly.
// This guards against the direct AppendFormat path silently ignoring snap.TimeFormat.
func Test_LogEntry_TimestampFormat_Custom(t *testing.T) {
	const layout = "2006-01-02"
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		TimeFormat(layout).
		Build()
	spy := support.NewFakePreProc("ts-custom")
	config.InitPreProcessors(spy)

	entry.NewLogEntry().Info(context.Background(), "test")

	raw := drainTimestampRec(t, spy)

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v — output: %s", err, raw)
	}
	ts, ok := m["time"].(string)
	if !ok {
		t.Fatalf("no 'time' string field in output: %s", raw)
	}
	if _, err := time.Parse(layout, ts); err != nil {
		t.Fatalf("timestamp %q does not match layout %q: %v", ts, layout, err)
	}
}

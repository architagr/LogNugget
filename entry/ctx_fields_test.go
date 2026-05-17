//go:build testing

// Package entry_test provides D-19 tests for the ctx-data sequential loop
// introduced by Story 022. These tests verify that context-extracted fields
// are emitted correctly regardless of whether extra fields are present,
// and that a nil context produces no user/ctx keys in the JSON output.
//
// Parallel-safety note: all sub-tests mutate the process-wide config singleton
// via config.SetContextFieldsParser and config.InitPreProcessors. They are
// sequential sub-tests inside a non-parallel parent, following the same pattern
// as Test_LogEntry_Methods in levels_test.go (issue #54 / story 039).
package entry_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	"github.com/architagr/lognugget/test/support"
)

// resetAndSpyForCtx resets the singleton to JSON/Debug defaults, installs a
// fresh spy, and registers a context parser that returns the provided kv pairs.
// Returned spy captures every pre-processed log event.
func resetAndSpyForCtx(t *testing.T, name string, ctxKV map[string]any) *support.FakePreProc {
	t.Helper()
	config.TestResetConfig()
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	if ctxKV != nil {
		config.SetContextFieldsParser(func(_ context.Context) map[string]any {
			return ctxKV
		})
	}
	spy := support.NewFakePreProc("ctx-spy-" + name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_LogEntry_CtxData groups all D-19 / Story 022 ctx-data tests.
// Each sub-test mutates the global config singleton so they run sequentially —
// same pattern as Test_LogEntry_Methods in levels_test.go (issue #54 / story 039).
//
// Sub-tests covered:
//   - LogWithCtx_NoFields — two ctx keys, zero extra fields: both ctx keys present in output
//   - LogNoCtx_NoFields — nil context, zero extra fields: no user/ctx keys in output
//   - LogCtxAndFields_AllPresent — ctx keys and extra fields both populated: all keys present
func Test_LogEntry_CtxData(t *testing.T) {
	// Test_LogEntry_LogWithCtx_NoFields verifies D-19 acceptance criterion 1:
	// when len(fields)=0 and the context parser returns 2 entries, both ctx
	// fields appear in the JSON output without panic. The sequential loop in
	// logWithSkip must emit every element of ctxData with a leading comma even
	// when the fields slice is empty (no i+x arithmetic involved).
	t.Run("LogWithCtx_NoFields", func(t *testing.T) {
		ctxKV := map[string]any{
			"trace_id": "tid-abc",
			"user_id":  "uid-42",
		}
		spy := resetAndSpyForCtx(t, "ctx-only", ctxKV)

		entry.NewLogEntry().Info(context.Background(), "ctx-only-msg")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["trace_id"] != "tid-abc" {
			t.Errorf("trace_id = %q, want tid-abc; payload: %s", got["trace_id"], raw)
		}
		if got["user_id"] != "uid-42" {
			t.Errorf("user_id = %q, want uid-42; payload: %s", got["user_id"], raw)
		}
		if got["message"] != "ctx-only-msg" {
			t.Errorf("message = %q, want ctx-only-msg; payload: %s", got["message"], raw)
		}
	})

	// Test_LogEntry_LogNoCtx_NoFields verifies D-19 acceptance criterion 2:
	// when ctx is nil (no context parser result) and fields is empty, the JSON
	// output contains only the three mandatory fields (time, level, message) and
	// no user/ctx keys. This guards the nil-ctx early return in setLogContextFields.
	t.Run("LogNoCtx_NoFields", func(t *testing.T) {
		// No context parser registered — ctxKV=nil skips SetContextFieldsParser.
		spy := resetAndSpyForCtx(t, "no-ctx", nil)

		//nolint:staticcheck // nil context is intentional: verifies the nil-ctx guard in setLogContextFields.
		entry.NewLogEntry().Info(nil, "no-ctx-msg")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		// Mandatory fields must be present.
		if got["message"] != "no-ctx-msg" {
			t.Errorf("message = %q, want no-ctx-msg; payload: %s", got["message"], raw)
		}
		// Only the three mandatory keys (time, level, message) must be present.
		// Any extra user/ctx key would be a bug — the nil-ctx guard must prevent it.
		mandatory := map[string]bool{"time": true, "level": true, "message": true}
		for k := range got {
			if !mandatory[k] {
				t.Errorf("unexpected field %q in nil-ctx output; payload: %s", k, raw)
			}
		}
	})

	// Test_LogEntry_LogCtxAndFields_AllPresent verifies D-19 acceptance criterion 3:
	// when both the context parser and extra fields are populated, every key
	// (ctx-derived and caller-supplied) appears in the JSON output. The count of
	// keys in the unmarshalled map must equal: 3 mandatory + 2 ctx + 2 extra = 7.
	t.Run("LogCtxAndFields_AllPresent", func(t *testing.T) {
		ctxKV := map[string]any{
			"span_id":  "sid-001",
			"req_host": "api.example.com",
		}
		spy := resetAndSpyForCtx(t, "ctx-and-fields", ctxKV)

		fields := []model.LogAttr{
			{Key: "alpha", Value: "a"},
			{Key: "beta", Value: "b"},
		}
		entry.NewLogEntry().Info(context.Background(), "both-msg", fields...)

		raw := drainSpy(t, spy)

		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("json.Unmarshal failed: %v\npayload: %s", err, raw)
		}

		// 3 mandatory (time, level, message) + 2 ctx + 2 extra = 7.
		const wantKeys = 7
		if len(m) != wantKeys {
			t.Errorf("key count = %d, want %d; payload: %s", len(m), wantKeys, raw)
		}

		for _, key := range []string{"time", "level", "message", "span_id", "req_host", "alpha", "beta"} {
			if _, ok := m[key]; !ok {
				t.Errorf("key %q absent from JSON; payload: %s", key, raw)
			}
		}
	})
}

//go:build testing

// Package entry_test provides D-16 tests for the comma separator sequencing
// introduced by Story 018. These tests verify that sequential AppendField calls
// produce valid, parseable JSON output with no trailing or double commas, and
// that all fields survive round-trip through the JSON encoder.
//
// Strategy: the JSON encoder wraps the inner payload in braces, so the inner
// content ("key":"value","key2":"value2") is the source of separator correctness.
// All assertions use json.Unmarshal so the test never hard-codes separator
// positions — instead it validates structural correctness. This follows the T-2
// lesson: never assert literal byte patterns; assert semantic outcome.
//
// Parallel-safety note: all sub-tests mutate the process-wide config singleton
// via resetAndSpyForSeparator. They are therefore sequential sub-tests inside a
// non-parallel parent, following the same pattern as Test_LogEntry_Methods in
// levels_test.go (see issue #54 / story 039 for the root-cause race).
package entry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	"github.com/architagr/lognugget/test/support"
)

// resetAndSpyForSeparator resets the singleton to JSON/Debug defaults and
// installs a fresh spy. Returned spy captures every pre-processed log event.
func resetAndSpyForSeparator(t *testing.T, name string) *support.FakePreProc {
	t.Helper()
	config.TestResetConfig()
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("sep-spy-" + name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_LogEntry_SeparatorSequencing groups all D-16 / Story 018 separator tests.
// Each sub-test mutates the global config singleton so they run sequentially —
// same pattern as Test_LogEntry_Methods in levels_test.go (issue #54 / story 039).
//
// Sub-tests covered:
//   - ThreeExtraFields_AllPresentInJSON — multi-field comma sequencing (D-16 criterion 3)
//   - SingleField_NoTrailingComma — no trailing comma after last field (SC4)
//   - ZeroExtraFields_NoTrailingComma — bare message produces clean JSON (SC4)
//   - ErrorField_AppearsWithCorrectSeparation — error branch comma prefix
//   - StaticFields_AppendedWithCorrectSeparation — static-fields comma prefix
//   - ContextFields_AppearsWithCorrectSeparation — ctx-fields comma prefix
func Test_LogEntry_SeparatorSequencing(t *testing.T) {
	// Test_LogEntry_FieldSeparator_Comma verifies D-16 acceptance criterion 3:
	// sequential AppendField calls with comma separators produce valid JSON that
	// contains all three extra fields alongside the mandatory time/level/message
	// fields. The output must unmarshal cleanly via json.Unmarshal (no malformed
	// separator sequences, no duplicate commas, no trailing commas).
	//
	// Before Story 018, logWithSkip built data []string and used strings.Join.
	// After Story 018 that slice is replaced by a single []byte buffer built with
	// sequential AppendField calls; commas are prepended to every field after the
	// first.
	t.Run("ThreeExtraFields_AllPresentInJSON", func(t *testing.T) {
		spy := resetAndSpyForSeparator(t, "three-fields")

		fields := []model.LogAttr{
			{Key: "alpha", Value: "a"},
			{Key: "beta", Value: "b"},
			{Key: "gamma", Value: "c"},
		}
		entry.NewLogEntry().Info(context.Background(), "sep-test", fields...)

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		// Mandatory fields must always be present.
		if got["level"] != "INFO" {
			t.Errorf("level = %q, want INFO; payload: %s", got["level"], raw)
		}
		if got["message"] != "sep-test" {
			t.Errorf("message = %q, want sep-test; payload: %s", got["message"], raw)
		}
		// All three extra fields must survive the comma-sequenced AppendField path.
		for _, key := range []string{"alpha", "beta", "gamma"} {
			if _, ok := got[key]; !ok {
				t.Errorf("field %q absent from JSON; payload: %s", key, raw)
			}
		}
		if got["alpha"] != "a" || got["beta"] != "b" || got["gamma"] != "c" {
			t.Errorf("extra field values wrong; payload: %s", raw)
		}
	})

	// Test_LogEntry_TrailingNoComma verifies D-16 acceptance criterion 5 (SC4):
	// a single-field log produces valid JSON with no trailing commas. A trailing
	// comma would cause json.Unmarshal to return a non-nil error, which the shared
	// parseLogJSON helper surfaces as a test failure.
	//
	// Commas are prepended (not appended) to every field after the first, so the
	// last field in the buffer always ends without a trailing comma regardless of
	// field count.
	t.Run("SingleField_NoTrailingComma", func(t *testing.T) {
		spy := resetAndSpyForSeparator(t, "single-field")

		entry.NewLogEntry().Info(context.Background(), "no-trailing", model.LogAttr{Key: "x", Value: "1"})

		raw := drainSpy(t, spy)
		// parseLogJSON will fatal on any parse error including trailing commas.
		got := parseLogJSON(t, raw)

		if got["message"] != "no-trailing" {
			t.Errorf("message = %q, want no-trailing; payload: %s", got["message"], raw)
		}
		if got["x"] != "1" {
			t.Errorf("field x = %q, want 1; payload: %s", got["x"], raw)
		}
	})

	t.Run("ZeroExtraFields_NoTrailingComma", func(t *testing.T) {
		spy := resetAndSpyForSeparator(t, "zero-fields")

		entry.NewLogEntry().Info(context.Background(), "bare-message")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["message"] != "bare-message" {
			t.Errorf("message = %q, want bare-message; payload: %s", got["message"], raw)
		}
	})

	// Test_LogEntry_ErrorField_Separator verifies that the error field is correctly
	// comma-separated and present in the JSON output when a non-nil error is passed.
	// This guards the error-append branch of logWithSkip which must prepend a comma
	// before AppendField for the error key (D-16 criterion 3).
	t.Run("ErrorField_AppearsWithCorrectSeparation", func(t *testing.T) {
		spy := resetAndSpyForSeparator(t, "error-field")

		sentinel := errors.New("separator-error")
		entry.NewLogEntry().Error(context.Background(), sentinel, "err-sep-test")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["error"] != sentinel.Error() {
			t.Errorf("error = %q, want %q; payload: %s", got["error"], sentinel.Error(), raw)
		}
		if got["message"] != "err-sep-test" {
			t.Errorf("message = %q, want err-sep-test; payload: %s", got["message"], raw)
		}
	})

	// Test_LogEntry_StaticFields_Separator verifies that static fields appended via
	// config.SetStaticEnvFieldsParser are correctly separated from dynamic fields.
	// A missing or double comma before the static fields segment would make the
	// combined JSON unparseable (D-16 criterion 4 / SC4).
	t.Run("StaticFields_AppendedWithCorrectSeparation", func(t *testing.T) {
		config.TestResetConfig()
		support.NewConfigBuilder(t).
			MinLevel(enum.LevelDebug).
			Encoder(enum.EncoderJSON).
			Build()
		// Register a static field parser that emits one key; it must appear in
		// the final JSON separated correctly from the dynamic fields.
		config.SetStaticEnvFieldsParser(func() map[string]any {
			return map[string]any{"env": "prod"}
		})
		spy := support.NewFakePreProc("sep-spy-static")
		config.InitPreProcessors(spy)

		entry.NewLogEntry().Info(context.Background(), "static-sep")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["env"] != "prod" {
			t.Errorf("env = %q, want prod; payload: %s", got["env"], raw)
		}
		if got["message"] != "static-sep" {
			t.Errorf("message = %q, want static-sep; payload: %s", got["message"], raw)
		}
	})

	// Test_LogEntry_ContextFields_Separator verifies that context-extracted fields
	// returned by setLogContextFields are correctly comma-separated when appended to
	// the dst buffer. Before Story 018 each string was an element in data []string
	// joined via strings.Join; after Story 018 each string is appended with a
	// leading comma byte (D-16 criterion 3).
	t.Run("ContextFields_AppearsWithCorrectSeparation", func(t *testing.T) {
		config.TestResetConfig()
		support.NewConfigBuilder(t).
			MinLevel(enum.LevelDebug).
			Encoder(enum.EncoderJSON).
			Build()
		config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
			return map[string]any{"request_id": "req-123"}
		})
		spy := support.NewFakePreProc("sep-spy-ctx")
		config.InitPreProcessors(spy)

		entry.NewLogEntry().Info(context.Background(), "ctx-sep")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["request_id"] != "req-123" {
			t.Errorf("request_id = %q, want req-123; payload: %s", got["request_id"], raw)
		}
		if got["message"] != "ctx-sep" {
			t.Errorf("message = %q, want ctx-sep; payload: %s", got["message"], raw)
		}
	})
}

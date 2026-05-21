//go:build testing

// Package entry_test covers P2 chain methods on *LogEntry: Str, Int, Uint,
// Float64, Bool. These methods write to an internal pendingBuf that is flushed
// into the log body before context fields are appended, allowing a zero-alloc
// fluent API: entry.NewLogEntry().Str("k","v").Int("n",1).Info(ctx, "msg").
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

// parseRaw unmarshals a log line into a map of raw JSON values (strings,
// numbers, booleans) for assertions on typed fields that are not quoted.
func parseRaw(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed on log payload: %v\npayload: %s", err, data)
	}
	return m
}

// rawStr extracts a string value from a RawMessage map. Fails the test if the
// key is absent or the value is not a JSON string.
func rawStr(t *testing.T, m map[string]json.RawMessage, key string) string {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("key %q absent from JSON", key)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("key %q: expected string, got %s: %v", key, raw, err)
	}
	return s
}

// rawNum extracts the raw number bytes for a key (e.g. "3", "99", "1.5").
func rawNum(t *testing.T, m map[string]json.RawMessage, key string) string {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("key %q absent from JSON", key)
	}
	return string(raw)
}

// rawBool extracts a bool value from a RawMessage map.
func rawBool(t *testing.T, m map[string]json.RawMessage, key string) bool {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("key %q absent from JSON", key)
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("key %q: expected bool, got %s: %v", key, raw, err)
	}
	return b
}

// resetAndSpyForTyped resets the singleton to JSON/Debug defaults and installs
// a fresh FakePreProc. Returns the spy for subsequent assertions.
func resetAndSpyForTyped(t *testing.T, name string) *support.FakePreProc {
	t.Helper()
	config.TestResetConfig()
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("typed-spy-" + name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_LogEntry_TypedFields groups all P2 chain-method tests.
// Sequential sub-tests: each mutates the global config singleton.
func Test_LogEntry_TypedFields(t *testing.T) {

	// Test_Str_ChainReturn verifies that Str returns *LogEntry (allowing chaining)
	// and that chaining Str + Int produces both fields in the JSON output.
	t.Run("Str_ChainReturn", func(t *testing.T) {
		spy := resetAndSpyForTyped(t, "str-chain")

		entry.NewLogEntry().Str("greet", "hi").Int("count", 3).Info(context.Background(), "chain-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "greet") != "hi" {
			t.Errorf("greet want hi; payload: %s", payload)
		}
		if rawNum(t, m, "count") != "3" {
			t.Errorf("count want 3; payload: %s", payload)
		}
		if rawStr(t, m, "message") != "chain-msg" {
			t.Errorf("message want chain-msg; payload: %s", payload)
		}
	})

	// Test_Uint_Chain verifies that Uint writes an unquoted uint64 in the JSON output.
	t.Run("Uint_Chain", func(t *testing.T) {
		spy := resetAndSpyForTyped(t, "uint-chain")

		entry.NewLogEntry().Uint("uid", 99).Info(context.Background(), "u-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawNum(t, m, "uid") != "99" {
			t.Errorf("uid want 99; payload: %s", payload)
		}
	})

	// Test_Float64_Chain verifies that Float64 writes a JSON number.
	t.Run("Float64_Chain", func(t *testing.T) {
		spy := resetAndSpyForTyped(t, "float-chain")

		entry.NewLogEntry().Float64("ratio", 1.5).Info(context.Background(), "f-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawNum(t, m, "ratio") != "1.5" {
			t.Errorf("ratio want 1.5; payload: %s", payload)
		}
	})

	// Test_Bool_Chain verifies that Bool writes an unquoted JSON boolean.
	t.Run("Bool_Chain", func(t *testing.T) {
		spy := resetAndSpyForTyped(t, "bool-chain")

		entry.NewLogEntry().Bool("ok", true).Info(context.Background(), "b-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if !rawBool(t, m, "ok") {
			t.Errorf("ok want true; payload: %s", payload)
		}
	})

	// Test_BackwardCompat_LogAttrStructLiteral verifies that the legacy
	// struct-literal form (LogAttr{Key:k, Value:v}) still produces the same
	// output it did before P2.
	t.Run("BackwardCompat_LogAttrStructLiteral", func(t *testing.T) {
		spy := resetAndSpyForTyped(t, "compat")

		entry.NewLogEntry().Info(context.Background(), "compat-msg",
			model.LogAttr{Key: "legacy", Value: "val"},
		)

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "legacy") != "val" {
			t.Errorf("legacy want val; payload: %s", payload)
		}
	})

	// Test_ChainAndFields_AllPresent verifies that chain methods and variadic
	// fields args coexist: both sets appear in the JSON output.
	t.Run("ChainAndFields_AllPresent", func(t *testing.T) {
		spy := resetAndSpyForTyped(t, "chain-and-fields")

		entry.NewLogEntry().Str("chain_key", "chain_val").Info(
			context.Background(), "mixed-msg",
			model.Str("field_key", "field_val"),
		)

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "chain_key") != "chain_val" {
			t.Errorf("chain_key want chain_val; payload: %s", payload)
		}
		if rawStr(t, m, "field_key") != "field_val" {
			t.Errorf("field_key want field_val; payload: %s", payload)
		}
	})
}

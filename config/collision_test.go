// Package config_test exercises the collision-prefix logic in
// ValidateandParseLogField. These tests cover TS-12: reserved keys must be
// prefixed with DefaultPrefix ("custom.") when a caller provides them as
// user-defined field keys.
package config_test

import (
	"strings"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// reservedByDefault are the five core log keys that must always be treated as
// reserved, even before any SetDefaultFields call is made. The stored string
// is the current default field-name value (identity mapping at startup).
var reservedByDefault = []string{
	string(enum.DefaultLogKeyTime),
	string(enum.DefaultLogKeyLevel),
	string(enum.DefaultLogKeyMessage),
	string(enum.DefaultLogKeyError),
	string(enum.DefaultLogKeyCaller),
}

// nonReserved are field keys that must never be prefixed.
var nonReserved = []string{
	"user_id",
	"request_id",
	"service",
	"host",
	"custom_field",
}

// Test_ValidateAndParse_PrefixesCollidingKey asserts that user-provided keys
// that match a reserved field name are rewritten with the "custom." prefix,
// while non-reserved keys pass through unchanged.
//
// Table structure:
//
//	input key      -> expected output prefix present?
func Test_ValidateAndParse_PrefixesCollidingKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func() // optional config mutation before assertion
		teardown    func() // optional cleanup
		inputKey    string
		inputValue  any
		wantPrefixed bool // true → output should contain "custom.<key>"
	}{
		// --- Built-in defaults (no SetDefaultFields call needed) ---
		{
			name:         "reserved_time_is_prefixed",
			inputKey:     "time",
			inputValue:   "2026-01-01",
			wantPrefixed: true,
		},
		{
			name:         "reserved_level_is_prefixed",
			inputKey:     "level",
			inputValue:   "info",
			wantPrefixed: true,
		},
		{
			name:         "reserved_message_is_prefixed",
			inputKey:     "message",
			inputValue:   "hello",
			wantPrefixed: true,
		},
		{
			name:         "reserved_error_is_prefixed",
			inputKey:     "error",
			inputValue:   "something failed",
			wantPrefixed: true,
		},
		{
			name:         "reserved_caller_is_prefixed",
			inputKey:     "caller",
			inputValue:   "pkg.Func",
			wantPrefixed: true,
		},
		// --- Non-reserved keys pass through unchanged ---
		{
			name:         "non_reserved_user_id_not_prefixed",
			inputKey:     "user_id",
			inputValue:   "abc123",
			wantPrefixed: false,
		},
		{
			name:         "non_reserved_request_id_not_prefixed",
			inputKey:     "request_id",
			inputValue:   "req-42",
			wantPrefixed: false,
		},
		{
			name:         "non_reserved_service_not_prefixed",
			inputKey:     "service",
			inputValue:   "auth",
			wantPrefixed: false,
		},
		// --- After SetDefaultFields renames a key, the NEW name is reserved ---
		{
			name:  "renamed_time_key_new_name_is_prefixed",
			setup: func() { config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyTime: "ts"}) },
			teardown: func() {
				// Restore: call SetDefaultFields to reset time key back to "time"
				config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyTime: "time"})
			},
			inputKey:     "ts",
			inputValue:   "2026-01-01",
			wantPrefixed: true,
		},
		{
			name:  "renamed_time_key_old_name_no_longer_prefixed",
			setup: func() { config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyTime: "ts"}) },
			teardown: func() {
				config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyTime: "time"})
			},
			inputKey:     "time", // "time" was the old name; "ts" is now the reserved name
			inputValue:   "2026-01-01",
			wantPrefixed: false,
		},
		{
			name:  "renamed_level_key_new_name_is_prefixed",
			setup: func() { config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyLevel: "severity"}) },
			teardown: func() {
				config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyLevel: "level"})
			},
			inputKey:     "severity",
			inputValue:   "warn",
			wantPrefixed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Not parallel: tests that call setup/teardown mutate global config.
			if tc.setup != nil {
				tc.setup()
			}
			if tc.teardown != nil {
				defer tc.teardown()
			}

			got := config.ValidateandParseLogField(tc.inputKey, tc.inputValue)

			prefixedKey := config.DefaultPrefix + tc.inputKey
			if tc.wantPrefixed {
				if !strings.Contains(got, "\""+prefixedKey+"\"") {
					t.Errorf("ValidateandParseLogField(%q, _) = %q; want key prefixed as %q",
						tc.inputKey, got, prefixedKey)
				}
			} else {
				// The raw key should appear; the prefixed form must NOT.
				if strings.Contains(got, "\""+prefixedKey+"\"") {
					t.Errorf("ValidateandParseLogField(%q, _) = %q; key must not be prefixed (want plain %q)",
						tc.inputKey, got, tc.inputKey)
				}
				if !strings.Contains(got, "\""+tc.inputKey+"\"") {
					t.Errorf("ValidateandParseLogField(%q, _) = %q; want plain key %q in output",
						tc.inputKey, got, tc.inputKey)
				}
			}
		})
	}
}

// Test_ValidateAndParse_ReservedKeysPopulatedAtInit verifies that the
// restricted-field set is populated from built-in defaults without any
// explicit SetDefaultFields call. This guards against the D-8 defect where
// the slice was only filled after SetDefaultFields was invoked.
func Test_ValidateAndParse_ReservedKeysPopulatedAtInit(t *testing.T) {
	t.Parallel()

	for _, key := range reservedByDefault {
		key := key // capture
		t.Run("reserved_at_init_"+key, func(t *testing.T) {
			t.Parallel()
			got := config.ValidateandParseLogField(key, "v")
			prefixedKey := config.DefaultPrefix + key
			if !strings.Contains(got, "\""+prefixedKey+"\"") {
				t.Errorf("ValidateandParseLogField(%q, _) = %q; reserved key must be prefixed as %q even without SetDefaultFields",
					key, got, prefixedKey)
			}
		})
	}
}

// Test_ValidateAndParse_NonReservedKeysNotPrefixedAtInit ensures that field
// keys that are not in the built-in restricted set are never silently
// prefixed before any SetDefaultFields call.
func Test_ValidateAndParse_NonReservedKeysNotPrefixedAtInit(t *testing.T) {
	t.Parallel()

	for _, key := range nonReserved {
		key := key
		t.Run("not_reserved_at_init_"+key, func(t *testing.T) {
			t.Parallel()
			got := config.ValidateandParseLogField(key, "v")
			prefixedKey := config.DefaultPrefix + key
			if strings.Contains(got, "\""+prefixedKey+"\"") {
				t.Errorf("ValidateandParseLogField(%q, _) = %q; non-reserved key must not be prefixed",
					key, got)
			}
		})
	}
}

// Benchmark_ValidateField measures O(1) map-lookup cost of
// ValidateandParseLogField against a reserved key ("time") and a non-reserved
// key ("user_id"). The budget is < 20 ns / 0 alloc per call.
//
// Input shape: single call with a 4–7 byte key and a short string value,
// simulating the hot path for context-field collision checks.
// Budget: < 20 ns per operation, 0 allocations (excluding string builder
// growth, which is pre-grown to 100 bytes).
func Benchmark_ValidateField(b *testing.B) {
	b.Run("reserved_key", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = config.ValidateandParseLogField("time", "2026-01-01")
		}
	})
	b.Run("non_reserved_key", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = config.ValidateandParseLogField("user_id", "abc123")
		}
	})
}

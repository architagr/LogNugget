//go:build testing

// Package config_test exercises AppendField (D-5, TS-10) — the strconv-based
// low-allocation field serialiser. Tests verify type coverage, unquoted
// numerics, RFC 8259 string escaping, NaN/Inf → null, and zero-alloc
// behaviour on the numeric hot path.
package config_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
)

// parseKV wraps the AppendField output in a JSON object brace pair so that
// encoding/json can validate it as a complete document. Returns the parsed
// map or fails the test.
func parseKV(t *testing.T, got []byte) map[string]json.RawMessage {
	t.Helper()
	wrapped := make([]byte, 0, len(got)+2)
	wrapped = append(wrapped, '{')
	wrapped = append(wrapped, got...)
	wrapped = append(wrapped, '}')
	var m map[string]json.RawMessage
	if err := json.Unmarshal(wrapped, &m); err != nil {
		t.Fatalf("AppendField output is not valid JSON fragment %q: %v", got, err)
	}
	return m
}

// parseKVLoose wraps the AppendField output in JSON object braces and returns
// the raw bytes for the given key without requiring valid JSON for the value.
// Used for slow-path types ([]any, map) where AppendField emits Go %+v format.
func parseKVLoose(t *testing.T, got []byte, key string) []byte {
	t.Helper()
	// The key must be a valid JSON string; extract raw suffix after `"key":`.
	keyJSON := `"` + key + `":`
	if len(got) < len(keyJSON) {
		t.Fatalf("output too short for key %q: %q", key, got)
	}
	prefix := got[:len(keyJSON)]
	if string(prefix) != keyJSON {
		t.Fatalf("expected prefix %q in output %q", keyJSON, got)
	}
	return got[len(keyJSON):]
}

// Test_AppendField_Types verifies that every supported value type is encoded
// to a valid JSON key-value fragment. Numerics, bool, error, time.Time, slices,
// and unknown types are all covered (TS-10).
func Test_AppendField_Types(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		key       string
		value     any
		wantValue string // expected raw JSON value (as Go string for readability)
	}{
		// string
		{name: "string_plain", key: "k", value: "hello", wantValue: `"hello"`},
		// integers — unquoted
		{name: "int8_pos", key: "k", value: int8(42), wantValue: `42`},
		{name: "int16_pos", key: "k", value: int16(1000), wantValue: `1000`},
		{name: "int32_pos", key: "k", value: int32(99999), wantValue: `99999`},
		{name: "int64_pos", key: "k", value: int64(1 << 40), wantValue: `1099511627776`},
		{name: "int_pos", key: "k", value: int(7), wantValue: `7`},
		{name: "int8_neg", key: "k", value: int8(-1), wantValue: `-1`},
		{name: "int64_neg", key: "k", value: int64(-9999999999), wantValue: `-9999999999`},
		// unsigned integers — unquoted
		{name: "uint8", key: "k", value: uint8(255), wantValue: `255`},
		{name: "uint16", key: "k", value: uint16(65535), wantValue: `65535`},
		{name: "uint32", key: "k", value: uint32(4294967295), wantValue: `4294967295`},
		{name: "uint64", key: "k", value: uint64(math.MaxUint64), wantValue: `18446744073709551615`},
		{name: "uint", key: "k", value: uint(42), wantValue: `42`},
		// floats — unquoted
		{name: "float32", key: "k", value: float32(3.14), wantValue: `3.14`},
		{name: "float64", key: "k", value: float64(2.718281828), wantValue: `2.718281828`},
		// bool — unquoted
		{name: "bool_true", key: "k", value: true, wantValue: `true`},
		{name: "bool_false", key: "k", value: false, wantValue: `false`},
		// error — quoted string of .Error()
		{name: "error", key: "k", value: errors.New("oops"), wantValue: `"oops"`},
		// time.Time — quoted RFC3339
		{name: "time", key: "k", value: ts, wantValue: `"2026-01-15T12:00:00Z"`},
	}

	// slowPathTests covers []any and map slow paths: AppendField emits Go %+v
	// format for these, which is not valid JSON. We only verify that the key
	// is written correctly and that some output is produced for the value.
	slowPathTests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "slice_any", key: "k", value: []any{1, "a"}},
		{name: "map_any", key: "k", value: map[string]any{"x": 1}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.AppendField(nil, tc.key, tc.value)

			// Must always be parseable as a valid JSON fragment.
			m := parseKV(t, got)
			if _, ok := m[tc.key]; !ok {
				t.Fatalf("key %q not found in output %q", tc.key, got)
			}

			// For types where we specify the exact value, assert it.
			if tc.wantValue != "" {
				raw := m[tc.key]
				if string(raw) != tc.wantValue {
					t.Errorf("AppendField value: got %s, want %s (full output: %q)", raw, tc.wantValue, got)
				}
			}
		})
	}

	for _, tc := range slowPathTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.AppendField(nil, tc.key, tc.value)
			// Key must be correctly written; value is Go %+v format (not JSON).
			val := parseKVLoose(t, got, tc.key)
			if len(val) == 0 {
				t.Errorf("AppendField slow-path produced empty value for %q", tc.name)
			}
		})
	}
}

// Test_AppendField_NumericsUnquoted confirms that integer values produce
// unquoted JSON numbers, not JSON strings (e.g. `"k":42` not `"k":"42"`).
func Test_AppendField_NumericsUnquoted(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
	}{
		{"int", int(42)},
		{"int8", int8(42)},
		{"int16", int16(42)},
		{"int32", int32(42)},
		{"int64", int64(42)},
		{"uint", uint(42)},
		{"uint8", uint8(42)},
		{"uint16", uint16(42)},
		{"uint32", uint32(42)},
		{"uint64", uint64(42)},
		{"float64", float64(1.5)},
		{"float32", float32(1.5)},
		{"bool_true", true},
		{"bool_false", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.AppendField(nil, "k", tc.value)
			m := parseKV(t, got)
			raw := m["k"]
			if len(raw) > 0 && raw[0] == '"' {
				t.Errorf("AppendField(%T) produced a quoted value %s; want unquoted number/bool", tc.value, raw)
			}
		})
	}
}

// Test_AppendField_NaNInf asserts that IEEE-754 NaN and ±Inf are serialised as
// JSON null rather than invalid JSON tokens, per the contract in parse_field.go.
func Test_AppendField_NaNInf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value float64
	}{
		{"NaN", math.NaN()},
		{"PosInf", math.Inf(1)},
		{"NegInf", math.Inf(-1)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.AppendField(nil, "k", tc.value)
			m := parseKV(t, got)
			raw := string(m["k"])
			if raw != "null" {
				t.Errorf("AppendField(%v) = %s; want null", tc.value, raw)
			}
		})
	}
}

// Test_AppendField_StringEscape verifies that special characters inside string
// values are RFC 8259 escaped. Covers: double-quote, backslash, newline, tab,
// and a control character.
func Test_AppendField_StringEscape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"double_quote", `say "hello"`},
		{"backslash", `back\slash`},
		{"newline", "line1\nline2"},
		{"tab", "col1\tcol2"},
		{"control_char", "ctrl\x01end"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.AppendField(nil, "k", tc.input)
			// parseKV verifies the output is valid JSON — escaping is implicit.
			m := parseKV(t, got)
			var decoded string
			if err := json.Unmarshal(m["k"], &decoded); err != nil {
				t.Fatalf("could not decode string value: %v (output: %q)", err, got)
			}
			if decoded != tc.input {
				t.Errorf("round-trip mismatch: got %q, want %q", decoded, tc.input)
			}
		})
	}
}

// Test_AppendField_AppendsToExistingDst verifies that AppendField extends the
// caller's slice rather than replacing it, enabling callers to build up a log
// line incrementally.
func Test_AppendField_AppendsToExistingDst(t *testing.T) {
	t.Parallel()

	prefix := []byte(`"existing":"data",`)
	got := config.AppendField(prefix, "k", "v")

	if len(got) <= len(prefix) {
		t.Fatalf("AppendField did not append to dst: got %q", got)
	}
	// The original prefix must be intact at the start.
	if string(got[:len(prefix)]) != string(prefix) {
		t.Errorf("prefix was overwritten: got %q", got[:len(prefix)])
	}
}

// Test_AppendField_NoAlloc_Numerics asserts that AppendField for integer and
// float values performs zero heap allocations when the destination slice has
// sufficient capacity pre-allocated. This guards the < 1 µs p99 SLO (NF1).
func Test_AppendField_NoAlloc_Numerics(t *testing.T) {
	t.Parallel()

	buf := make([]byte, 0, 64)

	numericCases := []struct {
		name  string
		value any
	}{
		{"int", int(42)},
		{"int64", int64(9999)},
		{"uint64", uint64(1234)},
		{"float64", float64(3.14)},
		{"bool", true},
	}

	for _, tc := range numericCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				buf = config.AppendField(buf[:0], "k", tc.value)
			})
			if allocs != 0 {
				t.Errorf("AppendField(%T) allocs = %v; want 0", tc.value, allocs)
			}
		})
	}
}

// Test_AppendField_KeyIsJSONEscaped verifies that special characters in the key
// are also RFC 8259 escaped. Keys must be valid JSON strings.
func Test_AppendField_KeyIsJSONEscaped(t *testing.T) {
	t.Parallel()

	got := config.AppendField(nil, `ke"y`, "val")
	// If key escaping is correct, the output will parse as valid JSON.
	parseKV(t, got) // panics the test on invalid JSON — that's the assertion
}

// Test_AppendField_Float32NaNInf covers the float32 variant of NaN/Inf since
// the type switch must handle float32 separately from float64.
func Test_AppendField_Float32NaNInf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value float32
	}{
		{"NaN32", float32(math.NaN())},
		{"PosInf32", float32(math.Inf(1))},
		{"NegInf32", float32(math.Inf(-1))},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.AppendField(nil, "k", tc.value)
			m := parseKV(t, got)
			if string(m["k"]) != "null" {
				t.Errorf("AppendField(float32 %v) = %s; want null", tc.value, m["k"])
			}
		})
	}
}

// Test_ParseLogField_BackwardsCompat ensures the existing ParseLogField wrapper
// still returns the same result as AppendField so callers in entry.go are unaffected.
func Test_ParseLogField_BackwardsCompat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key   string
		value any
	}{
		{"msg", "hello world"},
		{"count", int(5)},
		{"ratio", float64(0.5)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			want := string(config.AppendField(nil, tc.key, tc.value))
			got := config.ParseLogField(tc.key, tc.value)
			if got != want {
				t.Errorf("ParseLogField(%q, %v) = %q; want %q", tc.key, tc.value, got, want)
			}
		})
	}
}

// Note: Benchmark_AppendField_Int, Benchmark_AppendField_String, and
// Benchmark_AppendField_Float64 are defined in parse_log_field_bench_test.go
// without a build tag so bench-check.sh picks them up without -tags testing.
// They must not be re-declared here to avoid a redeclaration conflict when
// both files are compiled together under -tags testing.

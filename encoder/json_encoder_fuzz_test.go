// Package encoder_test provides fuzz testing for JSONEncoder.Append to verify
// that any key-value pair produced by config.AppendField yields valid JSON when
// wrapped by JSONEncoder. It is an external test package to avoid the import
// cycle that would arise from importing config inside the encoder package
// (config itself imports encoder).
package encoder_test

// FuzzJSONEncoder is the fuzz entry point for TS-17. The seed corpus lives in
// testdata/fuzz/FuzzJSONEncoder/ and is replayed on every PR via:
//
//	go test ./encoder
//
// To run active fuzzing (CI nightly, NOT per-PR):
//
//	go test -fuzz=FuzzJSONEncoder -fuzztime=30s ./encoder
//
// The fuzz invariant: for any (key, value string) pair, calling
// config.AppendField to produce a body and then JSONEncoder.Append to wrap it
// must yield a byte slice (minus the trailing newline) that parses as a valid
// JSON object via json.Unmarshal.
//
// NaN/Inf edge case: when value is not provided as a floating-point type
// (strings are always quoted), this code path never emits null from
// AppendField. Float NaN/Inf → null handling is covered separately in
// config/parse_log_field_test.go.

import (
	"encoding/json"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/encoder"
)

// FuzzJSONEncoder verifies that JSONEncoder.Append always produces valid JSON
// for any (key, value string) pair processed through config.AppendField.
// Seeds cover: empty inputs, control characters, unicode, escape sequences,
// long strings, JSON-special characters, and invalid UTF-8 bytes.
func FuzzJSONEncoder(f *testing.F) {
	// Seed corpus: representative inputs that exercise distinct code paths in
	// escapeJSON and appendJSONString. The testdata/fuzz/FuzzJSONEncoder/
	// directory provides 50 additional hand-picked seeds replayed automatically.
	f.Add("msg", "hello world")
	f.Add("msg", "")
	f.Add("", "empty key")
	f.Add("", "")
	f.Add("n", "42")
	f.Add("key", `say "hello"`)
	f.Add("path", `C:\Users\foo`)
	f.Add("msg", "line1\nline2")
	f.Add("msg", "col1\tcol2")
	f.Add("url", "https://example.com/path")
	f.Add("count", "12345")
	f.Add("level", "error")
	f.Add("html", "<script>alert(1)</script>")
	f.Add("trace", "panic: nil pointer\n\tgoroutine 1 [running]")
	f.Add("error", "connection refused: dial tcp 127.0.0.1:5432")

	enc := encoder.NewJSONEncoder()

	f.Fuzz(func(t *testing.T, key, value string) {
		// Build a pre-rendered body fragment "key":"value" using the same
		// AppendField path that production log entries use.
		body := config.AppendField(nil, key, value)

		// Wrap in JSON object braces and trailing newline (ARCH-14).
		result := enc.Append(nil, body)

		// Strip the mandatory trailing newline before unmarshalling.
		// why: json.Unmarshal does not accept trailing whitespace from all
		// decoders; stripping the single '\n' appended by Append is safer
		// than relying on decoder leniency.
		if len(result) == 0 {
			t.Fatal("Append returned empty slice")
		}
		jsonBytes := result[:len(result)-1]

		// The round-trip invariant: any key-value pair must produce a valid
		// JSON object. A string value via AppendField always emits a quoted
		// string, so the result is always {"key":"value"} — never null.
		var m map[string]any
		if err := json.Unmarshal(jsonBytes, &m); err != nil {
			t.Fatalf("invalid JSON: %v\nkey:    %q\nvalue:  %q\nresult: %s", err, key, value, result)
		}

		// Structural integrity: the parsed object must contain exactly one key.
		// why: AppendField emits exactly one key-value fragment; wrapping it in
		// braces must produce a single-key object.
		if len(m) != 1 {
			t.Fatalf("expected 1 key in JSON object, got %d\nkey: %q\nresult: %s", len(m), key, result)
		}
	})
}

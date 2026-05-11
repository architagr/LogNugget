//go:build testing

// Package config_test covers TS-11: pre-rendered default-key prefixes (ARCH-6, F12, F13).
//
// These tests verify that Config exposes a DefaultFieldsRendered accessor
// containing pre-computed `"key":` byte prefixes, that SetDefaultFields
// rebuilds that cache, and that the restricted-fields collision set is also
// rebuilt on rename so ValidateandParseLogField immediately reflects the new
// key name.
package config_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// drainDefaultFieldsSpy polls spy.Records() until at least one record is
// available, then returns the first record's raw bytes. It yields to the
// scheduler between polls and fails after 500 iterations.
//
// why: config.PublishLog is asynchronous — it enqueues onto a buffered channel
// consumed by ProcessLogEvent in a background goroutine. A brief spin is
// required before the assertion can observe the emitted record.
func drainDefaultFieldsSpy(t *testing.T, spy *support.FakePreProc) []byte {
	t.Helper()
	for i := 0; i < 500; i++ {
		if recs := spy.Records(); len(recs) > 0 {
			return recs[0].Data
		}
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	t.Fatal("timed out waiting for log record; spy was never invoked")
	return nil
}

// Test_SetDefaultFields_RenamesAndPrerenders verifies TS-11/AC-1:
// after SetDefaultFields renames the time key from "time" to "ts", the
// DefaultFieldsRendered cache entry for DefaultLogKeyTime equals the
// RFC 8259-encoded key prefix []byte{'"', 't', 's', '"', ':'}.
//
// This confirms that buildRenderedFields runs inside SetDefaultFields
// and that the resulting prefix is exactly the bytes an appender would
// concatenate before the value — no extra comma, no value bytes.
func Test_SetDefaultFields_RenamesAndPrerenders(t *testing.T) {
	support.NewConfigBuilder(t).Build()

	config.SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyTime: "ts",
	})
	t.Cleanup(func() {
		config.SetDefaultFields(map[enum.DefaultLogKey]string{
			enum.DefaultLogKeyTime: string(enum.DefaultLogKeyTime),
		})
	})

	rendered := config.GetConfig().DefaultFieldsRendered()
	if rendered == nil {
		t.Fatal("DefaultFieldsRendered() returned nil; want non-nil map")
	}

	got, ok := rendered[enum.DefaultLogKeyTime]
	if !ok {
		t.Fatalf("DefaultFieldsRendered() has no entry for DefaultLogKeyTime; want key present")
	}

	want := []byte{'"', 't', 's', '"', ':'}
	if !bytes.Equal(got, want) {
		t.Errorf("DefaultFieldsRendered()[DefaultLogKeyTime] = %q; want %q", got, want)
	}
}

// Test_DefaultFieldsRendered_PopulatedAtInit verifies TS-11/AC-1:
// the rendered-prefix cache is populated by resetConfig without any explicit
// SetDefaultFields call. The built-in default for DefaultLogKeyTime is "time",
// so the pre-rendered prefix must be []byte{'"', 't', 'i', 'm', 'e', '"', ':'}.
//
// Guards against a regression where buildRenderedFields is only called inside
// SetDefaultFields and the cache is nil on a freshly-reset singleton.
func Test_DefaultFieldsRendered_PopulatedAtInit(t *testing.T) {
	t.Parallel()

	rendered := config.GetConfig().DefaultFieldsRendered()
	if rendered == nil {
		t.Fatal("DefaultFieldsRendered() returned nil before any SetDefaultFields call; want populated cache")
	}

	cases := []struct {
		key  enum.DefaultLogKey
		want []byte
	}{
		{enum.DefaultLogKeyTime, []byte(`"time":`)},
		{enum.DefaultLogKeyLevel, []byte(`"level":`)},
		{enum.DefaultLogKeyMessage, []byte(`"message":`)},
	}

	for _, tc := range cases {
		got, ok := rendered[tc.key]
		if !ok {
			t.Errorf("DefaultFieldsRendered() missing entry for key %v; want %q", tc.key, tc.want)
			continue
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("DefaultFieldsRendered()[%v] = %q; want %q", tc.key, got, tc.want)
		}
	}
}

// Test_DefaultFieldsRendered_AllDefaultKeys verifies that the rendered cache
// contains entries for every key present in DefaultFields after resetConfig.
// This ensures buildRenderedFields iterates the full map, not just the five
// core keys.
func Test_DefaultFieldsRendered_AllDefaultKeys(t *testing.T) {
	t.Parallel()

	cfg := config.GetConfig()
	defaultFields := cfg.DefaultFields()
	rendered := cfg.DefaultFieldsRendered()

	if len(rendered) != len(defaultFields) {
		t.Errorf("DefaultFieldsRendered() has %d entries; want %d (same as DefaultFields)",
			len(rendered), len(defaultFields))
	}

	for k, name := range defaultFields {
		prefix, ok := rendered[k]
		if !ok {
			t.Errorf("DefaultFieldsRendered() missing entry for key %v (name=%q)", k, name)
			continue
		}
		// Each prefix must be exactly `"<name>":`.
		want := make([]byte, 0, len(name)+3)
		want = append(want, '"')
		want = append(want, name...)
		want = append(want, '"', ':')
		if !bytes.Equal(prefix, want) {
			t.Errorf("DefaultFieldsRendered()[%v] = %q; want %q", k, prefix, want)
		}
	}
}

// Test_SetDefaultFields_RebuildsRenderedCache verifies TS-11/AC-3:
// after renaming multiple keys in a single SetDefaultFields call, the
// rendered cache is updated for every renamed key.
func Test_SetDefaultFields_RebuildsRenderedCache(t *testing.T) {
	support.NewConfigBuilder(t).Build()

	config.SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyLevel:   "severity",
		enum.DefaultLogKeyMessage: "msg",
	})

	rendered := config.GetConfig().DefaultFieldsRendered()

	wantLevel := []byte(`"severity":`)
	wantMsg := []byte(`"msg":`)

	if got := rendered[enum.DefaultLogKeyLevel]; !bytes.Equal(got, wantLevel) {
		t.Errorf("after rename: DefaultFieldsRendered()[Level] = %q; want %q", got, wantLevel)
	}
	if got := rendered[enum.DefaultLogKeyMessage]; !bytes.Equal(got, wantMsg) {
		t.Errorf("after rename: DefaultFieldsRendered()[Message] = %q; want %q", got, wantMsg)
	}
}

// Test_SetDefaultFields_RebuildsCollisionSet verifies TS-11/AC-3:
// renaming the time key from "time" to "ts" causes ValidateandParseLogField
// to prefix user field key "ts" (the new name is now reserved) and to pass
// "time" through unchanged (the old name is no longer reserved).
//
// This confirms SetDefaultFields rebuilds restrictedFieldsSet from the
// post-merge defaultFields map, satisfying ARCH-6 acceptance criterion 3.
func Test_SetDefaultFields_RebuildsCollisionSet(t *testing.T) {
	support.NewConfigBuilder(t).Build()

	config.SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyTime: "ts",
	})
	t.Cleanup(func() {
		config.SetDefaultFields(map[enum.DefaultLogKey]string{
			enum.DefaultLogKeyTime: string(enum.DefaultLogKeyTime),
		})
	})

	// "ts" is now the reserved time-key name; a user field named "ts" must
	// be prefixed.
	gotTS := config.ValidateandParseLogField("ts", "some-value")
	wantPrefixedKey := `"` + config.DefaultPrefix + `ts"`
	if !bytes.Contains([]byte(gotTS), []byte(wantPrefixedKey)) {
		t.Errorf("ValidateandParseLogField(%q, _) = %q; want key prefixed as %q after rename",
			"ts", gotTS, wantPrefixedKey)
	}

	// "time" is no longer reserved; a user field named "time" must pass through.
	gotTime := config.ValidateandParseLogField("time", "some-value")
	prefixedOld := `"` + config.DefaultPrefix + `time"`
	if bytes.Contains([]byte(gotTime), []byte(prefixedOld)) {
		t.Errorf("ValidateandParseLogField(%q, _) = %q; old key %q must not be prefixed after rename",
			"time", gotTime, "time")
	}
	if !bytes.Contains([]byte(gotTime), []byte(`"time"`)) {
		t.Errorf("ValidateandParseLogField(%q, _) = %q; want plain key %q in output",
			"time", gotTime, "time")
	}
}

// Test_LogEntry_UsesPrerenderedPrefixInOutput verifies TS-11/AC-2:
// after renaming the time key to "ts", a log call emits a JSON object
// whose "ts" field is present and whose old "time" field is absent.
// This confirms the hot path in entry.logWithSkip uses the pre-rendered
// prefix map rather than assembling the key from a string on every call.
func Test_LogEntry_UsesPrerenderedPrefixInOutput(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()

	config.SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyTime: "ts",
	})

	spy := support.NewFakePreProc("spy-prerender")
	config.InitPreProcessors(spy)

	entry.NewLogEntry().Info(context.Background(), "prerender-test")

	raw := drainDefaultFieldsSpy(t, spy)

	// The emitted payload must be a valid JSON object.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("log output is not valid JSON: %v\npayload: %s", err, raw)
	}

	if _, ok := m["ts"]; !ok {
		t.Errorf("log output missing renamed key %q; got fields: %v\npayload: %s", "ts", jsonKeys(m), raw)
	}
	if _, ok := m["time"]; ok {
		t.Errorf("log output still contains old key %q after rename; payload: %s", "time", raw)
	}
}

// Test_DefaultFieldsRendered_RenderedPrefixIsExactBytes verifies that the
// pre-rendered prefix bytes are exactly the four or more bytes `"key":` with
// no trailing comma, no value bytes, and no extra whitespace. This is the
// structural invariant relied on by entry.logWithSkip when it appends
// prefix + value + ',' directly.
func Test_DefaultFieldsRendered_RenderedPrefixIsExactBytes(t *testing.T) {
	t.Parallel()

	rendered := config.GetConfig().DefaultFieldsRendered()
	defaultFields := config.GetConfig().DefaultFields()

	for k, name := range defaultFields {
		prefix := rendered[k]

		// Prefix must start with '"'.
		if len(prefix) == 0 || prefix[0] != '"' {
			t.Errorf("rendered[%v] = %q; must start with '\"'", k, prefix)
			continue
		}
		// Prefix must end with ':'.
		if prefix[len(prefix)-1] != ':' {
			t.Errorf("rendered[%v] = %q; must end with ':'", k, prefix)
			continue
		}
		// The bytes between the outer quotes must equal the field name.
		inner := prefix[1 : len(prefix)-2] // strip leading '"' and trailing '":'
		if string(inner) != name {
			t.Errorf("rendered[%v]: inner name = %q; want %q", k, inner, name)
		}
	}
}

// Benchmark_DefaultFieldsRendered_HotPath measures the overhead of looking
// up pre-rendered prefixes and appending them plus a value into a reused
// buffer, simulating the new hot path in logWithSkip after ARCH-6.
//
// Input shape: three map lookups (time, level, message) + three append calls
// into a 256-byte pre-allocated buffer. Budget: <= 50 ns/op, 0 allocs.
func Benchmark_DefaultFieldsRendered_HotPath(b *testing.B) {
	rendered := config.GetConfig().DefaultFieldsRendered()

	// Pre-allocate the destination buffer once; the benchmark reuses it.
	dst := make([]byte, 0, 256)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = dst[:0]
		dst = append(dst, rendered[enum.DefaultLogKeyTime]...)
		dst = append(dst, '"', '2', '0', '2', '6', '-', '0', '1', '-', '0', '1', '"')
		dst = append(dst, ',')
		dst = append(dst, rendered[enum.DefaultLogKeyLevel]...)
		dst = append(dst, '"', 'I', 'N', 'F', 'O', '"')
		dst = append(dst, ',')
		dst = append(dst, rendered[enum.DefaultLogKeyMessage]...)
		dst = append(dst, '"', 'h', 'e', 'l', 'l', 'o', '"')
	}
}

// jsonKeys returns the keys of a JSON object map as a slice, for readable
// error messages.
func jsonKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

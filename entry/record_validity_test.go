//go:build testing

// Tests that every supported field shape produces a parseable record with no
// duplicate keys.
package entry_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/model"
)

// Test_Record_AnyCompositeValueIsValidJSON asserts that Any() emits parseable
// JSON for values with no typed method.
//
// why: the KindAny fallback appended fmt "%+v" bytes raw, so Any("k",
// []string{"1s","2s"}) produced `"k":[1s 2s]` — the whole line failed to
// parse, taking the other fields with it.
func Test_Record_AnyCompositeValueIsValidJSON(t *testing.T) {
	type payload struct {
		Region string
		Shards int
	}

	cases := []struct {
		name  string
		value any
	}{
		{"string_slice", []string{"1s", "2s", "4s"}},
		{"int_slice", []int{1, 2, 3}},
		{"map", map[string]int{"a": 1}},
		{"struct", payload{Region: "us-east-1", Shards: 4}},
		{"duration", 1500 * time.Millisecond},
		{"nil", nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spy := resetAndSpyForP5(t, "any-"+tc.name)

			entry.NewLogEntry().Any("value", tc.value).Info(context.Background(), "any composite")

			raw := drainSpy(t, spy)
			var into map[string]any
			if err := json.Unmarshal(raw, &into); err != nil {
				t.Fatalf("record is not valid JSON: %v\npayload: %s", err, raw)
			}
			if _, ok := into["value"]; !ok {
				t.Errorf("record has no \"value\" field; payload: %s", raw)
			}
		})
	}
}

// Test_Record_ReservedKeysAreNotDuplicated asserts that a user field named
// after a core key is prefixed instead of emitted twice.
//
// why: the typed chain methods wrote the caller's key verbatim, so
// Str("time", …) produced a record with two "time" members. Which one a
// consumer keeps is parser-dependent, so the record was ambiguous.
func Test_Record_ReservedKeysAreNotDuplicated(t *testing.T) {
	reserved := []string{"time", "level", "message"}

	for _, key := range reserved {
		key := key
		t.Run("chain_"+key, func(t *testing.T) {
			spy := resetAndSpyForP5(t, "reserved-chain-"+key)

			entry.NewLogEntry().Str(key, "user value").Info(context.Background(), "collision")

			raw := drainSpy(t, spy)
			assertSingleKey(t, raw, key)
			if !strings.Contains(string(raw), `"custom.`+key+`":"user value"`) {
				t.Errorf("user field must be prefixed as custom.%s; payload: %s", key, raw)
			}
		})

		t.Run("variadic_"+key, func(t *testing.T) {
			spy := resetAndSpyForP5(t, "reserved-variadic-"+key)

			entry.NewLogEntry().Info(context.Background(), "collision", model.Str(key, "user value"))

			raw := drainSpy(t, spy)
			assertSingleKey(t, raw, key)
		})
	}
}

// assertSingleKey fails when key appears more than once as a JSON member name
// in raw. encoding/json silently keeps one of the duplicates, so the check
// counts occurrences in the bytes instead of in the decoded map.
func assertSingleKey(t *testing.T, raw []byte, key string) {
	t.Helper()
	if n := strings.Count(string(raw), `"`+key+`":`); n != 1 {
		t.Errorf("key %q appears %d times in one record; want exactly 1\npayload: %s", key, n, raw)
	}
}

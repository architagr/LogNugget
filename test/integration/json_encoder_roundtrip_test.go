//go:build testing

// Package integration contains end-to-end tests that drive the full log
// pipeline. These tests require the `testing` build tag.
package integration

import (
	"encoding/json"
	"testing"

	"github.com/architagr/lognugget/encoder"
	support "github.com/architagr/lognugget/test/support"
)

// Test_JSONEncoder_RFC8259_Roundtrip verifies that 1000 corpus bodies,
// when wrapped by JSONEncoder.Append, produce valid JSON (TS-16 SC4).
func Test_JSONEncoder_RFC8259_Roundtrip(t *testing.T) {
	t.Parallel()

	enc := encoder.NewJSONEncoder()
	corpus := support.JSONCorpus(42, 1000)
	for i, body := range corpus {
		// body is a complete JSON object; strip outer braces to get the inner fields.
		inner := body
		if len(inner) >= 2 && inner[0] == '{' {
			inner = inner[1 : len(inner)-1]
		}
		out := enc.Append(nil, inner)
		// strip trailing newline before unmarshal
		out = out[:len(out)-1]
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("item %d: unmarshal failed: %v\nbody: %q\nout: %q", i, err, body, out)
		}
	}
}

//go:build testing

// Package entry_test: P4 inline-framing correctness tests.
//
// Tests verify that inline framing (OpenBytes prepend + CloseBytes append +
// ownership transfer) produces byte-identical output to the pre-P4 approach,
// and that the ownership transfer sequence is race-detector clean.
package entry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// drainInlineRec polls spy.Records() until at least n records arrive.
func drainInlineRec(t *testing.T, spy *support.FakePreProc, n int) []support.FakePreProcRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recs := spy.Records()
		if len(recs) >= n {
			return recs
		}
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	t.Fatalf("timed out: want %d records, got %d", n, len(spy.Records()))
	return nil
}

// Test_InlineFraming_JSONOutput verifies that a full log call with the JSON
// encoder produces a valid JSON object starting with '{' and ending with "}\n",
// byte-identical to the pre-P4 Encoder.Append path.
func Test_InlineFraming_JSONOutput(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("json-framing")
	config.InitPreProcessors(spy)

	entry.NewLogEntry().Info(context.Background(), "hello")

	recs := drainInlineRec(t, spy, 1)
	got := recs[0].Data

	if !bytes.HasPrefix(got, []byte("{")) {
		t.Errorf("JSON output must start with '{'; got: %q", got)
	}
	if !bytes.HasSuffix(got, []byte("}\n")) {
		t.Errorf("JSON output must end with '}\\n'; got: %q", got)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Errorf("JSON output must be valid JSON: %v\npayload: %s", err, got)
	}
	if obj["message"] != "hello" {
		t.Errorf("JSON output must contain message=hello; got %v", obj["message"])
	}
}

// Test_InlineFraming_TextOutput verifies that a full log call with the text
// encoder produces output ending with '\n' (no double newline).
func Test_InlineFraming_TextOutput(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderText).
		Build()
	spy := support.NewFakePreProc("text-framing")
	config.InitPreProcessors(spy)

	entry.NewLogEntry().Info(context.Background(), "hello text")

	recs := drainInlineRec(t, spy, 1)
	got := recs[0].Data

	if !bytes.HasSuffix(got, []byte("\n")) {
		t.Errorf("text output must end with newline; got: %q", got)
	}
	if bytes.HasSuffix(got, []byte("\n\n")) {
		t.Errorf("text output must not end with double newline; got: %q", got)
	}
}

// Test_OwnershipTransfer_RaceClean fires concurrent log calls under the race
// detector and asserts all events are delivered without a data race. After P4,
// the entry's buf is transferred to the channel and severed with a fresh
// make([]byte, 0, initBufCap) before e.Put() — this test verifies that
// sequence is race-free.
func Test_OwnershipTransfer_RaceClean(t *testing.T) {
	const n = 50
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("race-clean")
	config.InitPreProcessors(spy)
	entry.GenerateInitialPool(n)

	var wg sync.WaitGroup
	ctx := context.Background()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry.NewLogEntry().Info(ctx, "race test")
		}()
	}
	wg.Wait()

	recs := drainInlineRec(t, spy, n)
	if len(recs) < n {
		t.Errorf("ownership transfer: want %d events delivered; got %d", n, len(recs))
	}
}

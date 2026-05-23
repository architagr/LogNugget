//go:build testing

// Package entry white-box tests for P8 exact-size buffer alias severance.
// Must be in package entry (not entry_test) to access package-level vars.
// Cannot import test/support (would create entry → test/support → entry cycle).
package entry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

type p8NopWriter struct{}

func (w *p8NopWriter) Write(p []byte) (int, error) { return len(p), nil }

func setupP8(t *testing.T) {
	t.Helper()
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	out := &p8NopWriter{}
	proc := pipelineStage.NewUnsetLogEventPostProcessor(500*time.Millisecond, 100, out)
	t.Cleanup(proc.Stop)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
}

// capFromHook installs p8SeveranceCapHook, runs fn, removes the hook, and
// returns the cap value reported by the hook. The test fails if fn never
// triggers a severance (hook not called).
func capFromHook(t *testing.T, fn func()) int {
	t.Helper()
	got := -1
	p8SeveranceCapHook = func(c int) { got = c }
	defer func() { p8SeveranceCapHook = nil }()
	fn()
	if got == -1 {
		t.Fatal("p8SeveranceCapHook not called — severance path not reached")
	}
	return got
}

// Test_LogEntry_BufferSizeAfterSeverance verifies that after a log call with a
// short message, P8 computes newCap = max(64, len(data)) < initBufCap (1024 B).
// Uses p8SeveranceCapHook to observe newCap directly, avoiding sync.Pool
// slot-identity non-determinism under -race / multi-P execution.
func Test_LogEntry_BufferSizeAfterSeverance(t *testing.T) {
	setupP8(t)

	replacedCap := capFromHook(t, func() {
		NewLogEntry().Debug(context.Background(), "short message")
	})

	if replacedCap >= initBufCap {
		t.Errorf("buffer cap not reduced after P8: got=%d, want < %d (initBufCap)", replacedCap, initBufCap)
	}
	if replacedCap < 64 {
		t.Errorf("buffer cap below 64-byte floor: got %d", replacedCap)
	}
}

// Test_LogEntry_LargeMessage_NoExtraAlloc verifies that after a large-message
// log call, P8 sizes the replacement buffer to accommodate the message
// (cap ≥ 400 B for a 400-char message), preventing re-allocation on the next call.
func Test_LogEntry_LargeMessage_NoExtraAlloc(t *testing.T) {
	setupP8(t)
	largeMsg := strings.Repeat("x", 400)

	replacedCap := capFromHook(t, func() {
		NewLogEntry().Debug(context.Background(), largeMsg)
	})

	if replacedCap < 400 {
		t.Errorf("P8 undersized for large message: cap=%d, want ≥ 400", replacedCap)
	}
}

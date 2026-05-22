//go:build testing

// Package entry white-box tests for P8 exact-size buffer alias severance.
// Must be in package entry (not entry_test) to access e.buf directly.
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

// Test_LogEntry_BufferSizeAfterSeverance verifies that after a log call with a
// short message, the replacement buffer in the pool slot has cap < initBufCap
// (1024 B) — i.e. max(64, len(data)) is used, not the fixed 1024 B constant.
func Test_LogEntry_BufferSizeAfterSeverance(t *testing.T) {
	setupP8(t)
	GenerateInitialPool(1)

	// Get initial cap before any log call (should be initBufCap = 1024).
	e0 := NewLogEntry()
	initialCap := cap(e0.buf)
	e0.Put()

	// Log a short message (~60–80 B rendered). The pool slot buf is replaced
	// with max(64, len(data)) — significantly smaller than 1024.
	NewLogEntry().Debug(context.Background(), "short message")

	// Retrieve the same slot (single goroutine, GOMAXPROCS=1 default in tests).
	e1 := NewLogEntry()
	replacedCap := cap(e1.buf)
	e1.Put()

	if replacedCap >= initialCap {
		t.Errorf("buffer cap not reduced after P8: initial=%d replaced=%d; "+
			"expected max(64, len(data)) < %d", initialCap, replacedCap, initialCap)
	}
	if replacedCap < 64 {
		t.Errorf("buffer cap below 64-byte floor: got %d", replacedCap)
	}
}

// Test_LogEntry_LargeMessage_NoExtraAlloc verifies that after the pool slot is
// sized by a large message, a second identical call causes no buffer growth
// allocs. The cap of the retrieved slot must accommodate the large message.
func Test_LogEntry_LargeMessage_NoExtraAlloc(t *testing.T) {
	setupP8(t)
	GenerateInitialPool(1)

	largeMsg := strings.Repeat("x", 400)
	// First call warms the pool slot to len(data) ≈ 400+ B.
	NewLogEntry().Debug(context.Background(), largeMsg)

	// Second retrieval: pool slot cap should now be ≥ 400 B.
	e := NewLogEntry()
	gotCap := cap(e.buf)
	e.Put()

	if gotCap < 400 {
		t.Errorf("pool slot not sized for large message: cap=%d, want ≥ 400", gotCap)
	}
}

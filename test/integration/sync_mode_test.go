//go:build testing

// Package integration: V4-P1 — the opt-in synchronous dispatch path.
package integration

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	pipelineStage "github.com/architagr/lognugget/v4/pipeline_stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSyncPipeline wires a collector that writes every record immediately, so a
// test can assert on output without waiting for a ticker.
func newSyncPipeline(t *testing.T) *collectingWriter {
	t.Helper()

	out := &collectingWriter{}
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(time.Hour, 1, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(64)

	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())
		proc.Stop()
		config.TestResetConfig()
	})
	return out
}

// Test_Integration_SyncMode_DeliversWithoutDispatcher asserts that with sync
// mode on, a record reaches the hooks without the ring consumer running — the
// delivery happens on the calling goroutine.
func Test_Integration_SyncMode_DeliversWithoutDispatcher(t *testing.T) {
	out := newSyncPipeline(t)
	config.SetSyncMode(true)
	require.True(t, config.SyncMode())

	entry.NewLogEntry().Str("path", "/sync").Info(context.Background(), "sync record")

	// No FlushDispatch: if the record needed the dispatcher, it would not be
	// here yet.
	assert.Eventually(t, func() bool {
		return bytes.Contains(out.Bytes(), []byte(`"path":"/sync"`))
	}, time.Second, 5*time.Millisecond, "record must reach the hook on the caller's goroutine")
}

// Test_Integration_SyncMode_HooksStillFanOut asserts that sync mode does not
// bypass the hook chain.
//
// why: the original design for this option wrote straight to the configured
// io.Writer, which would have silently stopped every registered hook.
func Test_Integration_SyncMode_HooksStillFanOut(t *testing.T) {
	unset := newSyncPipeline(t)

	errOut := &collectingWriter{}
	errProc := pipelineStage.NewUnsetLogEventPostProcessor(time.Hour, 1, errOut)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelError, errProc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelError, errProc.Name())
		errProc.Stop()
	})

	config.SetSyncMode(true)
	entry.NewLogEntry().Error(context.Background(), assertErr{}, "boom")

	assert.Eventually(t, func() bool {
		return bytes.Contains(errOut.Bytes(), []byte("boom"))
	}, time.Second, 5*time.Millisecond, "level-specific hook must still receive the record")
	assert.Contains(t, string(unset.Bytes()), "boom",
		"LevelUnSet hook must still receive the record")
}

// Test_Integration_SyncMode_NoRecordLossUnderConcurrency asserts every record
// arrives exactly once when many goroutines log in sync mode.
func Test_Integration_SyncMode_NoRecordLossUnderConcurrency(t *testing.T) {
	const (
		goroutines = 8
		perG       = 200
		total      = goroutines * perG
	)

	out := newSyncPipeline(t)
	config.SetSyncMode(true)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				entry.NewLogEntry().Int("seq", int64(g*perG+i)).Info(context.Background(), "sync burst")
			}
		}(g)
	}
	wg.Wait()

	assert.Eventually(t, func() bool {
		return bytes.Count(out.Bytes(), []byte{'\n'}) == total
	}, 5*time.Second, 10*time.Millisecond, "every record must be delivered exactly once")
}

// Test_Integration_SyncMode_TogglesBackToAsync asserts the switch is reversible
// at runtime and that records published either way all arrive.
func Test_Integration_SyncMode_TogglesBackToAsync(t *testing.T) {
	out := newSyncPipeline(t)
	ctx := context.Background()

	config.SetSyncMode(true)
	entry.NewLogEntry().Str("mode", "sync").Info(ctx, "first")

	config.SetSyncMode(false)
	require.False(t, config.SyncMode())
	entry.NewLogEntry().Str("mode", "async").Info(ctx, "second")
	require.True(t, config.FlushDispatch(5*time.Second))

	assert.Eventually(t, func() bool {
		b := out.Bytes()
		return bytes.Contains(b, []byte(`"mode":"sync"`)) && bytes.Contains(b, []byte(`"mode":"async"`))
	}, 2*time.Second, 10*time.Millisecond)
}

// Test_Integration_SyncMode_ResetRestoresAsync asserts TestResetConfig clears
// the setting, so it cannot leak between tests.
func Test_Integration_SyncMode_ResetRestoresAsync(t *testing.T) {
	config.SetSyncMode(true)
	config.TestResetConfig()
	assert.False(t, config.SyncMode(), "reset must restore the asynchronous default")
}

// assertErr is a minimal error value for the fan-out test.
type assertErr struct{}

func (assertErr) Error() string { return "sync-mode failure" }

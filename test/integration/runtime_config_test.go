//go:build testing

// Package integration: V4 — the documented configuration setters must change
// the behaviour of the built-in collector at runtime.
package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	pipelineStage "github.com/architagr/lognugget/v4/pipeline_stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSinkPipeline wires a collector registered as the default sink, the way
// package lognugget does at init, and returns the writer it feeds.
func newSinkPipeline(t *testing.T, rate time.Duration, bucket int) *collectingWriter {
	t.Helper()

	out := &collectingWriter{}
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(rate, bucket, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	config.RegisterDefaultSink(proc)
	entry.GenerateInitialPool(64)

	t.Cleanup(func() {
		config.RegisterDefaultSink(nil)
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())
		proc.Stop()
		config.TestResetConfig()
	})
	return out
}

// Test_Integration_SetOutput_RedirectsCollector asserts that config.SetOutput
// changes where the built-in collector writes.
//
// why: SetOutput used to store the writer on a struct field that nothing read,
// so the documented "output writer for the default collector" knob silently
// did nothing.
func Test_Integration_SetOutput_RedirectsCollector(t *testing.T) {
	original := newSinkPipeline(t, time.Hour, 1)

	redirected := &collectingWriter{}
	config.SetOutput(redirected)

	entry.NewLogEntry().Str("where", "redirected").Info(context.Background(), "after SetOutput")
	require.True(t, config.FlushDispatch(5*time.Second))

	assert.Eventually(t, func() bool {
		return bytes.Contains(redirected.Bytes(), []byte(`"where":"redirected"`))
	}, 2*time.Second, 10*time.Millisecond, "record must reach the writer passed to SetOutput")
	assert.NotContains(t, string(original.Bytes()), "after SetOutput",
		"nothing must reach the previous writer once SetOutput has been called")
}

// Test_Integration_SetLogBufferMaxSize_ChangesFlushThreshold asserts that the
// record count that triggers a flush follows config.SetLogBufferMaxSize.
func Test_Integration_SetLogBufferMaxSize_ChangesFlushThreshold(t *testing.T) {
	// A huge starting bucket and an hour-long rate mean nothing flushes until
	// the threshold is lowered.
	out := newSinkPipeline(t, time.Hour, 1_000_000)

	ctx := context.Background()
	config.SetLogBufferMaxSize(2)

	entry.NewLogEntry().Int("n", 1).Info(ctx, "first")
	entry.NewLogEntry().Int("n", 2).Info(ctx, "second")
	require.True(t, config.FlushDispatch(5*time.Second))

	assert.Eventually(t, func() bool {
		return bytes.Count(out.Bytes(), []byte{'\n'}) == 2
	}, 2*time.Second, 10*time.Millisecond, "bucket of 2 must flush on the second record")
}

// Test_Integration_SetRate_ChangesFlushInterval asserts that the ticker-driven
// flush interval follows config.SetRate.
func Test_Integration_SetRate_ChangesFlushInterval(t *testing.T) {
	// Start with an hour-long rate and a bucket far larger than the test load,
	// so only a rate change can produce output.
	out := newSinkPipeline(t, time.Hour, 1_000_000)

	config.SetRate(20 * time.Millisecond)

	entry.NewLogEntry().Str("k", "v").Info(context.Background(), "tick me out")
	require.True(t, config.FlushDispatch(5*time.Second))

	assert.Eventually(t, func() bool {
		return bytes.Contains(out.Bytes(), []byte("tick me out"))
	}, 2*time.Second, 10*time.Millisecond, "record must be flushed by the new ticker interval")
}

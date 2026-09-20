//go:build testing

// Package integration: V4-P4 — Shutdown must not drop events that are still
// sitting in the dispatch ring.
package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_FlushDispatch_NoLossOnShutdown logs a burst and then shuts
// the pipeline down the way an application's main() would.
//
// why: PublishLog returns once the event is in the MPSC ring. Before
// FlushDispatch existed, stopping the post-processor immediately afterwards
// threw away every event the consumer goroutine had not yet popped — the
// common "log and exit" shape lost its tail.
func Test_Integration_FlushDispatch_NoLossOnShutdown(t *testing.T) {
	const total = 2000

	out := &collectingWriter{}

	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)

	// A long rate means the only thing that flushes the hook is Stop; a bucket
	// larger than the burst means no size-triggered flush happens either.
	proc := pipelineStage.NewUnsetLogEventPostProcessor(time.Hour, total*2, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(64)

	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())
		config.TestResetConfig()
	})

	ctx := context.Background()
	for i := 0; i < total; i++ {
		entry.NewLogEntry().Int("seq", int64(i)).Info(ctx, "flush probe")
	}

	require.True(t, config.FlushDispatch(5*time.Second), "dispatcher did not drain within timeout")
	proc.Stop()

	got := bytes.Count(out.Bytes(), []byte{'\n'})
	assert.Equal(t, total, got, "every published event must reach the writer")
}

// Test_Integration_FlushDispatch_IsIdempotent asserts that flushing an already
// quiescent dispatcher returns immediately and reports success.
func Test_Integration_FlushDispatch_IsIdempotent(t *testing.T) {
	config.TestResetConfig()
	t.Cleanup(config.TestResetConfig)

	assert.True(t, config.FlushDispatch(time.Second))
	assert.True(t, config.FlushDispatch(time.Second))
}

// Test_Integration_FlushDispatch_ZeroTimeoutUsesDefault asserts the documented
// fallback: a non-positive timeout means DefaultFlushTimeout, not "give up".
func Test_Integration_FlushDispatch_ZeroTimeoutUsesDefault(t *testing.T) {
	config.TestResetConfig()
	t.Cleanup(config.TestResetConfig)

	assert.True(t, config.FlushDispatch(0))
}

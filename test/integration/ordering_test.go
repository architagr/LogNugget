//go:build testing

// Package integration: records must reach the writer in publication order.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	pipelineStage "github.com/architagr/lognugget/v4/pipeline_stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_SingleProducerOrderIsPreserved logs an ascending sequence
// from one goroutine and asserts the writer sees it ascending.
//
// why: each flush used to start its own goroutine, so two batches could write
// concurrently and interleave — a later record could appear before an earlier
// one in the output file. A bucket of 1 maximises the number of separate
// flushes, which is the condition that exposed the reordering.
func Test_Integration_SingleProducerOrderIsPreserved(t *testing.T) {
	const total = 500

	out := &collectingWriter{}

	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(time.Hour, 1, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(64)

	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())
		config.TestResetConfig()
	})

	ctx := context.Background()
	for i := 0; i < total; i++ {
		entry.NewLogEntry().Int("seq", int64(i)).Info(ctx, "ordered")
	}

	require.True(t, config.FlushDispatch(5*time.Second))
	proc.Stop()

	lines := bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte{'\n'})
	require.Len(t, lines, total)

	for i, line := range lines {
		var rec struct {
			Seq *int `json:"seq"`
		}
		require.NoError(t, json.Unmarshal(line, &rec), "line %d: %q", i, line)
		require.NotNil(t, rec.Seq)
		assert.Equalf(t, i, *rec.Seq, "record %d out of order: got seq %d", i, *rec.Seq)
		if t.Failed() {
			break
		}
	}
}

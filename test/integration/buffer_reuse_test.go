//go:build testing

// Package integration: V4-P1 — a pooled dispatch buffer must not be recycled
// while a hook still holds a reference to it.
package integration

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
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectingWriter accumulates every byte slice written to it.
type collectingWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *collectingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *collectingWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

// Test_Integration_NoBufferReuseCorruption asserts that every log line reaching
// the writer is exactly the line that was logged.
//
// why: dispatchEvent returns the dispatch buffer to dispatchBufPool as soon as
// every pre-processor's PreProcess call returns, but a buffering hook keeps the
// slice until its bucket flushes. If the hook stores the slice rather than its
// contents, a producer picks the same backing array out of the pool and
// overwrites the pending line — the writer then sees a duplicate or a spliced
// line instead of the original. Each message carries a unique sequence number
// so any overwrite shows up as a missing or repeated seq.
func Test_Integration_NoBufferReuseCorruption(t *testing.T) {
	const (
		goroutines = 8
		perG       = 500
		total      = goroutines * perG
	)

	out := &collectingWriter{}

	config.TestResetConfig()
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)

	// A large bucket keeps lines pending for a long window, which is exactly
	// when a recycled buffer can be overwritten underneath the hook.
	proc := pipelineStage.NewUnsetLogEventPostProcessor(50*time.Millisecond, 256, out)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(64)

	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())
		config.TestResetConfig()
	})

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				entry.NewLogEntry().
					Int("seq", int64(g*perG+i)).
					Info(context.Background(), "buffer reuse probe")
			}
		}(g)
	}
	wg.Wait()

	// Drain the ring before stopping the hook: Stop only flushes what the
	// dispatcher has already delivered.
	require.True(t, config.FlushDispatch(5*time.Second), "dispatcher did not drain")
	proc.Stop()

	seen := make(map[int]int, total)
	lines := bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte{'\n'})
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Seq *int `json:"seq"`
		}
		require.NoError(t, json.Unmarshal(line, &rec), "line is not valid JSON: %q", line)
		require.NotNil(t, rec.Seq, "line has no seq field: %q", line)
		seen[*rec.Seq]++
	}

	assert.Len(t, seen, total, "every logged sequence number must appear exactly once")
	for seq, n := range seen {
		assert.Equalf(t, 1, n, "seq %d written %d times — pooled buffer was reused before flush", seq, n)
	}
}

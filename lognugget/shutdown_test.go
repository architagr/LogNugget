// Package lognugget is the top-level façade for the LogNugget structured
// logging library. Tests in this file use white-box access (same package) so
// they can reset package-level state between runs.
package lognugget

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/stretchr/testify/assert"
)

// resetShutdown resets the package-level shutdown state so each test starts
// from a clean slate. Must be called before every test that exercises Shutdown.
func resetShutdown(proc postProcessor) {
	defaultPostProc = proc
	shutdownOnce = sync.Once{}
}

// countingWriter is a thread-safe io.Writer that counts every Write call.
type countingWriter struct {
	n atomic.Int64
}

// Write satisfies io.Writer; increments the call counter each invocation.
func (w *countingWriter) Write(p []byte) (int, error) {
	w.n.Add(1)
	return len(p), nil
}

// count returns the total number of Write calls recorded.
func (w *countingWriter) count() int {
	return int(w.n.Load())
}

// Test_Shutdown_Idempotent verifies that calling Shutdown twice does not panic
// and the second call is a no-op (idempotency via sync.Once).
func Test_Shutdown_Idempotent(t *testing.T) {
	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, 10, io.Discard)
	resetShutdown(proc)

	Shutdown() // first call — drains and closes

	// second call must not panic
	assert.NotPanics(t, func() { Shutdown() }, "second Shutdown must be a no-op and must not panic")
}

// Test_Shutdown_DrainsDefaultPostProcessor enqueues N messages into the default
// post-processor, calls Shutdown, then asserts that the writer saw all events
// (each message produces 2 Write calls: data + newline separator).
func Test_Shutdown_DrainsDefaultPostProcessor(t *testing.T) {
	const msgCount = 5
	out := &countingWriter{}
	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, 100, out)
	resetShutdown(proc)

	for i := 0; i < msgCount; i++ {
		proc.PublishLogMessage([]byte("event"))
	}

	Shutdown()

	// each message produces data write + "\n" write = 2 calls each
	assert.Equal(t, msgCount*2, out.count(),
		"Shutdown must drain all enqueued messages through the writer")
}

// Test_Shutdown_BlocksUntilDrain asserts that Shutdown returns only AFTER all
// buffered log messages have been fully written — i.e., the call is synchronous
// with respect to the underlying flush.
func Test_Shutdown_BlocksUntilDrain(t *testing.T) {
	const msgCount = 20
	out := &countingWriter{}
	proc := pipelineStage.NewUnsetLogEventPostProcessor(10*time.Minute, 100, out)
	resetShutdown(proc)

	for i := 0; i < msgCount; i++ {
		proc.PublishLogMessage([]byte("payload"))
	}

	Shutdown()

	// If Shutdown is truly synchronous, all writes must be done by the time
	// Shutdown returns — no polling or waiting required here.
	assert.Equal(t, msgCount*2, out.count(),
		"Shutdown must block until all writes complete before returning")
}

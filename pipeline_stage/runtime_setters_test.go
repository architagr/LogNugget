package pipelineStage

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/enum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a thread-safe bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// Test_PostProcessor_SetOutput_RedirectsSubsequentWrites asserts that records
// buffered after SetOutput land in the new writer.
func Test_PostProcessor_SetOutput_RedirectsSubsequentWrites(t *testing.T) {
	first, second := &syncBuffer{}, &syncBuffer{}
	proc := NewUnsetLogEventPostProcessor(time.Hour, 1, first)
	defer proc.Stop()

	proc.SetOutput(second)
	proc.PublishLogMessage([]byte("after-redirect\n"))

	assert.Eventually(t, func() bool {
		return strings.Contains(second.String(), "after-redirect")
	}, time.Second, 5*time.Millisecond, "record must go to the writer passed to SetOutput")
	assert.NotContains(t, first.String(), "after-redirect")
}

// Test_PostProcessor_SetOutput_NilIsIgnored asserts a nil writer cannot
// silently disable the collector.
func Test_PostProcessor_SetOutput_NilIsIgnored(t *testing.T) {
	out := &syncBuffer{}
	proc := NewUnsetLogEventPostProcessor(time.Hour, 1, out)
	defer proc.Stop()

	proc.SetOutput(nil)
	proc.PublishLogMessage([]byte("still-delivered\n"))

	assert.Eventually(t, func() bool {
		return strings.Contains(out.String(), "still-delivered")
	}, time.Second, 5*time.Millisecond, "nil output must be ignored, not installed")
}

// Test_PostProcessor_SetRate_ShortensFlushInterval asserts the ticker follows
// the new rate.
func Test_PostProcessor_SetRate_ShortensFlushInterval(t *testing.T) {
	out := &syncBuffer{}
	// An hour-long rate and a bucket far above the load: only a rate change
	// can produce output.
	proc := NewUnsetLogEventPostProcessor(time.Hour, 1_000_000, out)
	defer proc.Stop()

	proc.PublishLogMessage([]byte("tick-me\n"))
	proc.SetRate(10 * time.Millisecond)

	assert.Eventually(t, func() bool {
		return strings.Contains(out.String(), "tick-me")
	}, 2*time.Second, 5*time.Millisecond, "new ticker interval must flush the pending record")
}

// Test_PostProcessor_SetRate_NonPositiveIgnored asserts a zero or negative
// rate cannot stop the ticker.
func Test_PostProcessor_SetRate_NonPositiveIgnored(t *testing.T) {
	out := &syncBuffer{}
	proc := NewUnsetLogEventPostProcessor(10*time.Millisecond, 1_000_000, out)
	defer proc.Stop()

	proc.SetRate(0)
	proc.SetRate(-time.Second)
	proc.PublishLogMessage([]byte("ticker-alive\n"))

	assert.Eventually(t, func() bool {
		return strings.Contains(out.String(), "ticker-alive")
	}, 2*time.Second, 5*time.Millisecond, "the original ticker must survive an invalid rate")
}

// Test_PostProcessor_SetMaxBucketSize_ChangesFlushThreshold asserts the
// record count that triggers a flush follows the setter.
func Test_PostProcessor_SetMaxBucketSize_ChangesFlushThreshold(t *testing.T) {
	out := &syncBuffer{}
	proc := NewUnsetLogEventPostProcessor(time.Hour, 1_000_000, out)
	defer proc.Stop()

	proc.SetMaxBucketSize(2)
	proc.PublishLogMessage([]byte("one\n"))
	proc.PublishLogMessage([]byte("two\n"))

	assert.Eventually(t, func() bool {
		return strings.Count(out.String(), "\n") == 2
	}, time.Second, 5*time.Millisecond, "a bucket of 2 must flush on the second record")
}

// Test_PostProcessor_SetMaxBucketSize_NonPositiveIgnored asserts an invalid
// size cannot turn every record into its own flush, or stall flushing.
func Test_PostProcessor_SetMaxBucketSize_NonPositiveIgnored(t *testing.T) {
	out := &syncBuffer{}
	proc := NewUnsetLogEventPostProcessor(time.Hour, 1, out)
	defer proc.Stop()

	proc.SetMaxBucketSize(0)
	proc.PublishLogMessage([]byte("unchanged\n"))

	assert.Eventually(t, func() bool {
		return strings.Contains(out.String(), "unchanged")
	}, time.Second, 5*time.Millisecond, "the original bucket size must survive an invalid value")
}

// Test_PutArena_DropsOversizedBuffers asserts that an arena grown past the
// retention cap is dropped rather than recycled.
//
// why: a single very large batch would otherwise pin that memory in the pool
// for the life of the process.
func Test_PutArena_DropsOversizedBuffers(t *testing.T) {
	oversized := make([]byte, 0, maxArenaRetainCap+1)
	putArena(oversized)

	got := getArena()
	require.LessOrEqual(t, cap(got), maxArenaRetainCap,
		"an oversized arena must not be returned to the pool")
}

// Test_EventPreProcessorObserver_Name pins the observer's registry identifier.
func Test_EventPreProcessorObserver_Name(t *testing.T) {
	assert.Equal(t, "EventPreProcessorObserver", EventPreProcessorObj.Name())
}

// Test_DeRegisterHook_UnknownLevelIsNoOp covers the early return when no hook
// has ever been registered at the given level.
func Test_DeRegisterHook_UnknownLevelIsNoOp(t *testing.T) {
	observer := newEventPreProcessingObserver()
	assert.NotPanics(t, func() {
		observer.DeRegisterHook(enum.LevelError, "never-registered")
	})
}

//go:build testing

// Unit coverage for the V4 additions to package config. These are exercised
// end to end from test/integration, but Go only credits coverage to the
// package under test, so the behaviour is pinned here too.
package config

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/v4/enum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSink implements DefaultSink and records what it was asked to do.
type recordingSink struct {
	mu     sync.Mutex
	output io.Writer
	rate   time.Duration
	bucket int
}

func (s *recordingSink) SetOutput(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output = w
}

func (s *recordingSink) SetRate(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rate = d
}

func (s *recordingSink) SetMaxBucketSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bucket = n
}

func (s *recordingSink) snapshot() (io.Writer, time.Duration, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output, s.rate, s.bucket
}

// Test_RegisterDefaultSink_ReceivesSetterCalls asserts the three collector
// setters are forwarded to the registered sink.
func Test_RegisterDefaultSink_ReceivesSetterCalls(t *testing.T) {
	TestResetConfig()
	t.Cleanup(func() {
		RegisterDefaultSink(nil)
		TestResetConfig()
	})

	sink := &recordingSink{}
	RegisterDefaultSink(sink)

	var buf bytes.Buffer
	SetOutput(&buf)
	SetRate(1234 * time.Millisecond)
	SetLogBufferMaxSize(77)

	gotOut, gotRate, gotBucket := sink.snapshot()
	assert.Same(t, io.Writer(&buf), gotOut)
	assert.Equal(t, 1234*time.Millisecond, gotRate)
	assert.Equal(t, 77, gotBucket)
}

// Test_RegisterDefaultSink_NilDetaches asserts a nil registration stops the
// forwarding without panicking on the next setter call.
func Test_RegisterDefaultSink_NilDetaches(t *testing.T) {
	TestResetConfig()
	t.Cleanup(func() {
		RegisterDefaultSink(nil)
		TestResetConfig()
	})

	sink := &recordingSink{}
	RegisterDefaultSink(sink)
	RegisterDefaultSink(nil)

	require.Nil(t, loadDefaultSink())
	assert.NotPanics(t, func() { SetRate(time.Second) })

	_, gotRate, _ := sink.snapshot()
	assert.Zero(t, gotRate, "a detached sink must not be called")
}

// Test_SafeFieldKey_PrefixesReservedKeys covers both branches of the
// reserved-key check the chain methods rely on.
func Test_SafeFieldKey_PrefixesReservedKeys(t *testing.T) {
	TestResetConfig()
	t.Cleanup(TestResetConfig)

	assert.Equal(t, DefaultPrefix+"time", SafeFieldKey("time"))
	assert.Equal(t, DefaultPrefix+"level", SafeFieldKey("level"))
	assert.Equal(t, "request_id", SafeFieldKey("request_id"),
		"a non-reserved key must be returned unchanged")
}

// Test_SafeFieldKey_FollowsRenamedDefaults asserts the check tracks
// SetDefaultFields rather than a hardcoded list.
func Test_SafeFieldKey_FollowsRenamedDefaults(t *testing.T) {
	TestResetConfig()
	t.Cleanup(TestResetConfig)

	SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyTime: "ts"})

	assert.Equal(t, DefaultPrefix+"ts", SafeFieldKey("ts"),
		"the renamed key becomes the reserved one")
	assert.Equal(t, "time", SafeFieldKey("time"),
		"the old name is no longer reserved once renamed")
}

// Test_SyncMode_TogglesAndResets covers the accessor pair and the reset path.
func Test_SyncMode_TogglesAndResets(t *testing.T) {
	TestResetConfig()
	t.Cleanup(TestResetConfig)

	assert.False(t, SyncMode(), "async is the default")

	SetSyncMode(true)
	assert.True(t, SyncMode())

	SetSyncMode(false)
	assert.False(t, SyncMode())

	SetSyncMode(true)
	TestResetConfig()
	assert.False(t, SyncMode(), "reset must restore the asynchronous default")
}

// countingProc counts dispatches so the sync path can be observed directly.
type countingProc struct {
	mu sync.Mutex
	n  int
}

func (c *countingProc) Name() string { return "sync-counter" }

func (c *countingProc) PreProcess(_ enum.LogLevel, _ []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *countingProc) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// Test_PublishLog_SyncModeDispatchesInline asserts that with sync mode on,
// PublishLog has already delivered the event by the time it returns — no
// dispatcher goroutine involved.
func Test_PublishLog_SyncModeDispatchesInline(t *testing.T) {
	TestResetConfig()
	t.Cleanup(TestResetConfig)

	proc := &countingProc{}
	InitPreProcessors(proc)
	SetSyncMode(true)

	PublishLog(enum.LevelInfo, GetDispatchBuf())

	assert.Equal(t, 1, proc.count(),
		"sync mode must deliver on the calling goroutine, before PublishLog returns")
}

// Test_FlushDispatch_NonPositiveTimeoutUsesDefault covers the timeout
// normalisation branch.
func Test_FlushDispatch_NonPositiveTimeoutUsesDefault(t *testing.T) {
	TestResetConfig()
	t.Cleanup(TestResetConfig)

	assert.True(t, FlushDispatch(0))
	assert.True(t, FlushDispatch(-time.Second))
}

// Test_FlushDispatch_QuiescentRingReturnsImmediately covers the fast path
// where nothing is outstanding.
func Test_FlushDispatch_QuiescentRingReturnsImmediately(t *testing.T) {
	TestResetConfig()
	t.Cleanup(TestResetConfig)

	start := time.Now()
	require.True(t, FlushDispatch(5*time.Second))
	assert.Less(t, time.Since(start), time.Second,
		"an idle dispatcher must not wait for the timeout")
}

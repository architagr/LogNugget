//go:build testing

package support_test

import (
	"sync"
	"testing"

	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_FakeWriter_BytesAndCount_Sequential verifies the basic Fake
// contract for acceptance #4: Write appends; Bytes returns a copy of
// every byte ever written; Count tracks Write calls.
func Test_FakeWriter_BytesAndCount_Sequential(t *testing.T) {
	t.Parallel()

	w := support.NewFakeWriter()
	n, err := w.Write([]byte("alpha"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	_, err = w.Write([]byte("beta"))
	require.NoError(t, err)

	assert.Equal(t, 2, w.Count(), "Count must equal number of Write calls")
	assert.Equal(t, []byte("alphabeta"), w.Bytes(), "Bytes must return concatenated payload")
}

// Test_FakeWriter_BytesReturnsCopy guards against callers mutating the
// internal buffer through the returned slice (T-13-style hygiene).
func Test_FakeWriter_BytesReturnsCopy(t *testing.T) {
	t.Parallel()

	w := support.NewFakeWriter()
	_, _ = w.Write([]byte("xy"))
	got := w.Bytes()
	got[0] = 'Z'

	assert.Equal(t, []byte("xy"), w.Bytes(), "mutating returned slice must not affect FakeWriter state")
}

// Test_FakeWriter_ConcurrentSafe exercises the mu-guard requirement of
// acceptance #4: the writer must survive parallel writers under -race.
func Test_FakeWriter_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	const writers = 16
	const perWriter = 64

	w := support.NewFakeWriter()
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				_, _ = w.Write([]byte("x"))
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, writers*perWriter, w.Count())
	assert.Len(t, w.Bytes(), writers*perWriter)
}

// Test_SpyHook_RecordsOrderedAndExposesName covers acceptance #5:
// SpyHook is chan-backed, records (level, payload) entries, and
// satisfies the public hook contract.
func Test_SpyHook_RecordsOrderedAndExposesName(t *testing.T) {
	t.Parallel()

	hook := support.NewSpyHook("spy-1", 4)
	assert.Equal(t, "spy-1", hook.Name())

	hook.PublishLogMessage([]byte("first"))
	hook.PublishLogMessage([]byte("second"))

	got1, ok1 := hook.Next()
	require.True(t, ok1, "Next must return the first record")
	assert.Equal(t, []byte("first"), got1.Data)

	got2, ok2 := hook.Next()
	require.True(t, ok2)
	assert.Equal(t, []byte("second"), got2.Data)
}

// Test_SpyHook_DropsBeyondCapacity asserts the chan-backed buffer
// drops new records once full instead of blocking the producer; this
// keeps tests deterministic when wired into hook fan-out paths.
func Test_SpyHook_DropsBeyondCapacity(t *testing.T) {
	t.Parallel()

	hook := support.NewSpyHook("cap", 1)
	hook.PublishLogMessage([]byte("kept"))
	hook.PublishLogMessage([]byte("dropped"))

	got, ok := hook.Next()
	require.True(t, ok)
	assert.Equal(t, []byte("kept"), got.Data)

	_, ok = hook.NextNonBlocking()
	assert.False(t, ok, "second record must have been dropped, not buffered")
}

// Test_FakePreProc_RecordsAllInvocations covers acceptance #6.
func Test_FakePreProc_RecordsAllInvocations(t *testing.T) {
	t.Parallel()

	pp := support.NewFakePreProc("pp")
	assert.Equal(t, "pp", pp.Name())

	pp.PreProcess(enum.LevelInfo, []byte("a"))
	pp.PreProcess(enum.LevelError, []byte("b"))

	rec := pp.Records()
	require.Len(t, rec, 2)
	assert.Equal(t, enum.LevelInfo, rec[0].Level)
	assert.Equal(t, []byte("a"), rec[0].Data)
	assert.Equal(t, enum.LevelError, rec[1].Level)
	assert.Equal(t, []byte("b"), rec[1].Data)
}

//go:build testing

package support_test

import (
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/test/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_SlowHook_BlocksUntilRelease covers acceptance #1.
func Test_SlowHook_BlocksUntilRelease(t *testing.T) {
	t.Parallel()
	hook := support.NewSlowHook("slow-1")
	assert.Equal(t, "slow-1", hook.Name())

	done := make(chan struct{})
	go func() { hook.PublishLogMessage([]byte("payload")); close(done) }()

	select {
	case <-done:
		t.Fatal("PublishLogMessage returned before Release()")
	case <-time.After(20 * time.Millisecond):
	}

	hook.Release()
	assert.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond)

	got := hook.Records()
	require.Len(t, got, 1)
	assert.Equal(t, []byte("payload"), got[0])
}

// Test_FakePostProcessor_GotReturnsCopies covers acceptance #2.
func Test_FakePostProcessor_GotReturnsCopies(t *testing.T) {
	t.Parallel()
	pp := support.NewFakePostProcessor("pp")
	assert.Equal(t, "pp", pp.Name())

	pp.PublishLogMessage([]byte("a"))
	pp.PublishLogMessage([]byte("b"))

	got := pp.Got()
	require.Len(t, got, 2)
	assert.Equal(t, []byte("a"), got[0])
	assert.Equal(t, []byte("b"), got[1])

	got[0][0] = 'Z'
	assert.Equal(t, []byte("a"), pp.Got()[0], "Got must return defensive copies")
}

// Test_StubContextParser_ReturnsFixedAndCounts covers acceptance #3.
func Test_StubContextParser_ReturnsFixedAndCounts(t *testing.T) {
	t.Parallel()
	want := map[string]any{"trace": "abc", "user": 7}
	stub := support.NewStubContextParser(want)

	parser := stub.Parser()
	assert.Equal(t, want, parser(context.Background()))
	assert.Equal(t, want, parser(context.TODO()))
	assert.Equal(t, 2, stub.Calls())
}

// Test_StubStaticParser_EvaluatesOnce covers acceptance #4 / F15.
func Test_StubStaticParser_EvaluatesOnce(t *testing.T) {
	t.Parallel()
	want := map[string]any{"env": "test"}
	stub := support.NewStubStaticParser(want)

	parser := stub.Parser()
	for i := 0; i < 5; i++ {
		assert.Equal(t, want, parser())
	}
	assert.Equal(t, 1, stub.Evaluations(), "F15: static parser must be evaluated once")
}

// Test_StubEncoder_WritePassthrough covers acceptance #5.
func Test_StubEncoder_WritePassthrough(t *testing.T) {
	t.Parallel()
	enc := support.NewStubEncoder()
	out, err := enc.Write("hello")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), out)

	_, _ = enc.Write("world")
	assert.Equal(t, []string{"hello", "world"}, enc.Calls())
}

// Test_FakeClock_NowDeterministic covers acceptance #6.
func Test_FakeClock_NowDeterministic(t *testing.T) {
	t.Parallel()
	clk := support.NewFakeClock()
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.Equal(t, want, clk.Now())
	assert.Equal(t, want, clk.Now(), "Now must remain pinned across calls")

	clk.Advance(time.Second)
	assert.Equal(t, want.Add(time.Second), clk.Now(), "Advance must shift the pinned instant")
}

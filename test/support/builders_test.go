//go:build testing

// Package support_test exercises the test/support helpers.
package support_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_NewConfigBuilder_ChainsAllSettersAndCleansUp verifies that the
// ConfigBuilder applies every setter, that Build() returns a cleanup
// function, and that the cleanup is registered with t.Cleanup so the
// singleton resets between tests (acceptance #1, #7, #8).
func Test_NewConfigBuilder_ChainsAllSettersAndCleansUp(t *testing.T) {
	t.Parallel()

	var buf testWriter
	tb := newRecordingTB(t)
	cleanup := support.NewConfigBuilder(tb).
		MinLevel(enum.LevelError).
		Encoder(enum.EncoderText).
		Output(&buf).
		AddSource(false).
		Build()

	require.NotNil(t, cleanup, "Build must return a non-nil cleanup func")

	cfg := config.GetConfig()
	assert.Equal(t, enum.LevelError, cfg.MinLevel())
	assert.Equal(t, enum.EncoderText, cfg.EncoderType())
	assert.False(t, cfg.AddSource())
	assert.Same(t, io.Writer(&buf), cfg.Output())

	require.Len(t, tb.cleanups, 1, "Build must register exactly one t.Cleanup")

	// Run the cleanup; it must restore singleton defaults so the next test sees a fresh state.
	cleanup()
	cfg = config.GetConfig()
	assert.Equal(t, config.DafaultLevel, cfg.MinLevel(), "cleanup must restore default min-level")
	assert.Equal(t, config.DafaultEncoderType, cfg.EncoderType(), "cleanup must restore default encoder")
	assert.Equal(t, config.DefaultAddSource, cfg.AddSource(), "cleanup must restore default addSource")
}

// Test_ConfigBuilder_TBCleanupRunsAutomatically asserts the cleanup
// registered with tb.Cleanup actually fires when the test scope ends,
// guarding against the T-13 singleton-leak class of bug.
func Test_ConfigBuilder_TBCleanupRunsAutomatically(t *testing.T) {
	t.Parallel()

	tb := newRecordingTB(t)
	support.NewConfigBuilder(tb).MinLevel(enum.LevelWarn).Build()
	require.Len(t, tb.cleanups, 1)

	tb.runCleanups()

	assert.Equal(t, config.DafaultLevel, config.GetConfig().MinLevel())
}

// Test_EntryBuilder_AppliesAllOptions covers acceptance #2: the chainable
// EntryBuilder records ctx, err, and N synthetic fields, and Build()
// returns a usable *entry.LogEntry.
func Test_EntryBuilder_AppliesAllOptions(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxKey("k"), "v")
	bootErr := errors.New("boom")

	b := support.NewEntryBuilder().
		WithFields(5).
		WithCtx(ctx).
		WithErr(bootErr)

	assert.Len(t, b.Fields(), 5, "WithFields(n) must produce n attrs")
	assert.Same(t, ctx, b.Ctx(), "WithCtx must store the context")
	assert.Same(t, bootErr, b.Err(), "WithErr must store the error")

	got := b.Build()
	require.NotNil(t, got, "Build must return a non-nil *entry.LogEntry")
}

// Test_EntryBuilder_FieldsHaveDistinctKeys protects against accidental
// key collisions in the synthetic field generator — important for any
// downstream test that asserts on N distinct fields.
func Test_EntryBuilder_FieldsHaveDistinctKeys(t *testing.T) {
	t.Parallel()

	b := support.NewEntryBuilder().WithFields(8)
	seen := make(map[string]struct{}, 8)
	for _, f := range b.Fields() {
		_, dup := seen[string(f.Key)]
		assert.False(t, dup, "field key %q duplicated", f.Key)
		seen[string(f.Key)] = struct{}{}
	}
}

// Test_EventBuilder_ProducesLogEvent covers acceptance #3.
func Test_EventBuilder_ProducesLogEvent(t *testing.T) {
	t.Parallel()

	ev := support.NewEventBuilder().
		Level(enum.LevelWarn).
		Body([]byte("hello")).
		Build()

	assert.Equal(t, enum.LevelWarn, ev.Level)
	assert.Equal(t, []byte("hello"), ev.Data)
}

// ctxKey is a typed key to avoid the lint warning about string keys in
// context.WithValue.
type ctxKey string

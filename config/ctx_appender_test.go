//go:build testing

package config_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/stretchr/testify/assert"
)

func Test_ValidateAndAppendField_NonRestricted(t *testing.T) {
	dst := config.ValidateAndAppendField(nil, "req_id", "abc123", map[string]struct{}{})
	assert.Equal(t, `,"req_id":"abc123"`, string(dst))
}

func Test_ValidateAndAppendField_RestrictedKey(t *testing.T) {
	restricted := map[string]struct{}{"level": {}}
	dst := config.ValidateAndAppendField(nil, "level", "hack", restricted)
	expected := `,"` + config.DefaultPrefix + `level":"hack"`
	assert.Equal(t, expected, string(dst))
}

func Test_SetContextFieldsAppender_Priority(t *testing.T) {
	config.TestResetConfig()
	parserCalled := false
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		parserCalled = true
		return map[string]any{"from_parser": "yes"}
	})
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"from_appender":"yes"`...)
	})

	snap := config.GetHotSnapshot()
	ctx := context.Background()
	dst := config.AppendContextFields(ctx, []byte{}, snap)

	assert.False(t, parserCalled, "parser must not be called when appender is registered")
	assert.Contains(t, string(dst), "from_appender")
	assert.NotContains(t, string(dst), "from_parser")
}

func Test_AppendContextFields_NilCtx(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"x":"1"`...)
	})
	snap := config.GetHotSnapshot()

	dst := []byte("before")
	got := config.AppendContextFields(nil, dst, snap)
	assert.Equal(t, "before", string(got))
}

func Test_AppendContextFields_AppenderOutput(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"req_id":"abc"`...)
	})
	snap := config.GetHotSnapshot()

	ctx := context.Background()
	got := config.AppendContextFields(ctx, nil, snap)
	assert.Equal(t, `,"req_id":"abc"`, string(got))
}

func Test_AppendContextFields_LegacyParserStillWorks(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		return map[string]any{"user": "alice"}
	})
	snap := config.GetHotSnapshot()

	ctx := context.Background()
	got := config.AppendContextFields(ctx, nil, snap)
	assert.Contains(t, string(got), `"user":"alice"`)
}

// Test_AppendContextFields_AppenderCollisionBypass documents the explicit
// trade-off of the appender path: unlike variadic fields (which are checked
// against the reserved-key set in logWithSkip), appender-written keys bypass
// the collision check. A key that matches a reserved default key (e.g. "level")
// is written verbatim. This test records the known behaviour so any future
// change to that contract is a deliberate, visible decision.
func Test_AppendContextFields_AppenderCollisionBypass(t *testing.T) {
	config.TestResetConfig()
	// Appender writes "level" — a reserved key — without prefixing.
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"level":"injected"`...)
	})
	snap := config.GetHotSnapshot()

	ctx := context.Background()
	got := config.AppendContextFields(ctx, nil, snap)

	// The appender output is present verbatim — no prefix added.
	assert.Equal(t, `,"level":"injected"`, string(got),
		"appender bypasses reserved-key collision check: caller is responsible for key safety")
}

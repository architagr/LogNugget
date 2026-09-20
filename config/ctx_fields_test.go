//go:build testing

package config_test

import (
	"context"
	"math"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/stretchr/testify/assert"
)

func Test_SetContextFields_StrField(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Str("trace_id", "abc123")
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"trace_id":"abc123"`, string(got))
}

func Test_SetContextFields_IntField(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Int("status", 200)
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"status":200`, string(got))
}

func Test_SetContextFields_UintField(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Uint("count", 42)
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"count":42`, string(got))
}

func Test_SetContextFields_BoolField(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Bool("sampled", true)
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"sampled":true`, string(got))
}

func Test_SetContextFields_Float64Field(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Float64("score", 1.5)
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"score":1.5`, string(got))
}

func Test_SetContextFields_Float64NaN(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Float64("bad", math.NaN())
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"bad":null`, string(got))
}

func Test_SetContextFields_MultipleFields(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Str("trace_id", "t1")
		f.Str("span_id", "s1")
		f.Bool("sampled", true)
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"trace_id":"t1","span_id":"s1","sampled":true`, string(got))
}

func Test_SetContextFields_NilClearsAppender(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Str("x", "y")
	})
	config.SetContextFields(nil)
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Empty(t, got)
}

func Test_SetContextFields_EscapesSpecialChars(t *testing.T) {
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		f.Str("msg", `say "hi"\n`)
	})
	snap := config.GetHotSnapshot()
	got := config.AppendContextFields(context.Background(), nil, snap, &config.CtxFields{})
	assert.Contains(t, string(got), `\"hi\"`)
}

func Test_SetContextFields_ContextPassedThrough(t *testing.T) {
	type key struct{}
	config.TestResetConfig()
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		if v, ok := ctx.Value(key{}).(string); ok {
			f.Str("user_id", v)
		}
	})
	snap := config.GetHotSnapshot()
	ctx := context.WithValue(context.Background(), key{}, "alice")
	got := config.AppendContextFields(ctx, nil, snap, &config.CtxFields{})
	assert.Equal(t, `,"user_id":"alice"`, string(got))
}

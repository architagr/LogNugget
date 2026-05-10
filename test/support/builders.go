// Package support provides test builders and lightweight fakes/spies
// for the lognugget logger. Test-only; consumed by *_test.go across
// the module to keep arrange-phase setup terse and free of singleton
// leak hazards (T-13).
package support

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
)

// ConfigBuilder applies a fluent chain of setters against the
// process-wide config singleton, then registers a tb.Cleanup that
// restores defaults at end of scope. Mitigates T-13.
type ConfigBuilder struct {
	tb testing.TB
	// why: defer apply until Build() so partial state never leaks.
	apply []func()
}

// NewConfigBuilder returns a builder bound to tb.
func NewConfigBuilder(tb testing.TB) *ConfigBuilder {
	tb.Helper()
	return &ConfigBuilder{tb: tb}
}

// MinLevel queues SetMinLevel.
func (b *ConfigBuilder) MinLevel(l enum.LogLevel) *ConfigBuilder {
	b.apply = append(b.apply, func() { config.SetMinLevel(l) })
	return b
}

// Encoder queues SetEncoderType.
func (b *ConfigBuilder) Encoder(e enum.LogEncodeType) *ConfigBuilder {
	b.apply = append(b.apply, func() { config.SetEncoderType(e) })
	return b
}

// Output queues SetOutput.
func (b *ConfigBuilder) Output(w io.Writer) *ConfigBuilder {
	b.apply = append(b.apply, func() { config.SetOutput(w) })
	return b
}

// AddSource queues SetAddSource.
func (b *ConfigBuilder) AddSource(v bool) *ConfigBuilder {
	b.apply = append(b.apply, func() { config.SetAddSource(v) })
	return b
}

// Build applies queued setters and returns a cleanup func that resets
// the singleton; cleanup is also registered via tb.Cleanup.
func (b *ConfigBuilder) Build() func() {
	b.tb.Helper()
	for _, fn := range b.apply {
		fn()
	}
	cleanup := func() { config.ResetConfig() }
	b.tb.Cleanup(cleanup)
	return cleanup
}

// EntryBuilder records inputs (ctx, err, fields) for *entry.LogEntry
// methods and exposes them for assertions.
type EntryBuilder struct {
	ctx    context.Context
	err    error
	fields []model.LogAttr
}

// NewEntryBuilder returns a fresh EntryBuilder.
func NewEntryBuilder() *EntryBuilder { return &EntryBuilder{} }

// WithFields generates n synthetic LogAttr entries with distinct keys
// "k0".."k{n-1}".
func (b *EntryBuilder) WithFields(n int) *EntryBuilder {
	b.fields = make([]model.LogAttr, n)
	for i := 0; i < n; i++ {
		b.fields[i] = model.LogAttr{Key: model.LogAttrKey(fmt.Sprintf("k%d", i)), Value: i}
	}
	return b
}

// WithCtx stores ctx for retrieval via Ctx().
func (b *EntryBuilder) WithCtx(ctx context.Context) *EntryBuilder { b.ctx = ctx; return b }

// WithErr stores err for retrieval via Err().
func (b *EntryBuilder) WithErr(err error) *EntryBuilder { b.err = err; return b }

// Fields returns the recorded attrs.
func (b *EntryBuilder) Fields() []model.LogAttr { return b.fields }

// Ctx returns the recorded context (may be nil).
func (b *EntryBuilder) Ctx() context.Context { return b.ctx }

// Err returns the recorded error (may be nil).
func (b *EntryBuilder) Err() error { return b.err }

// Build returns a fresh *entry.LogEntry from the entry pool. Recorded
// ctx/err/fields are not bound here — LogEntry's fields are unexported,
// so callers pass them at the Log/Info/Error call site themselves.
func (b *EntryBuilder) Build() *entry.LogEntry { return entry.NewLogEntry() }

// EventBuilder constructs config.LogEvent values for tests that bypass
// the full logger entrypoint.
type EventBuilder struct {
	level enum.LogLevel
	data  []byte
}

// NewEventBuilder returns a builder defaulting to LevelInfo.
func NewEventBuilder() *EventBuilder { return &EventBuilder{level: enum.LevelInfo} }

// Level overrides the event level.
func (b *EventBuilder) Level(l enum.LogLevel) *EventBuilder { b.level = l; return b }

// Body overrides the event payload.
func (b *EventBuilder) Body(data []byte) *EventBuilder { b.data = data; return b }

// Build returns a config.LogEvent populated from recorded fields.
func (b *EventBuilder) Build() config.LogEvent {
	return config.LogEvent{Level: b.level, Data: b.data}
}

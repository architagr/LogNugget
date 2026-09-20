// Command configuration walks through every configuration knob and shows what
// each one changes in the output.
//
// All setters are optional: the defaults work with no configuration at all.
// They are startup-time settings — set them in main before serving traffic,
// not per request.
//
// Run: go run ./04-configuration
package main

import (
	"context"
	"os"
	"time"

	"github.com/architagr/lognugget/examples/cookbook/internal/demo"
	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	"github.com/architagr/lognugget/v4/lognugget"
)

func main() {
	defer lognugget.Shutdown()
	demo.Immediate()

	ctx := context.Background()

	// ── Minimum level ───────────────────────────────────────────────────────
	// Records below this level are rejected by an atomic gate before any
	// encoding work happens — a filtered call costs single-digit nanoseconds
	// and allocates nothing. Default: Info.
	demo.Section("SetMinLevel(Warn) — Debug and Info are dropped")
	config.SetMinLevel(enum.LevelWarn)
	entry.NewLogEntry().Debug(ctx, "invisible: below minimum")
	entry.NewLogEntry().Info(ctx, "invisible: below minimum")
	entry.NewLogEntry().Warn(ctx, "visible: at minimum")

	config.SetMinLevel(enum.LevelDebug)

	// ── Timestamp format ────────────────────────────────────────────────────
	// Any layout time.Time understands. Default: time.RFC3339.
	demo.Section("SetTimeFormat")
	config.SetTimeFormat(time.RFC3339Nano)
	entry.NewLogEntry().Info(ctx, "RFC3339 with nanoseconds")
	config.SetTimeFormat("2006-01-02 15:04:05")
	entry.NewLogEntry().Info(ctx, "human-readable layout")
	config.SetTimeFormat(time.RFC3339)

	// ── Caller capture ──────────────────────────────────────────────────────
	// Adds the call site. Off by default because runtime.Callers is the single
	// most expensive thing a log call can do — it pushes the hot path past the
	// 1 µs budget on its own.
	demo.Section("SetAddSource(true) — adds the call site")
	config.SetAddSource(true)
	entry.NewLogEntry().Info(ctx, "with caller")
	config.SetAddSource(false)

	// ── Encoder ─────────────────────────────────────────────────────────────
	// JSON (default) wraps each record in braces. Text passes the rendered
	// body through unchanged.
	demo.Section("SetEncoderType(Text)")
	config.SetEncoderType(enum.EncoderText)
	entry.NewLogEntry().Str("shape", "text").Info(ctx, "text encoder output")
	config.SetEncoderType(enum.EncoderJSON)

	// ── Renaming the built-in keys ──────────────────────────────────────────
	// Match an existing log schema by renaming the core keys. A user field
	// that collides with a core key is prefixed with "custom." rather than
	// emitted twice.
	demo.Section("SetDefaultFields — rename core keys")
	config.SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyTime:    "ts",
		enum.DefaultLogKeyMessage: "msg",
	})
	entry.NewLogEntry().Str("ts", "user value wins a prefix").Info(ctx, "renamed keys")

	// ── Static fields ───────────────────────────────────────────────────────
	// Evaluated once at registration, then included in every record. Use for
	// values that never change: service name, version, host.
	demo.Section("SetStaticEnvFieldsParser — evaluated once")
	config.SetStaticEnvFieldsParser(func() map[string]any {
		host, _ := os.Hostname()
		return map[string]any{
			"service": "checkout-api",
			"version": "4.0.0",
			"host":    host,
		}
	})
	entry.NewLogEntry().Info(ctx, "with static fields")
	config.SetStaticEnvFieldsParser(nil)

	// ── Where records go ────────────────────────────────────────────────────
	// SetOutput retunes the built-in collector. Anything satisfying io.Writer
	// works: a file, a pipe, a rotating writer. For more than one destination
	// use hooks — see 05-hooks.
	demo.Section("SetOutput — redirect the built-in collector")
	config.SetOutput(os.Stderr)
	entry.NewLogEntry().Info(ctx, "this record went to stderr")
	config.SetOutput(os.Stdout)

	// Per-request context fields have their own example: see 03-context.
	// Batching and pool sizing have theirs: see 06-tuning.
	demo.Flush()
}

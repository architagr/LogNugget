// Command context shows the three ways to attach per-request context fields
// — and what logging without any context costs.
//
// Run: go run ./03-context
package main

import (
	"context"

	"github.com/architagr/lognugget/examples/cookbook/internal/demo"
	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/lognugget"
)

type ctxKey string

const (
	ctxKeyTraceID   ctxKey = "trace_id"
	ctxKeyRequestID ctxKey = "request_id"
)

func main() {
	defer lognugget.Shutdown()
	demo.Immediate()

	// ── 1. No context fields ────────────────────────────────────────────────
	// The default. Nothing is extracted from ctx, and context.Background() is
	// a perfectly good argument. This is the cheapest configuration.
	demo.Section("no context fields")
	entry.NewLogEntry().Info(context.Background(), "no context configured")

	// ── 2. Typed context fields (recommended) ───────────────────────────────
	// SetContextFields runs once per record. Add fields with typed methods;
	// LogNugget handles JSON encoding and escaping. Register it once at
	// startup — it is not safe to swap per request.
	demo.Section("SetContextFields (typed, recommended)")
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		// Pull whatever your framework put in the context. With OTel this is
		// where you would read trace.SpanFromContext(ctx) — see
		// examples/otel-appender for the full version.
		if v, ok := ctx.Value(ctxKeyTraceID).(string); ok {
			f.Str("trace_id", v)
		}
		if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
			f.Str("request_id", v)
		}
		f.Str("service", "checkout-api")
		f.Int("pid", 4242)
		f.Bool("canary", false)
	})

	ctx := context.WithValue(
		context.WithValue(context.Background(), ctxKeyTraceID, "4bf92f3577b34da6a3ce929d0e0e4736"),
		ctxKeyRequestID, "req-8817",
	)
	entry.NewLogEntry().Str("path", "/checkout").Info(ctx, "request handled")

	// A request without those values still logs; the callback simply adds
	// fewer fields.
	entry.NewLogEntry().Info(context.Background(), "background job, no request context")

	// ── 3. Raw appender (power users) ───────────────────────────────────────
	// SetContextFieldsAppender hands you the record buffer. You write the raw
	// JSON fragment yourself, which means you own the escaping — use it when
	// the fields are fixed-shape and you want to skip per-field encoding.
	demo.Section("SetContextFieldsAppender (raw bytes)")
	config.SetContextFields(nil) // appender takes precedence, but be explicit
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"service":"checkout-api","zone":"us-east-1a"`...)
	})
	entry.NewLogEntry().Info(ctx, "raw appender output")

	// ── 4. Legacy map parser (discouraged) ──────────────────────────────────
	// SetContextFieldsParser returns map[string]any. It still works, but it
	// allocates a map per record and costs roughly 2× the typed API at ten
	// fields. Prefer SetContextFields for new code.
	demo.Section("SetContextFieldsParser (legacy map)")
	config.SetContextFieldsAppender(nil)
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		return map[string]any{
			"service":    "checkout-api",
			"request_id": ctx.Value(ctxKeyRequestID),
		}
	})
	entry.NewLogEntry().Info(ctx, "legacy parser output")

	// Precedence when more than one is set: appender > typed > legacy parser.
	config.SetContextFieldsParser(nil)
	demo.Flush()
}

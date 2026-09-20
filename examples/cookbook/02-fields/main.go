// Command fields shows the three ways to attach data to a record, and what
// logging without any fields looks like.
//
// Run: go run ./02-fields
package main

import (
	"context"
	"errors"

	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/lognugget"
	"github.com/architagr/lognugget/model"
)

func main() {
	defer lognugget.Shutdown()
	ctx := context.Background()

	// 1. No fields. A bare message is a complete record: time, level, message.
	entry.NewLogEntry().Info(ctx, "worker loop started")

	// 2. Typed chain methods — the fast path. Each call appends directly to
	// the entry's buffer: no map, no interface boxing, no allocation.
	//
	//   Str     (key, string)
	//   Int     (key, int64)
	//   Uint    (key, uint64)
	//   Float64 (key, float64)   NaN and ±Inf are written as JSON null
	//   Bool    (key, bool)
	//   Err     (error)          writes "error"; a nil error is a no-op
	//   Any     (key, any)       type-switch fallback; boxes the value
	entry.NewLogEntry().
		Str("method", "GET").
		Str("path", "/api/v1/users").
		Int("status", 200).
		Int("bytes", 5_120).
		Float64("duration_ms", 12.5).
		Bool("cache_hit", true).
		Info(ctx, "request handled")

	// Err attaches an error to any level, not just Error().
	entry.NewLogEntry().
		Str("path", "/api/v1/orders").
		Err(errors.New("upstream timeout")).
		Warn(ctx, "retrying request")

	// Any accepts whatever does not have a typed method. It costs an
	// interface box, so prefer a typed method when one fits.
	entry.NewLogEntry().
		Any("retry_after", []string{"1s", "2s", "4s"}).
		Info(ctx, "backoff schedule computed")

	// 3. Variadic model.LogAttr — useful when fields are assembled elsewhere
	// and passed around as a slice.
	attrs := []model.LogAttr{
		model.Str("tenant", "acme"),
		model.Int("shard", 7),
		model.Bool("primary", false),
	}
	entry.NewLogEntry().Info(ctx, "shard selected", attrs...)

	// Error() takes the error as its own argument, before the message.
	err := errors.New("connection refused")
	entry.NewLogEntry().
		Str("host", "db-primary:5432").
		Int("attempt", 3).
		Error(ctx, err, "database unreachable")

	// Reserved keys are not silently overwritten: a user field named "time",
	// "level", "message", "error" or "caller" is prefixed with "custom." so
	// the record stays unambiguous.
	entry.NewLogEntry().
		Any("time", "this is my own field").
		Info(ctx, "reserved key collision")
}

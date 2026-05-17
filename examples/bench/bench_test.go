// Package bench provides cross-logger benchmark comparisons at the HTTP handler
// level. Each benchmark simulates a realistic Gin handler that:
//
//   - Builds a context with exactly 10 fields (trace_id, span_id, request_id,
//     user_id, tenant_id, session_id, env, region, service, version)
//   - Logs one Info message with 2 additional call-site attrs (method, path)
//   - Writes to io.Discard so IO cost is excluded
//
// LogNugget is async (channel + goroutine drain); logrus and zerolog write
// synchronously. The LogNugget numbers represent caller-side cost only.
// Run: go test -bench=. -benchmem -count=10 -run=^$
package bench

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
)

// ctxKey avoids string-key collisions in context.
type ctxKey string

// ctx10 builds a context carrying 10 realistic request-scoped fields.
func ctx10() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKey("trace_id"), "4bf92f3577b34da6a3ce929d0e0e4736")
	ctx = context.WithValue(ctx, ctxKey("span_id"), "00f067aa0ba902b7")
	ctx = context.WithValue(ctx, ctxKey("request_id"), "req-abc-123")
	ctx = context.WithValue(ctx, ctxKey("user_id"), "usr-0042")
	ctx = context.WithValue(ctx, ctxKey("tenant_id"), "tenant-acme")
	ctx = context.WithValue(ctx, ctxKey("session_id"), "sess-xyz-789")
	ctx = context.WithValue(ctx, ctxKey("env"), "production")
	ctx = context.WithValue(ctx, ctxKey("region"), "us-east-1")
	ctx = context.WithValue(ctx, ctxKey("service"), "api-gateway")
	ctx = context.WithValue(ctx, ctxKey("version"), "v1.0.0")
	return ctx
}

// setupLognugget configures the LogNugget pipeline once for the whole test
// binary and returns a stop func.
var lognuggetOnce sync.Once

func setupLognugget() {
	lognuggetOnce.Do(func() {
		config.SetOutput(io.Discard)
		config.SetMinLevel(enum.LevelDebug)
		config.SetEncoderType(enum.EncoderJSON)
		config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
			return map[string]any{
				"trace_id":   ctx.Value(ctxKey("trace_id")),
				"span_id":    ctx.Value(ctxKey("span_id")),
				"request_id": ctx.Value(ctxKey("request_id")),
				"user_id":    ctx.Value(ctxKey("user_id")),
				"tenant_id":  ctx.Value(ctxKey("tenant_id")),
				"session_id": ctx.Value(ctxKey("session_id")),
				"env":        ctx.Value(ctxKey("env")),
				"region":     ctx.Value(ctxKey("region")),
				"service":    ctx.Value(ctxKey("service")),
				"version":    ctx.Value(ctxKey("version")),
			}
		})
		hook := pipelineStage.NewUnsetLogEventPostProcessor(
			500*time.Millisecond, 100, io.Discard,
		)
		pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, hook)
		config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
		entry.GenerateInitialPool(1000)
	})
}

// ─── LogNugget ───────────────────────────────────────────────────────────────

func BenchmarkLognugget_Serial_10CtxFields(b *testing.B) {
	setupLognugget()
	ctx := ctx10()
	attrs := []model.LogAttr{
		{Key: "method", Value: "GET"},
		{Key: "path", Value: "/api/users"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := entry.NewLogEntry()
		e.Info(ctx, "request processed", attrs...)
	}
}

func BenchmarkLognugget_Parallel_10CtxFields(b *testing.B) {
	setupLognugget()
	ctx := ctx10()
	attrs := []model.LogAttr{
		{Key: "method", Value: "GET"},
		{Key: "path", Value: "/api/users"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			e := entry.NewLogEntry()
			e.Info(ctx, "request processed", attrs...)
		}
	})
}

func BenchmarkLognugget_Filtered_BelowMinLevel(b *testing.B) {
	setupLognugget()
	// Set min to Error so Debug is filtered at the gate (no alloc, no encode).
	prev := os.Stderr
	_ = prev
	config.SetMinLevel(enum.LevelError)
	defer config.SetMinLevel(enum.LevelDebug)

	ctx := ctx10()
	attrs := []model.LogAttr{{Key: "method", Value: "GET"}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := entry.NewLogEntry()
		e.Debug(ctx, "filtered debug", attrs...)
	}
}

// ─── zerolog ─────────────────────────────────────────────────────────────────

func BenchmarkZerolog_Serial_10CtxFields(b *testing.B) {
	logger := zerolog.New(io.Discard).With().Timestamp().Logger()
	ctx := ctx10()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info().
			Str("trace_id", ctx.Value(ctxKey("trace_id")).(string)).
			Str("span_id", ctx.Value(ctxKey("span_id")).(string)).
			Str("request_id", ctx.Value(ctxKey("request_id")).(string)).
			Str("user_id", ctx.Value(ctxKey("user_id")).(string)).
			Str("tenant_id", ctx.Value(ctxKey("tenant_id")).(string)).
			Str("session_id", ctx.Value(ctxKey("session_id")).(string)).
			Str("env", ctx.Value(ctxKey("env")).(string)).
			Str("region", ctx.Value(ctxKey("region")).(string)).
			Str("service", ctx.Value(ctxKey("service")).(string)).
			Str("version", ctx.Value(ctxKey("version")).(string)).
			Str("method", "GET").
			Str("path", "/api/users").
			Msg("request processed")
	}
}

func BenchmarkZerolog_Parallel_10CtxFields(b *testing.B) {
	logger := zerolog.New(io.Discard).With().Timestamp().Logger()
	ctx := ctx10()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info().
				Str("trace_id", ctx.Value(ctxKey("trace_id")).(string)).
				Str("span_id", ctx.Value(ctxKey("span_id")).(string)).
				Str("request_id", ctx.Value(ctxKey("request_id")).(string)).
				Str("user_id", ctx.Value(ctxKey("user_id")).(string)).
				Str("tenant_id", ctx.Value(ctxKey("tenant_id")).(string)).
				Str("session_id", ctx.Value(ctxKey("session_id")).(string)).
				Str("env", ctx.Value(ctxKey("env")).(string)).
				Str("region", ctx.Value(ctxKey("region")).(string)).
				Str("service", ctx.Value(ctxKey("service")).(string)).
				Str("version", ctx.Value(ctxKey("version")).(string)).
				Str("method", "GET").
				Str("path", "/api/users").
				Msg("request processed")
		}
	})
}

// ─── logrus ──────────────────────────────────────────────────────────────────

func BenchmarkLogrus_Serial_10CtxFields(b *testing.B) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetFormatter(&logrus.JSONFormatter{})
	ctx := ctx10()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.WithContext(ctx).WithFields(logrus.Fields{
			"trace_id":   ctx.Value(ctxKey("trace_id")),
			"span_id":    ctx.Value(ctxKey("span_id")),
			"request_id": ctx.Value(ctxKey("request_id")),
			"user_id":    ctx.Value(ctxKey("user_id")),
			"tenant_id":  ctx.Value(ctxKey("tenant_id")),
			"session_id": ctx.Value(ctxKey("session_id")),
			"env":        ctx.Value(ctxKey("env")),
			"region":     ctx.Value(ctxKey("region")),
			"service":    ctx.Value(ctxKey("service")),
			"version":    ctx.Value(ctxKey("version")),
			"method":     "GET",
			"path":       "/api/users",
		}).Info("request processed")
	}
}

func BenchmarkLogrus_Parallel_10CtxFields(b *testing.B) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetFormatter(&logrus.JSONFormatter{})
	ctx := ctx10()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			log.WithContext(ctx).WithFields(logrus.Fields{
				"trace_id":   ctx.Value(ctxKey("trace_id")),
				"span_id":    ctx.Value(ctxKey("span_id")),
				"request_id": ctx.Value(ctxKey("request_id")),
				"user_id":    ctx.Value(ctxKey("user_id")),
				"tenant_id":  ctx.Value(ctxKey("tenant_id")),
				"session_id": ctx.Value(ctxKey("session_id")),
				"env":        ctx.Value(ctxKey("env")),
				"region":     ctx.Value(ctxKey("region")),
				"service":    ctx.Value(ctxKey("service")),
				"version":    ctx.Value(ctxKey("version")),
				"method":     "GET",
				"path":       "/api/users",
			}).Info("request processed")
		}
	})
}

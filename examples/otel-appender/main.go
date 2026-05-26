// Package main demonstrates integrating LogNugget with OpenTelemetry by
// registering a SetContextFields callback that injects trace_id and span_id
// into every log line from an active span.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	// Configure LogNugget.
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)

	// Register OTel context fields — LogNugget handles JSON encoding internally.
	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return
		}
		sc := span.SpanContext()
		f.Str("trace_id", sc.TraceID().String())
		f.Str("span_id", sc.SpanID().String())
	})

	out := &stdoutWriter{}
	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 100, out)
	defer proc.Stop()
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)

	// Log without a span — appender is a no-op.
	entry.NewLogEntry().Info(context.Background(), "no active span")

	// Log inside a synthetic span to demonstrate field injection.
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		log.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		log.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	entry.NewLogEntry().Info(ctx, "inside traced request")

	// Give the async pipeline time to flush.
	time.Sleep(100 * time.Millisecond)
}

type stdoutWriter struct{}

func (w *stdoutWriter) Write(p []byte) (int, error) {
	fmt.Println(string(p))
	return len(p), nil
}

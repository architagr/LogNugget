// Package main demonstrates integrating LogNugget with OpenTelemetry by
// registering a ContextFieldsAppender that injects trace_id and span_id into
// every log line from an active span — zero allocations on the hot path.
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

	// Register the OTel appender — zero allocations when a span is active.
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return dst
		}
		sc := span.SpanContext()
		dst = append(dst, `,"trace_id":"`...)
		dst = append(dst, sc.TraceID().String()...)
		dst = append(dst, `","span_id":"`...)
		dst = append(dst, sc.SpanID().String()...)
		dst = append(dst, '"')
		return dst
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

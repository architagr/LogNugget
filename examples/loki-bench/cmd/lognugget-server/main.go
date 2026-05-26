// lognugget-server is the benchmark HTTP server using LogNugget → Loki.
// It exposes GET /api/v1/work which simulates 1 ms of CPU work, logs one
// structured event per request (OTel trace/span IDs via B3 propagation),
// and serves on :8081.
//
// Run: go run ./cmd/lognugget-server
// Env: LOKI_URL (default http://localhost:3100)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "lognugget-server"

func main() {
	// OTel: no-op exporter — spans carry real IDs but are not shipped anywhere.
	// B3 multi-header propagator handles X-B3-TraceId / X-B3-SpanId / X-B3-Sampled.
	shutdown := initOTel()
	defer shutdown()

	lokiURL := envOr("LOKI_URL", "http://localhost:3100")
	writer := newLokiWriter(lokiURL, "lognugget-server")

	// Wire LogNugget: async ring → Loki hook.
	config.SetMinLevel(enum.LevelInfo)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetOutput(writer)

	// Zero-alloc OTel context appender — reads trace/span IDs from the OTel
	// span stored in ctx. Called on the hot path; pre-rendered bytes, no boxing.
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		sc := trace.SpanFromContext(ctx).SpanContext()
		if sc.IsValid() {
			dst = append(dst, `,"trace_id":"`...)
			dst = append(dst, sc.TraceID().String()...)
			dst = append(dst, `","span_id":"`...)
			dst = append(dst, sc.SpanID().String()...)
			dst = append(dst, '"')
		}
		return dst
	})

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 4096, writer)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(runtime.GOMAXPROCS(0) * 64)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/work", workHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	addr := envOr("ADDR", ":8081")
	fmt.Printf("lognugget-server listening on %s  loki=%s  GOMAXPROCS=%d\n",
		addr, lokiURL, runtime.GOMAXPROCS(0))

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func initOTel() func() {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader)))
	return func() { tp.Shutdown(context.Background()) } //nolint:errcheck
}

func workHandler(w http.ResponseWriter, r *http.Request) {
	// Extract B3 trace context from incoming headers; create child span.
	// otel.Tracer resolves the global provider at call time — after initOTel.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := otel.Tracer(tracerName).Start(ctx, "handle-work")
	defer span.End()

	start := time.Now()
	doWork()
	lat := time.Since(start)

	// ctx carries the OTel span — the ContextFieldsAppender extracts trace/span IDs.
	entry.NewLogEntry().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Int("status", 200).
		Int("latency_us", lat.Microseconds()).
		Info(ctx, "request")

	sc := span.SpanContext()
	w.Header().Set("X-B3-TraceId", sc.TraceID().String())
	w.Header().Set("X-B3-SpanId", sc.SpanID().String())
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","latency_us":%d}`, lat.Microseconds())
}

// doWork simulates ~1 ms of CPU-bound work.
func doWork() {
	deadline := time.Now().Add(1 * time.Millisecond)
	x := uint64(1)
	for time.Now().Before(deadline) {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
	}
	_ = x
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// lokiWriter implements io.Writer and pipelineStage.PublishLogMessageHookContract,
// batching log lines and pushing them to Loki asynchronously from LogNugget's
// consumer goroutine (never on the HTTP handler goroutine).
type lokiWriter struct {
	url    string
	app    string
	client *http.Client
}

func newLokiWriter(lokiBase, app string) *lokiWriter {
	return &lokiWriter{
		url:    lokiBase + "/loki/api/v1/push",
		app:    app,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (w *lokiWriter) Write(p []byte) (int, error) {
	return w.push(p)
}

func (w *lokiWriter) PublishLogMessage(entry []byte) {
	_, _ = w.push(entry)
}

func (w *lokiWriter) Name() string { return "loki" }

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func (w *lokiWriter) push(p []byte) (int, error) {
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	payload := lokiPush{
		Streams: []lokiStream{{
			Stream: map[string]string{"app": w.app, "logger": "lognugget"},
			Values: [][2]string{{ts, string(p)}},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	resp, err := w.client.Post(w.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()
	return len(p), nil
}

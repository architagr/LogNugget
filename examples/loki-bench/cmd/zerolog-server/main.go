// zerolog-server is the benchmark HTTP server using zerolog → Loki.
// It exposes GET /api/v1/work which simulates 1 ms of CPU work, logs one
// structured event per request (with OTel trace/span IDs via B3 propagation),
// and serves on :8080.
//
// Run: go run ./cmd/zerolog-server
// Env: LOKI_URL (default http://localhost:3100)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const tracerName = "zerolog-server"

func main() {
	// OTel: no-op exporter — spans carry real IDs but are not shipped anywhere.
	// B3 multi-header propagator handles X-B3-TraceId / X-B3-SpanId / X-B3-Sampled.
	shutdown := initOTel()
	defer shutdown()

	lokiURL := envOr("LOKI_URL", "http://localhost:3100")
	writer := newLokiWriter(lokiURL, "zerolog-server")

	log := zerolog.New(writer).With().
		Str("app", "zerolog-server").
		Logger()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/work", workHandler(log))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	addr := envOr("ADDR", ":8080")
	fmt.Printf("zerolog-server listening on %s  loki=%s  GOMAXPROCS=%d\n",
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

func workHandler(log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract B3 trace context from incoming headers; create child span.
		// otel.Tracer resolves global provider at call time — after initOTel.
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer(tracerName).Start(ctx, "handle-work")
		defer span.End()
		_ = ctx

		start := time.Now()
		doWork()
		lat := time.Since(start)

		sc := span.SpanContext()
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", 200).
			Int64("latency_us", lat.Microseconds()).
			Str("trace_id", sc.TraceID().String()).
			Str("span_id", sc.SpanID().String()).
			Msg("request")

		w.Header().Set("X-B3-TraceId", sc.TraceID().String())
		w.Header().Set("X-B3-SpanId", sc.SpanID().String())
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","latency_us":%d}`, lat.Microseconds())
	}
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

func newTraceID() string {
	return strconv.FormatUint(rand.Uint64(), 16)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// lokiWriter is a synchronous io.Writer that POSTs each log line to Loki.
type lokiWriter struct {
	url    string
	client *http.Client
}

func newLokiWriter(lokiBase, app string) *lokiWriter {
	return &lokiWriter{
		url:    lokiBase + "/loki/api/v1/push",
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func (w *lokiWriter) Write(p []byte) (int, error) {
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	payload := lokiPush{
		Streams: []lokiStream{{
			Stream: map[string]string{"app": "zerolog-server", "logger": "zerolog"},
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

var _ = newTraceID // suppress unused warning — kept for test helpers

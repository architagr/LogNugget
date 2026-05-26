// lognugget-server is the benchmark HTTP server using LogNugget → Loki.
// It exposes GET /api/v1/work which simulates 1 ms of CPU work, logs one
// structured event per request, and serves on :8081.
//
// Run: go run ./examples/loki-bench/cmd/lognugget-server
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

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

func main() {
	lokiURL := envOr("LOKI_URL", "http://localhost:3100")
	writer := newLokiWriter(lokiURL, "lognugget-server")

	// Wire LogNugget: async ring → Loki hook.
	config.SetMinLevel(enum.LevelInfo)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetOutput(writer)

	// OTel-style context appender: zero-alloc pre-rendered fields.
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		if tid, ok := ctx.Value(traceIDKey{}).(string); ok && tid != "" {
			dst = append(dst, `,"trace_id":"`...)
			dst = append(dst, tid...)
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

type traceIDKey struct{}

func workHandler(w http.ResponseWriter, r *http.Request) {
	traceID := newTraceID()
	ctx := context.WithValue(r.Context(), traceIDKey{}, traceID)

	start := time.Now()
	doWork()
	lat := time.Since(start)

	entry.NewLogEntry().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Int("status", 200).
		Int("latency_us", int64(lat.Microseconds())).
		Info(ctx, "request")

	w.Header().Set("X-Trace-Id", traceID)
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

func newTraceID() string {
	return strconv.FormatUint(rand.Uint64(), 16)
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

// Write satisfies io.Writer and config.SetOutput. For LogNugget, writes arrive
// from ProcessLogEvent (not the handler goroutine) — no impact on handler latency.
func (w *lokiWriter) Write(p []byte) (int, error) {
	return w.push(p)
}

// PublishLogMessage satisfies PublishLogMessageHookContract.
func (w *lokiWriter) PublishLogMessage(entry []byte) {
	_, _ = w.push(entry)
}

// Name satisfies PublishLogMessageHookContract.
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
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return len(p), nil
}

// Package lokibench_test contains a self-contained micro-benchmark that proves
// the async-vs-sync latency story without requiring a live Loki instance.
//
// It simulates the Loki HTTP round-trip by sleeping inside the writer, then
// measures the handler latency seen by the caller. The key insight: when IO is
// slow, the async ring-buffer path keeps handler latency near the CPU-work
// cost; the sync path forces the caller to block for the full IO RTT.
//
// Run:
//
//	cd examples/loki-bench && go test -bench=. -benchmem -count=5 -run=^$
package lokibench_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/rs/zerolog"
)

// slowWriter wraps an io.Writer and injects a configurable sleep before each
// write to simulate an HTTP round-trip to a remote Loki instance.
type slowWriter struct {
	delay time.Duration
}

func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	return len(p), nil
}

// PublishLogMessage satisfies the LogNugget hook contract — called on the
// async consumer goroutine, never on the handler goroutine.
func (w *slowWriter) PublishLogMessage(entry []byte) { w.Write(entry) } //nolint:errcheck
func (w *slowWriter) Name() string                   { return "slow-loki-sim" }

// doWork spins for ~1 ms of CPU work (matches the example server).
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

// ─── zerolog sync path ───────────────────────────────────────────────────────

// BenchmarkHandler_Zerolog_Sync_1msLoki measures handler latency when zerolog
// writes synchronously to a 1 ms "Loki" writer. The handler blocks for the
// full IO RTT before it can return a response.
func BenchmarkHandler_Zerolog_Sync_1msLoki(b *testing.B) {
	w := &slowWriter{delay: 1 * time.Millisecond}
	log := zerolog.New(w).With().Str("app", "zerolog-server").Logger()

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		doWork()
		lat := time.Since(start)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", 200).
			Int64("latency_us", lat.Microseconds()).
			Msg("request")

		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, `{"status":"ok","latency_us":%d}`, lat.Microseconds())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// BenchmarkHandler_Zerolog_Sync_5msLoki repeats the sync benchmark at 5 ms
// latency (representative of a remote Loki in a different availability zone).
func BenchmarkHandler_Zerolog_Sync_5msLoki(b *testing.B) {
	w := &slowWriter{delay: 5 * time.Millisecond}
	log := zerolog.New(w).With().Str("app", "zerolog-server").Logger()

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		doWork()
		lat := time.Since(start)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", 200).
			Int64("latency_us", lat.Microseconds()).
			Msg("request")

		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, `{"status":"ok","latency_us":%d}`, lat.Microseconds())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// ─── LogNugget async path ────────────────────────────────────────────────────

type traceIDKey struct{}

// BenchmarkHandler_LogNugget_Async_1msLoki measures handler latency when
// LogNugget writes asynchronously. The handler only enqueues the event to the
// ring buffer; the consumer goroutine performs the 1 ms Loki write off-path.
func BenchmarkHandler_LogNugget_Async_1msLoki(b *testing.B) {
	w := &slowWriter{delay: 1 * time.Millisecond}

	config.SetMinLevel(enum.LevelInfo)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetOutput(w)
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		if tid, ok := ctx.Value(traceIDKey{}).(string); ok && tid != "" {
			dst = append(dst, `,"trace_id":"`...)
			dst = append(dst, tid...)
			dst = append(dst, '"')
		}
		return dst
	})

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 4096, w)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(runtime.GOMAXPROCS(0) * 64)
	b.Cleanup(proc.Stop)

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		traceID := strconv.FormatUint(uint64(b.N), 16)
		ctx := context.WithValue(r.Context(), traceIDKey{}, traceID)

		start := time.Now()
		doWork()
		lat := time.Since(start)

		entry.NewLogEntry().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", 200).
			Int("latency_us", lat.Microseconds()).
			Info(ctx, "request")

		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, `{"status":"ok","latency_us":%d}`, lat.Microseconds())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// BenchmarkHandler_LogNugget_Async_5msLoki repeats at 5 ms simulated RTT.
func BenchmarkHandler_LogNugget_Async_5msLoki(b *testing.B) {
	w := &slowWriter{delay: 5 * time.Millisecond}

	config.SetMinLevel(enum.LevelInfo)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetOutput(w)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 4096, w)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(runtime.GOMAXPROCS(0) * 64)
	b.Cleanup(proc.Stop)

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		doWork()
		lat := time.Since(start)

		entry.NewLogEntry().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", 200).
			Int("latency_us", lat.Microseconds()).
			Info(context.Background(), "request")

		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, `{"status":"ok","latency_us":%d}`, lat.Microseconds())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// ─── JSON body parser (used by the example k6 script) ───────────────────────

type workResponse struct {
	Status    string `json:"status"`
	LatencyUs int64  `json:"latency_us"`
}

func parseWorkResponse(body []byte) (workResponse, error) {
	var r workResponse
	return r, json.Unmarshal(body, &r)
}

// BenchmarkHandler_LogNugget_Async_Parallel_1msLoki measures parallel handler
// throughput — more representative of production load at 1 ms Loki RTT.
func BenchmarkHandler_LogNugget_Async_Parallel_1msLoki(b *testing.B) {
	w := &slowWriter{delay: 1 * time.Millisecond}

	config.SetMinLevel(enum.LevelInfo)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetOutput(w)

	proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 4096, w)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(runtime.GOMAXPROCS(0) * 256)
	b.Cleanup(proc.Stop)

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		doWork()
		lat := time.Since(start)

		entry.NewLogEntry().
			Str("method", r.Method).
			Int("latency_us", lat.Microseconds()).
			Info(context.Background(), "request")

		rw.WriteHeader(http.StatusOK)
		io.WriteString(rw, `{"status":"ok"}`) //nolint:errcheck
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
		for pb.Next() {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	})
}

// BenchmarkHandler_Zerolog_Sync_Parallel_1msLoki baseline for parallel sync.
func BenchmarkHandler_Zerolog_Sync_Parallel_1msLoki(b *testing.B) {
	var mu bytes.Buffer
	_ = mu // silence unused warning
	w := &slowWriter{delay: 1 * time.Millisecond}
	log := zerolog.New(w).With().Str("app", "zerolog-server").Logger()

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		doWork()
		lat := time.Since(start)

		log.Info().
			Str("method", r.Method).
			Int64("latency_us", lat.Microseconds()).
			Msg("request")

		rw.WriteHeader(http.StatusOK)
		io.WriteString(rw, `{"status":"ok"}`) //nolint:errcheck
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/work", nil)
		for pb.Next() {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	})
}

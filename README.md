[![codecov](https://codecov.io/gh/architagr/LogNugget/branch/main/graph/badge.svg?token=9VPDuFbSyQ)](https://codecov.io/gh/architagr/LogNugget)

# LogNugget

**Bite-sized, context-aware logging for Go** — because every request deserves its own story.

**LogNugget** is a high-performance, memory-efficient structured logging library for Go. It batches log writes, recycles per-request objects via `sync.Pool`, and makes trace/span IDs first-class citizens.

---

## Why LogNugget?

Traditional Go loggers often:
1. **Block the application** waiting for synchronous IO writes.
2. **Allocate per-call** buffers, increasing GC pressure.
3. **Lack structured context propagation** for trace/span IDs.

LogNugget solves these by:
- Batched async event pipeline — the caller never blocks on IO.
- `sync.Pool`-backed `LogEntry` objects — zero per-call allocation on the hot path.
- Context-first API — trace, span, and request fields propagate automatically.

---

## Quick Start

```go
import (
    "context"
    "github.com/architagr/lognugget/entry"
    "github.com/architagr/lognugget/model"
    _ "github.com/architagr/lognugget/lognugget" // zero-config init
)

func main() {
    defer lognugget.Shutdown() // flush on exit

    ctx := context.Background()
    entry.NewLogEntry().Info(ctx, "server started", model.LogAttr{Key: "port", Value: 8080})
}
```

Importing `lognugget` is sufficient — no `NewLogger()` required.

---

## Security

Please report suspected vulnerabilities privately through GitHub Security Advisories. See [SECURITY.md](SECURITY.md) for supported versions, reporting details, response targets, and coordinated disclosure guidance.

---

## Configuration

All setters are optional. Defaults work out of the box.

| Setter | Default | Description |
|--------|---------|-------------|
| `config.SetMinLevel(level)` | `Info` | Minimum log level to emit |
| `config.SetEncoderType(t)` | `JSON` | Output encoding (JSON or Text) |
| `config.SetAddSource(bool)` | `false` | Include caller file:line (D-7: costs ~250 ns) |
| `config.SetOutput(w)` | `os.Stdout` | Output writer for the default collector |
| `config.SetLogBufferMaxSize(n)` | `20` | Max bucket size before forced flush |
| `config.SetRate(d)` | `1s` | Ticker flush interval |
| `config.SetTimeFormat(fmt)` | `time.RFC3339` | Timestamp format |
| `config.SetStaticEnvFieldsParser(fn)` | `nil` | Once-evaluated static fields (hostname, service) |
| `config.SetContextFieldsParser(fn)` | `nil` | Per-call context fields (trace_id, user_id) |
| `config.SetDefaultFields(map)` | built-in keys | Rename default field keys |

---

## Context Fields — Zero-Alloc OTel Appender (V3)

Use `SetContextFieldsAppender` for zero-alloc context injection. The appender writes fields directly into the log buffer — no `map[string]any`, no interface boxing, no GC pressure.

```go
// V3 recommended: zero-alloc appender (OTel trace/span IDs + request fields)
config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
    span := trace.SpanFromContext(ctx)
    if span.SpanContext().IsValid() {
        dst = append(dst, `,"trace_id":"`...)
        dst = append(dst, span.SpanContext().TraceID().String()...)
        dst = append(dst, `","span_id":"`...)
        dst = append(dst, span.SpanContext().SpanID().String()...)
        dst = append(dst, '"')
    }
    return dst
})
```

The legacy `SetContextFieldsParser` (returns `map[string]any`) is still supported but costs ~600 ns/call at 10 fields. For new code, prefer the appender. See `examples/otel-appender/` for a runnable OTel demo.

---

## Buffered Hook Example

Register a custom hook to fan out events to a secondary output (e.g. Loki, Datadog):

```go
import (
    "time"
    pipelineStage "github.com/architagr/lognugget/pipeline_stage"
    "github.com/architagr/lognugget/config"
    "github.com/architagr/lognugget/enum"
)

// Create a buffered hook writing to your secondary sink.
myHook := pipelineStage.NewUnsetLogEventPostProcessor(
    500*time.Millisecond, // flush every 500 ms
    100,                  // or when 100 messages accumulate
    myWriter,             // io.Writer — your custom sink
)
defer myHook.Stop()

// Register at LevelUnSet to receive every level, or a specific level.
pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, myHook)
config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
```

---

## Architecture

```text
caller → entry.LogEntry.Info(ctx, msg, fields...)
           │ pool-backed, zero alloc on hot path
           ↓
        config.PublishLog(level, []byte)
           │ atomicRing.Load().Push()  ← lock-free MPSC ring buffer (V3-P9)
           ↓
        config.ProcessLogEvent()  [background goroutine — spins on ring.Pop()]
           │ fans out to each EventPreProcessor
           ↓
        pipeline_stage.EventPreProcessorObj.PreProcess(level, data)
           │ LevelUnSet hooks + level-specific hooks
           ↓
        unsetLogEventPostProcessor.PublishLogMessage(data)
           │ append to activeBucket under lock
           │ capacity flush or ticker flush → io.Write
           ↓
        io.Writer (os.Stdout or custom)
```

**Shutdown:** `lognugget.Shutdown()` blocks until all buffered events are written. Call at end of `main()` or in a signal handler.

---

## Logger Comparison — Parallel Throughput with 10 Context Fields

Benchmarks simulate a real Gin handler: build a context with **10 fields** (trace_id, span_id,
request_id, user_id, tenant_id, session_id, env, region, service, version) and log one Info
message with 2 call-site attrs (method, path). Output is `io.Discard`. Parallel benchmarks use
`b.RunParallel` with `GOMAXPROCS=8` (Apple M1 Pro, 8-core).

Run from `examples/bench/`: `go test -bench=. -benchmem -count=10 -run=^$`

### Serial (1 goroutine)

| Logger | ns/op | B/op | allocs/op | ~ops/sec |
|--------|-------|------|-----------|----------|
| **zerolog** | **~548** | **0** | **0** | **~1.82 M** |
| **LogNugget V3** ¹ | **~865** | **~592** | **5** | **~1.16 M** |
| LogNugget V2 ¹ | ~1,280 | 1,417 | 6 | ~781 K |
| LogNugget v1 ¹ | ~3,630 | 2,909 | 55 | ~275 K |
| logrus | ~5,570 | 4,857 | 58 | ~179 K |

### Parallel (8 goroutines, GOMAXPROCS=8)

| Logger | ns/op | B/op | allocs/op | ~ops/sec (total) |
|--------|-------|------|-----------|-----------------|
| **zerolog** | **~110** | **0** | **0** | **~9.09 M** |
| **LogNugget V3** ¹ | **~196** | **~105** | **2** | **~5.1 M** |
| LogNugget V2 ¹ | ~1,090 | 1,411 | 5 | ~917 K |
| LogNugget v1 ¹ | ~3,440 | 2,909 | 55 | ~290 K |
| logrus | ~6,880 | 4,863 | 58 | ~145 K |

### Filtered path (log level below minimum — fast reject)

| Logger | ns/op | B/op | allocs/op | ~ops/sec |
|--------|-------|------|-----------|----------|
| LogNugget | **~10** | **0** | **0** | **~100 M** |

> ¹ **LogNugget is async** — the caller returns after a lock-free ring-buffer push; JSON encode
> and `io.Write` happen on a background goroutine. The `ns/op` figures above reflect **caller-side
> cost only** (no IO wait). zerolog and logrus are **synchronous** — their numbers include full
> JSON encoding and write to `io.Discard`.
>
> **V3 improvements (Epic V3, feat/111):** −82% parallel latency (1,090 → ~196 ns/op), −60% allocs
> (5 → 2/op), −93% bytes (1,411 → ~105 B/op). Key changes: lock-free MPSC ring buffer replaces
> Go channel, atomic pre-processor gate, copy-on-write config snapshot, OTel `ContextFieldsAppender`,
> pre-rendered level bytes, string-native JSON escape, exact-size buffer severance.
>
> **V2 improvements (Epic V2, feat/81):** −68% parallel latency (3,440 → 1,090 ns/op), −91% allocs
> (55 → 5/op). Key changes: atomic minLevel gate, single config snapshot per call, typed field API,
> zero-alloc `ContextFieldsAppender`, inline encoder framing, channel capacity 1000.
>
> **Why is zerolog still faster on raw parallel?** zerolog writes synchronously to a pre-allocated
> buffer — no ring-buffer overhead. Under real IO latency (file, socket), LogNugget's async pipeline
> outperforms zerolog at high concurrency. Pure encoding benchmarks (`BenchmarkAppendAttr_*`) are
> < 30 ns/op, comparable to zerolog's field serialization.
>
> **Why does logrus degrade under parallelism?** logrus uses a global mutex — at 8 goroutines
> contention raises per-op cost from ~5,570 to ~6,880 ns.

### LogNugget internal benchmarks (V3 — current)

All benchmarks from `go test -tags testing -bench=. -benchmem -count=10 -run=^$ ./...` on Apple M1 Pro, GOMAXPROCS=8.

| Benchmark | ns/op | B/op | allocs/op | Notes |
|-----------|-------|------|-----------|-------|
| `Benchmark_Log` (serial) | **~865** | ~592 | 5 | Full pipeline, no ctx fields |
| `Benchmark_Log_Parallel_NoCtx` | **~196** | ~105 | 2 | `b.RunParallel`, no ctx |
| `Benchmark_Log_Parallel_10CtxFields` (OTel) | **~256** | ~313 | 2 | `b.RunParallel`, 10 OTel fields |
| `Benchmark_Log_Filtered_BelowMinLevel` | **~10** | 0 | 0 | Fast-reject path (atomic gate) |
| `BenchmarkRingBuffer_Push` | **~88** | 0 | 0 | MPSC ring push, 8 producers |
| `BenchmarkAppendAttr_Str` | ~25 | 0 | 0 | Per-field encoding (hot path) |
| `BenchmarkAppendAttr_Int` | ~11 | 0 | 0 | Per-field encoding (hot path) |

**V3 vs V2:** parallel NoCtx −82% (1,090→196 ns/op), allocs −60% (5→2), bytes −93% (1,411→105 B/op).
**Filtered path (~10 ns, 0 allocs)** — atomic level gate; sub-1 ns under parallel.

---

## Real-World API Latency Impact — zerolog vs LogNugget under Loki HTTP

> Full harness at [`examples/loki-bench/`](examples/loki-bench/) — two HTTP servers, k6 load script,
> Loki + Grafana via docker-compose.

When logs are written to a fast sink (in-process buffer, `io.Discard`) zerolog's synchronous model
wins on raw ns/op. Under **real IO latency** — an HTTP POST to Loki, S3, or any remote sink — the
story reverses: the async ring buffer decouples the handler from IO entirely.

### k6 load test (1,000 req/s, 60 s — baseline, local Loki ~1 ms RTT, Apple M1 Pro)

| Server                          | p50        | p90        | p95        | max         | Notes                              |
|---------------------------------|------------|------------|------------|-------------|------------------------------------|
| zerolog (sync → Loki)           | 1.75 ms    | 2.36 ms    | 3.96 ms    | 329 ms      | handler blocks on Loki POST        |
| **LogNugget (async → Loki)**    | **1.14 ms**| **1.21 ms**| **1.25 ms**| **44.9 ms** | ring push only; IO off hot path    |

**LogNugget p95 is 3.2× lower** than zerolog at the same request rate.

### k6 load test (10,000 req/s, 60 s — saturation, OTel B3 propagation, local Loki ~1 ms RTT)

At 10× the load, zerolog's synchronous Loki writes block all handler goroutines. LogNugget's ring buffer keeps handlers non-blocking, sustaining 5.5× more throughput with 0 errors.

| Server                       | Actual RPS   | p50 (http) | p90 (http) | p95 (http)  | Errors | Notes                                    |
|------------------------------|-------------|------------|------------|-------------|--------|------------------------------------------|
| zerolog (sync → Loki)        | 1,094 /s    | 1.33 s     | 3.81 s     | 4.72 s      | 0.62%  | Loki writes saturate; goroutines block   |
| **LogNugget (async → Loki)** | **6,016 /s**| **336 ms** | **391 ms** | **417 ms**  | **0%** | 5.5× throughput; handler ~1 ms CPU only |

> The 336 ms LogNugget http average is client queueing (2,000 VUs waiting), not handler work.
> Actual handler latency = ~1 ms (ring push only). OTel B3 trace/span IDs are logged on every request.

### Go microbenchmark (simulated slow writer — no external deps)

Run: `cd examples/loki-bench && go test -bench=. -benchmem -count=5 -run=^$`

| Benchmark                                           | ns/op           | Notes                          |
|-----------------------------------------------------|-----------------|--------------------------------|
| `BenchmarkHandler_Zerolog_Sync_1msLoki`             | ~2,021,000      | 1 ms CPU + 1 ms IO = 2 ms      |
| `BenchmarkHandler_Zerolog_Sync_5msLoki`             | ~6,021,000      | 1 ms CPU + 5 ms IO = 6 ms      |
| `BenchmarkHandler_LogNugget_Async_1msLoki`          | **~1,002,000**  | 1 ms CPU — IO irrelevant       |
| `BenchmarkHandler_LogNugget_Async_5msLoki`          | **~1,002,000**  | same: IO off hot path          |
| `BenchmarkHandler_Zerolog_Sync_Parallel_1msLoki`    | ~252,000        | 8 goroutines share slow lock   |
| `BenchmarkHandler_LogNugget_Async_Parallel_1msLoki` | **~134,000**    | **1.9× lower per-op cost**     |

**Key insight:** LogNugget's handler latency is independent of the sink's IO RTT. Whether Loki takes
1 ms or 10 ms per push, the caller sees only CPU work (~1 ms doWork) plus the ring-buffer push (~50 ns).

---

## Key Properties

- **Non-blocking** — caller never blocks on IO (channel buffer + async flush).
- **Low GC** — `sync.Pool` recycles `LogEntry` objects; backing `[]byte` capacity preserved.
- **Context-safe** — `LogEvent.Data` is always copied before the pool slot is released.
- **Graceful shutdown** — `Shutdown()` drains all buffered messages before returning.
- **Fan-out hooks** — multiple outputs (stdout, file, remote sink) via `RegisterHook`.
- **RFC 8259 JSON** — all string values escaped per spec; no injection via log fields.

---

## API Stability

LogNugget follows [Semantic Versioning](https://semver.org/):

- **Patch** (v2.0.x): bug fixes, no API changes.
- **Minor** (v2.x.0): backward-compatible additions. Existing callers need no changes.
- **Major** (v3.0.0): breaking changes announced in [CHANGELOG.md](CHANGELOG.md) with migration notes.

The public API surface is: `package lognugget` (Shutdown), `package entry` (NewLogEntry, LogEntry methods), `package config` (all Set* functions, ContextFieldsAppender), `package encoder` (Encoder interface, NewJSONEncoder, NewTextEncoder), `package model` (LogAttr, typed constructors), `package enum` (LogLevel, LogEncodeType, DefaultLogKey constants).

Internal packages (`pipeline_stage`, `custom_time`) are not stable API — callers should not import them directly.

---

## Hooks Support

LogNugget supports hooks for fan-out to external systems:

- Send logs to ELK, Loki, Datadog, or any `io.Writer`.
- Register at `LevelUnSet` (all levels) or a specific level.
- Each hook is a buffered `unsetLogEventPostProcessor` with its own flush rate and capacity.
- Multiple hooks coexist — order of delivery within a level is undefined (map iteration).

---

## Future Work

- Configurable log rotation strategies.
- Structured JSON filtering for high-volume streams.
- Adaptive ring buffer sizing (runtime-tunable capacity beyond fixed 4096 slots).
- OpenTelemetry SDK integration package (auto-extract trace/span without manual appender).

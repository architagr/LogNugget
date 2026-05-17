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

## Context Fields — Threshold Recommendation

**Recommended maximum: 10 values in the context object** (including trace_id and span_id).

The context parser returns `map[string]any`. Iteration cost scales linearly: each extra field adds ~60 ns. At 10 fields, the context-parsing step costs ~600 ns — still within the target budget. Beyond 10, context overhead dominates.

```go
// Optimal: 10 fields including 2 tracing fields
config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
    return map[string]any{
        "trace_id":   extractTraceID(ctx),   // OTel tracing
        "span_id":    extractSpanID(ctx),    // OTel tracing
        "request_id": extractRequestID(ctx),
        "user_id":    extractUserID(ctx),
        // up to 6 more application-specific fields
    }
})
```

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
           │ copies buf, sends to ch (buffer=10)
           ↓
        config.ProcessLogEvent()  [background goroutine]
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
| LogNugget ¹ | ~3,630 | 2,909 | 55 | ~275 K |
| logrus | ~5,570 | 4,857 | 58 | ~179 K |

### Parallel (8 goroutines, GOMAXPROCS=8)

| Logger | ns/op | B/op | allocs/op | ~ops/sec (total) |
|--------|-------|------|-----------|-----------------|
| **zerolog** | **~110** | **0** | **0** | **~9.09 M** |
| LogNugget ¹ | ~3,440 | 2,909 | 55 | ~290 K |
| logrus | ~6,880 | 4,863 | 58 | ~145 K |

### Filtered path (log level below minimum — fast reject)

| Logger | ns/op | B/op | allocs/op | ~ops/sec |
|--------|-------|------|-----------|----------|
| LogNugget | **~35** | **0** | **0** | **~28.6 M** |

> ¹ **LogNugget is async** — the caller returns after a channel put; actual JSON encode and
> `io.Write` happen on a background goroutine. The `ns/op` figures above reflect **caller-side
> cost only** (no IO wait). zerolog and logrus are **synchronous** — their numbers include full
> JSON encoding and write to `io.Discard`.
>
> **Why is zerolog faster?** zerolog uses a zero-alloc fluent API that writes directly to an
> internal byte buffer with no `map[string]any` iteration. LogNugget's dominant cost at 10
> context fields is `map[string]any` iteration in the context parser (~600 ns). This is tracked
> as a post-v1.0.0 optimization (pre-render context as `[]byte` on first call and cache).
>
> **Why does logrus degrade under parallelism?** logrus uses a global mutex for concurrent
> writes. At 8 goroutines on M1 Pro, contention raises per-op cost from ~5,570 ns to ~6,880 ns.

### LogNugget internal benchmarks (M3 Baseline)

All benchmarks from `go test -bench=. -benchmem -count=10 -run=^$ ./...`

| Benchmark | ns/op | B/op | allocs/op | Notes |
|-----------|-------|------|-----------|-------|
| `Benchmark_Log` (serial) | ~2,050 | 1,400 | 18 | Full pipeline, 2 ctx fields |
| `Benchmark_Log_Parallel` | ~1,880 | 986 | 17 | `b.RunParallel`, 2 ctx fields |
| `Benchmark_Log_Parallel_NoCtx` | ~1,840 | 882 | 16 | No context parser |
| `Benchmark_Log_Parallel_10CtxFields` | ~3,200 | 2,690 | 53 | Max recommended ctx size |
| `Benchmark_Log_Filtered_BelowMinLevel` | **36** | 0 | 0 | Fast-reject path (level gate) |
| `Benchmark_Log_JSONEscape_SafeASCII` | ~1,650 | 946 | 16 | Pure ASCII field values |
| `Benchmark_Log_JSONEscape_Unicode` | ~1,730 | 1,010 | 16 | Multibyte Unicode values |
| `Benchmark_Log_JSONEscape_ControlChars` | ~1,600 | 978 | 16 | Tab/newline escape |

**SLO target: < 1,000 ns/op (1 µs) on the hot path.**  
Current hot-path result: ~1,840–2,050 ns/op — 2× over budget. Dominant cost: `map[string]any`
context iteration. Optimization: pre-render context fields as `[]byte` on first call and cache.
Tracked post-v1.0.0.

**Filtered path (35 ns, 0 allocs)** is well within the < 80 ns target.

---

## Key Properties

- **Non-blocking** — caller never blocks on IO (channel buffer + async flush).
- **Low GC** — `sync.Pool` recycles `LogEntry` objects; backing `[]byte` capacity preserved.
- **Context-safe** — `LogEvent.Data` is always copied before the pool slot is released.
- **Graceful shutdown** — `Shutdown()` drains all buffered messages before returning.
- **Fan-out hooks** — multiple outputs (stdout, file, remote sink) via `RegisterHook`.
- **RFC 8259 JSON** — all string values escaped per spec; no injection via log fields.

---

## Hooks Support

LogNugget supports hooks for fan-out to external systems:

- Send logs to ELK, Loki, Datadog, or any `io.Writer`.
- Register at `LevelUnSet` (all levels) or a specific level.
- Each hook is a buffered `unsetLogEventPostProcessor` with its own flush rate and capacity.
- Multiple hooks coexist — order of delivery within a level is undefined (map iteration).

---

## Future Work (post v1.0.0)

- Pre-render context fields as `[]byte` to eliminate per-call `map[string]any` overhead.
- OpenTelemetry integration for automated trace/span extraction.
- Configurable log rotation strategies.
- Structured JSON filtering for high-volume streams.

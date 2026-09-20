# LogNugget

[![codecov](https://codecov.io/gh/architagr/LogNugget/branch/main/graph/badge.svg?token=9VPDuFbSyQ)](https://codecov.io/gh/architagr/LogNugget)

**Bite-sized, context-aware logging for Go** — because every request deserves its own story.

LogNugget is a structured logging library for Go with an asynchronous pipeline: a log call
renders the record and hands it to a lock-free ring buffer, and a background goroutine does the
encoding bookkeeping and the IO. The caller never waits for a write.

```go
package main

import (
    "context"

    "github.com/architagr/lognugget/entry"
    "github.com/architagr/lognugget/lognugget"
)

func main() {
    defer lognugget.Shutdown() // drains both async stages

    entry.NewLogEntry().
        Str("method", "GET").
        Int("status", 200).
        Info(context.Background(), "request handled")
}
```

```json
{"time":"2026-09-20T10:21:37Z","level":"INFO","message":"request handled","method":"GET","status":200}
```

Importing `lognugget` is the whole setup — no `NewLogger()`, no handler wiring.

---

## Why async?

Against a fast sink (`io.Discard`, a file on a warm page cache) a synchronous logger like zerolog
wins on raw ns/op, and this README says so below. The difference shows up when the sink is a
network service.

With logs going to Loki over HTTP at 10,000 req/s ([`examples/loki-bench`](examples/loki-bench)):

| Server | Throughput | p50 | p95 | Errors |
|--------|-----------|-----|-----|--------|
| zerolog (sync → Loki) | 1,094 /s | 1.33 s | 4.72 s | 0.62% |
| **LogNugget (async → Loki)** | **6,016 /s** | **336 ms** | **417 ms** | **0%** |

The handler's cost is the ring-buffer push. Whether Loki answers in 1 ms or 10 ms does not reach it.

---

## Install

```bash
go get github.com/architagr/lognugget
```

Go 1.21+. The library itself depends only on the standard library (testify is a test dependency).

---

## Cookbook

Six runnable programs in [`examples/cookbook`](examples/cookbook), smallest first:

```bash
cd examples/cookbook && go run ./01-quickstart
```

| Example | Answers |
|---------|---------|
| [`01-quickstart`](examples/cookbook/01-quickstart) | What is the least I have to write? |
| [`02-fields`](examples/cookbook/02-fields) | How do I attach data — and what if I have none? |
| [`03-context`](examples/cookbook/03-context) | How do trace IDs and per-request fields get in? |
| [`04-configuration`](examples/cookbook/04-configuration) | What can I configure, and what does each knob change? |
| [`05-hooks`](examples/cookbook/05-hooks) | How do I send records to more than one place? |
| [`06-tuning`](examples/cookbook/06-tuning) | How do I trade latency for fewer writes? |

Larger examples: [`gin-demo`](examples/gin-demo) (HTTP middleware),
[`otel-appender`](examples/otel-appender) (OpenTelemetry trace/span IDs),
[`loki-bench`](examples/loki-bench) (the load test above),
[`bench`](examples/bench) (the cross-logger comparison).

---

## Fields

Typed chain methods write straight into the record buffer — no `map[string]any`, no interface
boxing:

| Method | Signature | Notes |
|--------|-----------|-------|
| `Str` | `Str(key, value string)` | RFC 8259 escaped |
| `Int` | `Int(key string, value int64)` | Signed integer |
| `Uint` | `Uint(key string, value uint64)` | Unsigned integer |
| `Float64` | `Float64(key string, value float64)` | NaN and ±Inf become JSON `null` |
| `Bool` | `Bool(key string, value bool)` | |
| `Err` | `Err(err error)` | Writes `"error"`; no-op when `err` is nil |
| `Any` | `Any(key string, val any)` | Type-switch fallback; boxes the value |

```go
entry.NewLogEntry().
    Str("path", r.URL.Path).
    Int("status", 200).
    Float64("duration_ms", 12.5).
    Err(err).
    Info(ctx, "request handled")
```

Fields assembled elsewhere can be passed as `model.LogAttr` values instead:

```go
entry.NewLogEntry().Info(ctx, "shard selected",
    model.Str("tenant", "acme"),
    model.Int("shard", 7),
)
```

A field whose key collides with a core key (`time`, `level`, `message`, `error`, `caller`) is
written as `custom.<key>` rather than emitted twice.

Levels: `Debug`, `Info`, `Warn` take `(ctx, message, fields...)`; `Error`, `Fatal`, `Panic` take
the error first — `(ctx, err, message, fields...)`.

---

## Context fields

Register one extractor at startup; it runs once per record.

```go
config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
    span := trace.SpanFromContext(ctx)
    if span.SpanContext().IsValid() {
        f.Str("trace_id", span.SpanContext().TraceID().String())
        f.Str("span_id", span.SpanContext().SpanID().String())
    }
    f.Str("service", "checkout-api")
})
```

`CtxFields` has `Str`, `Int`, `Uint`, `Bool` and `Float64`.

Three strategies exist, highest precedence first:

| Setter | Shape | When to use |
|--------|-------|-------------|
| `SetContextFieldsAppender` | `func(ctx, []byte) []byte` | Fixed-shape fields, you own the escaping |
| `SetContextFields` | `func(ctx, *config.CtxFields)` | **Recommended** |
| `SetContextFieldsParser` | `func(ctx) map[string]any` | Legacy; allocates a map per record (~2× the typed API at ten fields) |

---

## Configuration

Every setter is optional and the defaults work unconfigured. These are startup-time settings:
call them in `main` before serving traffic.

| Setter | Default | Description |
|--------|---------|-------------|
| `config.SetMinLevel(level)` | `Info` | Records below this are rejected by an atomic gate |
| `config.SetEncoderType(t)` | `JSON` | `JSON` or `Text` |
| `config.SetAddSource(bool)` | `false` | Adds the call site; `runtime.Callers` is the most expensive option available |
| `config.SetOutput(w)` | `os.Stdout` | Where the built-in collector writes |
| `config.SetTimeFormat(fmt)` | `time.RFC3339` | Timestamp layout |
| `config.SetDefaultFields(map)` | built-in keys | Rename the core keys to match an existing schema |
| `config.SetStaticEnvFieldsParser(fn)` | `nil` | Fields evaluated once, added to every record |
| `config.SetContextFields(fn)` | `nil` | Per-request fields (see above) |
| `config.SetLogBufferMaxSize(n)` | `20` | Flush once `n` records are buffered |
| `config.SetRate(d)` | `1s` | Flush at least every `d` |
| `config.SetSyncMode(bool)` | `false` | Deliver on the calling goroutine instead of via the ring (see below) |

Full walk-through with output: [`examples/cookbook/04-configuration`](examples/cookbook/04-configuration).

---

## Hooks

A hook is any type with `PublishLogMessage([]byte)` and `Name() string`. Register it against a
level, or against `LevelUnSet` to receive every record:

```go
errorSink := pipelineStage.NewUnsetLogEventPostProcessor(
    500*time.Millisecond, // flush interval
    100,                  // or once 100 records are buffered
    myWriter,             // io.Writer
)
defer errorSink.Stop()

pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelError, errorSink)
config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
```

Records fan out to every matching hook, so an error reaches both the `LevelUnSet` hooks and the
`LevelError` ones. Registering a hook whose `Name()` matches an existing one at that level
replaces it. Delivery order between hooks at the same level is undefined.

**The `[]byte` a hook receives is borrowed.** The dispatcher recycles that buffer as soon as the
call returns — copy the bytes if you keep them.

---

## Batching and shutdown

Two knobs decide how long a record waits and how many writes it costs. Whichever fires first
wins:

```go
config.SetLogBufferMaxSize(500)  // flush after 500 records
config.SetRate(5 * time.Second)  // and at least every 5s
entry.GenerateInitialPool(runtime.GOMAXPROCS(0) * 64) // pre-warm the entry pool
```

Each flush is a single `Write` carrying the whole newline-delimited batch, so a larger bucket
means proportionally fewer writes. From [`examples/cookbook/06-tuning`](examples/cookbook/06-tuning),
500 records through one collector:

| Configuration | Writes | Bytes |
|---------------|--------|-------|
| `bucket=1 rate=1s` | 500 | 41,390 |
| `bucket=20 rate=1s` (default) | 25 | 41,390 |
| `bucket=500 rate=5s` | 1 | 41,390 |

**Sync mode.** `config.SetSyncMode(true)` bypasses the ring and delivers each record to the
hooks on the calling goroutine. It removes the ring push (~94 ns under 8 producers) and is
~18% faster serially (~356 vs ~435 ns/op), but ~16% slower at 8 goroutines (~385 vs ~331 ns/op)
because the callers then contend on the collector instead. Use it only with a fast local sink
where single-call latency matters; with a network sink every logging goroutine blocks on that
sink, which is the failure mode async exists to avoid.

`lognugget.Shutdown()` drains both asynchronous stages — the dispatch ring, then the collector's
buffer — and blocks until the last byte is written. Call it before exit: with a 5 s rate, even a
clean exit can otherwise drop five seconds of logs. `config.FlushDispatch(timeout)` drains only
the first stage, for tests and custom shutdown paths.

---

## Architecture

```text
caller → entry.LogEntry.Info(ctx, msg, fields...)
           │ pooled LogEntry, fields appended into an inline 256 B slab
           ↓
        config.PublishLog(level, []byte)
           │ atomicRing.Load().Push()  ← lock-free MPSC ring buffer, 4096 slots
           ↓
        config.ProcessLogEvent()  [single consumer goroutine]
           │ fans out to each registered pre-processor, then recycles the buffer
           ↓
        pipelineStage.EventPreProcessorObj.PreProcess(level, data)
           │ LevelUnSet hooks + level-specific hooks
           ↓
        unsetLogEventPostProcessor.PublishLogMessage(data)
           │ copies the record into a pooled arena
           │ bucket full or ticker fires → one batch to the writer goroutine
           ↓
        io.Writer (os.Stdout or your sink) — one Write per batch
```

Everything from the ring buffer rightwards is off the caller's goroutine.

---

## Performance

Apple M1 Pro, `GOMAXPROCS=8`, Go 1.26. Reproduce with `./scripts/bench-check.sh` (library) and
`cd examples/bench && go test -bench=. -benchmem -count=6 -run=^$` (comparison).

### Cross-logger, 10 context fields

Each benchmark builds a context with ten fields (trace_id, span_id, request_id, user_id,
tenant_id, session_id, env, region, service, version) and logs one Info record with two
call-site attributes. Output is `io.Discard`.

**Parallel (8 goroutines)**

| Logger | ns/op | B/op | allocs/op | Notes |
|--------|-------|------|-----------|-------|
| **zerolog** | **~92** | **0** | **0** | Synchronous, zero-alloc |
| **LogNugget** | **~347** | **~36** | **1** | Asynchronous — caller-side cost only |
| logrus | ~6,155 | ~4,860 | 58 | Synchronous; global mutex serialises under load |

**Serial (1 goroutine)**

| Logger | ns/op | B/op | allocs/op |
|--------|-------|------|-----------|
| **zerolog** | **~545** | **0** | **0** |
| **LogNugget** | **~1,131** | **~37** | **1** |
| logrus | ~5,182 | ~4,854 | 58 |

LogNugget's numbers are caller-side: encoding framing and the write happen on other goroutines,
which is why the parallel figure improves so much more than the serial one. zerolog's and
logrus's numbers include their full write. Under a sink with real latency the ranking inverts —
see [Why async?](#why-async) above.

### Library hot path

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `Benchmark_Log_Parallel_NoCtx` | ~324 | ~33 | 1 |
| `Benchmark_Log_Parallel_10CtxFields_Typed` | ~315 | ~38 | 1 |
| `Benchmark_Log_Serial_Typed` | ~491 | ~41 | 1 |
| `Benchmark_Log_Filtered_BelowMinLevel` (serial) | ~14 | 0 | 0 |
| `Benchmark_Log_Filtered_BelowMinLevel_Parallel` | ~3.6 | 0 | 0 |
| `BenchmarkRingBuffer_Push` | ~94 | 0 | 0 |
| `Benchmark_Log_Parallel_NoCtx_Sync` (sync mode) | ~385 | ~33 | 1 |
| `Benchmark_Log_Serial_Typed_Sync` (sync mode) | ~356 | ~37 | 1 |
| `BenchmarkAppendAttr_Str` | ~23 | 0 | 0 |
| `BenchmarkAppendAttr_Int` | ~11 | 0 | 0 |
| `BenchmarkLogEntry_CtxFields_10` (typed) | ~558 | ~56 | 1 |
| `BenchmarkLogEntry_CtxParser_10` (legacy map) | ~1,144 | ~1,026 | 5 |

The last two rows are the cost of the legacy `map[string]any` context parser against the typed
API, measured the same way.

### The gate

`./scripts/bench-check.sh` fails if any benchmark's mean exceeds 1 µs, or if any benchmark is
more than 25% slower than `bench-baseline.txt` (and at least 50 ns slower in absolute terms, so
noisy micro-benchmarks do not gate). Two benchmarks are exempt from the ceiling and documented in
the script: source capture (`runtime.Callers`, opt-in and off by default) and the deprecated map
context parser.

The 1 µs figure is defined on the reference machine above. ns/op does not travel between
machines: GitHub's 4-core runners measure this hot path 2-3x slower, so CI runs the same script
with a scaled ceiling (2.5 µs) and no baseline comparison — enough to catch an order-of-magnitude
regression, not a claim about absolute speed. The strict run is local, and required before a
release.

---

## Key properties

- **Non-blocking** — the caller returns after a ring-buffer push; encoding and IO happen elsewhere.
- **Low GC** — pooled `LogEntry` objects, pooled dispatch buffers, pooled flush arenas.
- **Ordered** — a single writer goroutine per collector, so records reach the sink in publication order.
- **Graceful shutdown** — `Shutdown()` drains the dispatch ring and then the collector.
- **Fan-out** — multiple hooks per level, each with its own flush policy.
- **RFC 8259 JSON** — every string value escaped per spec; no injection through log fields.

---

## API stability

LogNugget follows [Semantic Versioning](https://semver.org/). Breaking changes are listed in
[CHANGELOG.md](CHANGELOG.md) with migration notes.

Public API: `lognugget` (Shutdown), `entry` (NewLogEntry and its methods), `config` (the `Set*`
functions, `CtxFields`, `FlushDispatch`), `encoder` (the `Encoder` interface and constructors),
`model` (`LogAttr` and typed constructors), `enum` (`LogLevel`, `LogEncodeType`, `DefaultLogKey`).

`pipelineStage` is public because hooks are registered through it; `custom_time` is not stable
API.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Before opening a PR: `go test ./...`,
`golangci-lint run`, `./scripts/bench-check.sh`.

Security reports: see [SECURITY.md](SECURITY.md).

---

## Future work

- Configurable log rotation.
- Sampling for high-volume streams.
- Adaptive ring sizing (the ring is a fixed 4096 slots today).
- An OpenTelemetry integration package, so trace and span IDs need no manual extractor.

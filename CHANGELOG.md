# Changelog

All notable changes to LogNugget will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

Epic V4. Performance work, plus the correctness defects that re-measuring it
uncovered. Not yet released.

### Fixed

- **Log records could be corrupted under concurrent load.** `dispatchEvent`
  returned a record's buffer to `dispatchBufPool` as soon as every
  pre-processor's `PreProcess` call returned, but the built-in collector kept
  the borrowed slice in its bucket until that bucket flushed. A later log call
  then drew the same backing array from the pool and overwrote a record that
  had not been written yet; at eight goroutines the writer saw truncated JSON
  and duplicated lines. The collector now copies each record into a pooled
  arena (V4-C1).
- **`Shutdown` dropped records still in the dispatch ring.** `PublishLog`
  returns once the event is in the MPSC ring; `Shutdown` stopped the output
  hooks without draining it, so a process that logged and exited immediately
  lost its last records. Added [`config.FlushDispatch`](config/flush.go), which
  blocks until the consumer has delivered every published event, and
  `lognugget.Shutdown` now calls it before stopping hooks (V4-C2).
- **Records could reach the writer out of order.** Each flush ran in its own
  goroutine, so two batches could write concurrently. A single writer goroutine
  per collector, fed by a bounded channel, now preserves publication order and
  turns a slow sink into back-pressure rather than unbounded goroutines (V4-C3).
- **`SetOutput`, `SetRate` and `SetLogBufferMaxSize` had no effect.** They wrote
  to fields on `defaultConfig` that nothing read; the collector they describe
  was built by `lognugget.init()` with hardcoded values. They now retune the
  collector at runtime through `config.RegisterDefaultSink` (V4-C5).
- **`Any` emitted invalid JSON for composite values.** The `KindAny` fallback
  appended `fmt "%+v"` bytes raw, so `Any("retry", []string{"1s","2s"})` wrote
  `"retry":[1s 2s]` and the whole record failed to parse. Composite values are
  now written as JSON strings (V4-C6).
- **Typed chain methods ignored reserved-key protection.** `Str("time", …)`
  produced a record with two `"time"` members. Chain methods now prefix a
  colliding key with `custom.`, as the variadic `model.LogAttr` path already
  did.
- **Every record was followed by a blank line.** The encoder terminates each
  record with `"\n"` (ARCH-14) and the collector appended a second one.
- **Static fields were rendered with `", "`** where every other member used
  `","`.
- **Tests panicked on Go 1.26.** `testing.AllocsPerRun` now refuses to run
  inside a parallel test; three zero-alloc guards called it from one.
- **`golangci-lint run` could not start** — `.golangci.yml` was still the v1
  schema. Migrated to v2.
- **`resetConfig` dropped events during an epoch switch**, clearing the old
  pre-processors before the old consumer had finished with them.

### Added

- **`config.SetContextFields`** — typed per-request context fields via
  `*config.CtxFields` (`Str`, `Int`, `Uint`, `Bool`, `Float64`). Replaces raw
  `[]byte` handling for most callers; `SetContextFieldsAppender` remains for
  power users and `SetContextFieldsParser` is the deprecated map-based path.
- **`config.FlushDispatch(timeout)`** — drains the dispatch ring without
  stopping hooks. Useful in tests and custom shutdown paths.
- **`config.RegisterDefaultSink`** — lets a custom collector be retuned by the
  package-level setters.
- **`config.SafeFieldKey`** — the reserved-key check used by the chain methods.
- **`entry.LogEntry.Err` and `.Any`** chain methods (V4-P5).
- **Dispatch buffer pool** (`config.GetDispatchBuf` / `ReturnDispatchBuf`),
  removing a `make()` per call on the hot path (V4-P2).
- **Single-slab `LogEntry`** — `pendingBuf` is backed by an inline 256 B array,
  so a cold pool miss costs one allocation instead of three (V4-P3).
- **[`examples/cookbook`](examples/cookbook)** — six runnable programs:
  quickstart, fields, context, configuration, hooks, tuning.
- **[`examples/loki-bench`](examples/loki-bench)** — two HTTP servers, a k6
  script and a Loki + Grafana compose file, for measuring handler latency
  against a real sink (V4-BENCH).
- **Benchmark isolation harness.** Every benchmark now installs a clean
  configuration and tears it down. Previously `Benchmark_Log` left the legacy
  map context parser installed, and every benchmark that ran after it in the
  same binary silently paid for it — which is how the published "2 allocs,
  ~196 ns/op" hot-path figure was produced.
- **Performance gate in CI** (`.github/workflows/bench.yml`), including a job
  that builds and vets every example module. The gate's regression check is now
  real: it compares per-benchmark means against `bench-baseline.txt` and fails
  past a 20% tolerance. Previously it keyed off `benchstat`'s exit code, which
  is 0 whether or not anything regressed.

### Changed

- **BREAKING — one `Write` per flush batch.** An `io.Writer` registered through
  `SetOutput` now receives the whole newline-delimited batch in a single
  `Write` instead of one call per record. Line-oriented sinks are unaffected; a
  sink that treated each `Write` as exactly one record must split on `"\n"`.
  Hooks registered on the pre-processor stage still receive one call per
  record. This is what makes `SetLogBufferMaxSize` observable: 500 records cost
  500, 25 or 1 writes at bucket sizes 1, 20 and 500.
- **Deprecated `config.RegisterHook` / `config.DeRegisterHook`.** They mutate a
  registry the dispatch path never reads, so a hook registered there has never
  received an event. Use
  `pipelineStage.EventPreProcessorObj.RegisterHook(level, hook)`.
- **Deprecated `config.SetContextFieldsParser`** in favour of
  `SetContextFields` — ~1,108 ns/op versus ~511 ns/op at ten fields.
- CI now tests Go 1.22 through 1.26.

### Performance

Apple M1 Pro, `GOMAXPROCS=8`, Go 1.26, isolated benchmarks:

| Path | ns/op | B/op | allocs/op |
|------|-------|------|-----------|
| Parallel, no context fields | ~329 | ~33 | 1 |
| Parallel, 10 typed context fields | ~317 | ~42 | 1 |
| Serial, typed fields | ~399 | ~40 | 1 |
| Filtered (below min level), serial | ~14 | 0 | 0 |
| Filtered, parallel | ~3.4 | 0 | 0 |

> These are **not** comparable to the V3 figures published in 3.0.0. Those were
> measured with a 1M-entry pool and with leaked global state from earlier
> benchmarks in the same binary. The V4 numbers come from a clean configuration
> and a `GOMAXPROCS × 64` pool. Allocations per record dropped from 2 to 1.

Against other loggers (`examples/bench`, 10 context fields, `io.Discard`):
zerolog ~92 ns/op parallel, LogNugget ~347 ns/op, logrus ~6,155 ns/op. With a
real sink — Loki over HTTP at 10k rps — LogNugget sustains 6,016 rps at p95
417 ms with no errors, against zerolog's 1,094 rps at p95 4.72 s with 0.62%
errors.

---
---

## [3.0.1] - 2026-05-23

### Fixed

- **Data race in `dispatchEvent`** — pre-processor slice was iterated outside the lock while `AddPreProcessors`/`RemovePreProcessor` could mutate the same map concurrently. Fixed by replacing the shared map iteration with an `atomic.Pointer` to an immutable `[]preProcessingObserverContract` snapshot (`atomicProcsSlice`). Writers rebuild the slice inside `configMu.Lock` and store a new pointer; `dispatchEvent` loads the pointer once and iterates its own stable copy. A `drainSignal` struct carries the pre-processor snapshot active at `resetConfig` time so the draining goroutine dispatches with the correct epoch's processors after the new epoch's `atomicProcsSlice` is cleared.

---

## [3.0.0] - 2026-05-23

### Added

- **First-class OTel `ContextFieldsAppender`** (`config.SetContextFieldsAppender`) — register a `func(context.Context, []byte) []byte` that appends pre-rendered trace/span/request fields directly into the log buffer. Zero allocations. Replaces the legacy `map[string]any` parser for all new callers (V3-P7).
- **Lock-free MPSC ring buffer** (`config.mpscRingBuffer`, 4096 slots, cache-line padded) — replaces the Go channel for async event dispatch. Each slot is padded to 64 bytes to eliminate false sharing. Follows the Dmitry Vyukov MPSC pattern: producers fetch-and-add on `tail`; single consumer pops from `head` (V3-P9).
- **`BenchmarkRingBuffer_Push`**, **`BenchmarkPublishLog`**, and OTel parallel benchmarks added to the bench suite.

### Changed

- **Pre-processor gate** — `HasEventPreProcessors()` now reads `hasPreProcessorsAtomic` (`atomic.Bool`) instead of acquiring `configMu.RLock`. Cost drops from ~80 ns to < 1 ns/op (V3-P1).
- **Config snapshot** — `GetHotSnapshot()` now loads `hotSnapshotPtr` via `atomic.Pointer[HotSnapshot]` instead of `configMu.RLock` + struct copy. Cost drops ~120 ns/op; zero contention on the read side (V3-P2).
- **`PublishLog` channel pointer** — the dispatch target is now loaded via `atomic.Pointer[mpscRingBuffer]` (previously `atomicCh atomic.Value`), eliminating the final `configMu.RLock` on the hot path (V3-P3).
- **Timestamp formatting** — `time.AppendFormat` writes RFC 3339 bytes directly into `e.buf` with literal `"` wraps; removes the intermediate `customTime.Format()` string allocation and the second alloc inside `AppendQuotedString` (V3-P4, ~2 allocs, ~40 ns saved).
- **JSON string escaping** — `appendJSONStringStr` operates directly on `string` input without a `[]byte(s)` conversion, eliminating 2 allocs per escaped string value (V3-P5).
- **Level serialisation** — `AppendQuotedLevel` reads from a pre-rendered `[levelCount][]byte` table instead of calling `level.String()` + allocating, saving ~15 ns and 1 alloc per event (V3-P6).
- **Buffer alias severance** — exact-size copy: `newCap = max(len(data), 64)` instead of the former fixed 1024 B cap, reducing B/op by ~1,306 B/call on typical log lines (V3-P8).
- **Open-source readiness** (Epic OS): `CONTRIBUTING.md`, PR/issue templates, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `golangci-lint` config, GitHub Actions CI, godoc audit on all exported symbols, API stability contract.

### Performance

Measured on Apple M1 Pro (`go test -bench=. -benchmem -count=10 -run=^$ ./...`), GOMAXPROCS=8:

| Path | V2.0.0 | V3.0.0 | Delta |
|------|--------|--------|-------|
| Serial (no ctx fields) | ~1,280 ns/op, 6 allocs | ~865 ns/op, 5 allocs | −32% latency, −17% allocs |
| Parallel NoCtx (8-core) | ~1,090 ns/op, 5 allocs | ~196 ns/op, 2 allocs | −82% latency, −60% allocs |
| Parallel 10-ctx OTel (8-core) | ~1,280 ns/op, 7 allocs | ~256 ns/op, 2 allocs | −80% latency, −71% allocs |
| Bytes/op (parallel NoCtx) | ~1,411 B/op | ~105 B/op | −93% |
| Filtered (below min level) | ~10 ns/op, 0 allocs | ~10 ns/op, 0 allocs | unchanged |

> **V3 vs zerolog** (parallel, GOMAXPROCS=8): ~196 ns/op vs ~110 ns/op — gap closed from ~10× (V1) to ~1.8× (V3). zerolog writes synchronously; LogNugget's ~196 ns/op includes amortised ring-buffer dispatch overhead across 8 producers.

### Cross-logger comparison (parallel, 8 goroutines, Apple M1 Pro)

| Logger | ns/op | B/op | allocs/op | Notes |
|--------|-------|------|-----------|-------|
| **zerolog** | **~110** | **0** | **0** | Sync, zero-alloc |
| **LogNugget V3** | **~196** | **~105** | **2** | Async, lock-free ring |
| LogNugget V2 | ~1,090 | 1,411 | 5 | Async, Go channel |
| logrus | ~6,880 | 4,863 | 58 | Sync, global mutex |

---

## [2.0.3] - 2026-05-22

### Fixed

- **gofmt formatting** — 7 test files reformatted to satisfy `gofmt` CI gate (no logic changes).

---

## [2.0.2] - 2026-05-22

### Fixed

- **`.golangci.yml` schema** — removed invalid `version:` field from golangci-lint v1.x config; CI lint gate was failing on schema validation.

---

## [2.0.1] - 2026-05-22

### Fixed

- **Lint warnings + per-package coverage gate** — resolved all `golangci-lint` findings surfaced after Epic OS CI integration; coverage enforced at ≥ 85% per package.

---

## [2.0.0] - 2026-05-22

### Added

- **Typed chain methods** on `LogEntry`: `Str`, `Int`, `Uint`, `Float64`, `Bool` — eliminates interface boxing for the five most common field types (Epic V2 P2).
- **`config.SetChannelCapacity(n)`** — caller-configurable async channel depth (default 1000) (Epic V2 P5).
- **`config.ContextFieldsAppender` type** — zero-alloc context-field API; replaces `map[string]any` returned by `ContextFieldsParser` (Epic V2 P3).
- **`config.HasEventPreProcessors()`** — predicate to check whether any pre-processors are registered before entering the publish path.
- **`HotSnapshot`** — single config snapshot captured once per `LogEntry.Info/Warn/Error/Debug` call, replacing per-field config reads (Epic V2 P1).

### Changed

- **`config.SetContextFieldsParser`** now accepts `config.ContextFieldsAppender` instead of `func(context.Context) map[string]any`. Callers that use the old signature must migrate to the appender API. See migration notes below.
- **Inline encoder framing + ownership transfer** — encoder now writes directly into the entry buffer with no intermediate double-buffer copy (Epic V2 P4).
- **Minimum level gate** is now an atomic `int32` load (`atomic.LoadInt32`) instead of a mutex-protected read; filtered calls cost ~10 ns, 0 allocs (Epic V2 P1).
- **Async channel capacity** increased from 10 → 1000 (configurable via `config.SetChannelCapacity`) (Epic V2 P5).
- Internal package `pipeline_stage` is not part of the stable public API; callers should not import it directly.

### Performance

Measured on Apple M1 Pro (`go test -bench=. -benchmem -count=10 -run=^$ ./...`):

| Path | v1.0.0 | v2.0.0 | Delta |
|------|--------|--------|-------|
| Serial (no ctx fields) | ~3,630 ns/op, 55 allocs | ~1,280 ns/op, 6 allocs | −65% latency, −91% allocs |
| Parallel 8-core (no ctx fields) | ~3,440 ns/op, 55 allocs | ~1,090 ns/op, 5 allocs | −68% latency, −91% allocs |
| Filtered (below min level) | — | ~10 ns/op, 0 allocs | new fast-reject path |
| Per-field `Str` encode | — | ~25 ns/op, 0 allocs | new typed path |
| Per-field `Int` encode | — | ~11 ns/op, 0 allocs | new typed path |

#### Migration notes (v1 → v2)

The only breaking change is `config.SetContextFieldsParser`. Replace:

```go
// v1
config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
    return map[string]any{"trace_id": extractTraceID(ctx)}
})
```

with:

```go
// v2
config.SetContextFieldsParser(func(ctx context.Context, appender config.ContextFieldsAppender) {
    appender.AppendString("trace_id", extractTraceID(ctx))
})
```

All other v1 call sites are source-compatible with v2.

---

## [1.0.0] - 2026-05-21

### Added

- **First stable release** — 40 stories across Epics A–E landed on `main`.
- **`sync.Pool`-backed `LogEntry`** — backing `[]byte` capacity preserved across pool cycles; zero per-call allocation on the hot path (Epic C alloc discipline).
- **RFC 8259 JSON escaping** — all string field values escaped per spec; no log-injection via crafted field values (Epic B behavior gaps).
- **Context-fields parser** (`config.SetContextFieldsParser`) — per-call `map[string]any` propagating trace/span/request IDs from `context.Context` (Epic B).
- **`lognugget.Shutdown()`** — blocks until all buffered events are flushed; safe to `defer` in `main()` or a signal handler (Epic D lifecycle).
- **Zero-config init** — importing `_ "github.com/architagr/lognugget/lognugget"` starts the async pipeline with sensible defaults; no `NewLogger()` required.
- **Static env fields parser** (`config.SetStaticEnvFieldsParser`) — once-evaluated fields (hostname, service name) injected into every log line.
- **`pipeline_stage.Stop()`** — graceful per-stage shutdown separating drain from teardown (Epic D).
- **Full test suite** — table-driven tests + race-detector runs for all public APIs; codecov gating on CI (Epic A test hygiene).
- **Encoder abstraction** — `encoder.Encoder` interface with `NewJSONEncoder` and `NewTextEncoder` implementations (Epic B).
- **`model.LogAttr` + typed constructors** — `StringAttr`, `IntAttr`, `BoolAttr`, `Float64Attr` convenience constructors (Epic B).
- **M3 baseline** — serial ~2,050 ns/op, 18 allocs/op; bench-check gate enforced at < 1 µs on filtered path.
- **`config.SetLogBufferMaxSize`**, **`config.SetRate`**, **`config.SetTimeFormat`**, **`config.SetAddSource`** — full configuration surface.
- **Fan-out hooks** — `pipeline_stage.NewUnsetLogEventPostProcessor` + `RegisterHook` for secondary sinks (Loki, Datadog, file) (Epic B).

---

[Unreleased]: https://github.com/architagr/LogNugget/compare/v3.0.1...HEAD
[3.0.1]: https://github.com/architagr/LogNugget/compare/v3.0.0...v3.0.1
[3.0.0]: https://github.com/architagr/LogNugget/compare/v2.0.3...v3.0.0
[2.0.3]: https://github.com/architagr/LogNugget/compare/v2.0.2...v2.0.3
[2.0.2]: https://github.com/architagr/LogNugget/compare/v2.0.1...v2.0.2
[2.0.1]: https://github.com/architagr/LogNugget/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/architagr/LogNugget/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/architagr/LogNugget/releases/tag/v1.0.0

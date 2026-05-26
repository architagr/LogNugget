# Changelog

All notable changes to LogNugget will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

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

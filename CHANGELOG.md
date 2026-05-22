# Changelog

All notable changes to LogNugget will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

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

[Unreleased]: https://github.com/architagr/LogNugget/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/architagr/LogNugget/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/architagr/LogNugget/releases/tag/v1.0.0

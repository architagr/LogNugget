# LogNugget — Requirements Wiki (v1)

Status: Final for handoff to Delivery Manager.
Owner: Business Analyst.
Sources of truth: `README.md` (intent), `logger.go`, `config/`, `entry/`, `encoder/`, `pipeline_stage/`, `model/`, `enum/`, `custom_time/`, `test/benchmark/` (current code behavior).

---

## 1. Problem Statement

Existing Go logging libraries (stdlib `log`, `log/slog`, `zap`, `zerolog`, `logrus`) force application authors to choose between three flawed defaults:

1. **Synchronous IO on the request path.** Most loggers write to the destination `io.Writer` inside the same goroutine that called `logger.Info`. Under load, slow sinks (stdout pipes in containers, file handles on saturated disks, network log shippers) directly inflate request latency and tail-latency percentiles.
2. **No first-class story for context propagation.** Trace IDs, span IDs, request IDs, and user IDs live in `context.Context`, but most loggers require the caller to pull each value out by hand on every call site. This produces inconsistent log shapes across a codebase and silently drops correlation IDs whenever an engineer forgets to add them.
3. **Per-message allocation pressure.** Building a structured log line typically allocates an entry struct, a fields map, and an output buffer per call. At 50k+ log events/sec/instance this measurably increases GC pause frequency and steals CPU from the application.

LogNugget is a Go logging library (not a service) that fixes all three by:
- making the hot path (`entry.Info/Warn/Error`) construct a log message and hand it off to a buffered channel, with **all IO performed asynchronously by a separate goroutine**;
- treating `context.Context` extraction as a first-class, configurable parser (`SetContextFieldsParser`) that runs on every log call without per-call-site boilerplate;
- recycling `LogEntry` objects through a `sync.Pool` (`entry.GenerateInitialPool`, `entry.NewLogEntry`, `LogEntry.Put`) so steady-state allocations on the hot path approach zero.

---

## 2. Goals

- G1. Provide an idiomatic Go logging API (`Debug/Info/Warn/Error/Fatal/Panic`) that is **non-blocking on the caller's goroutine** with respect to IO.
- G2. Hot-path call (`entry.Info(ctx, msg, fields...)`) returns to the caller in **< 1 ms p99** under realistic field volumes (≤ 16 fields per event). This matches the project-wide SLO declared in `CLAUDE.md`.
- G3. Drive steady-state allocations on the hot path toward zero via `sync.Pool` reuse of `LogEntry` and pre-sized buffers.
- G4. First-class structured context: a single `SetContextFieldsParser` registration extracts trace/span/request/user IDs on every log call without per-call boilerplate.
- G5. Pluggable output via **hooks** (per-level and `LevelUnSet` = all levels) so consumers can fan out to ELK, Loki, Datadog, or arbitrary `io.Writer` sinks.
- G6. Pluggable encoding: **JSON** and **plain text**, with `SetEncoderType` switching at startup.
- G7. Zero-config bootstrap: importing the library and calling `entry.NewLogEntry().Info(...)` works against `os.Stdout` with sensible defaults (Info min level, JSON encoding, RFC822 timestamps in UTC, source capture on, 20-message buffer, 1 s flush rate).
- G8. Configurable rename of the standard log keys (`time`, `level`, `message`, `error`, `caller`, …) via `SetDefaultFields`, so log output can match an organization's existing schema.
- G9. Deterministic graceful shutdown: the caller can request a final flush of all pending batched events before process exit.

## 3. Non-Goals (v1)

The following are explicitly **out of scope for v1**; the README's "Future Enhancements" section is treated as the post-v1 backlog and will not appear in v1 stories:

- N1. **OpenTelemetry integration.** v1 supports OTel only insofar as the user wires `trace_id`/`span_id` themselves inside `SetContextFieldsParser`. No `go.opentelemetry.io/otel` dependency in v1.
- N2. **Log rotation.** Output is whatever `io.Writer` the caller provides. Rotation is the caller's concern (e.g., via `lumberjack`).
- N3. **JSON parsing/filtering of high-volume log streams.** v1 emits logs; it does not consume them.
- N4. **Sampling / rate limiting** of log events. Caller must drop noisy logs at call sites.
- N5. **Dynamic reconfiguration at runtime.** Configuration is set during `init()` / startup. Mutating configuration after the first log is emitted is undefined behavior in v1.
- N6. **Multiple independent logger instances.** The README example shows `lognugget.NewLogger()` returning an instance; the **current code uses a process-wide singleton** (`config.defaultConfig`, `pipelineStage.EventPreProcessorObj`, package-level `entryPool`). v1 ships the singleton design. *Decision: singleton in v1. Rationale: it matches the existing implementation, keeps the < 1 ms hot-path simpler (no per-instance pointer chasing), and is sufficient for the canonical use case (one logger per process). Multi-instance is a v2 concern.*
- N7. **A service / daemon / HTTP API.** LogNugget is a Go library imported as a module dependency. The `cmd`-shaped `logger.go` at the repo root is a demo harness, not a deliverable.

---

## 4. Actors

LogNugget has no human end-users. Its actors are:

| Actor | Type | How they interact |
|---|---|---|
| **Library Consumer** (Go application developer) | Direct, primary | Imports `github.com/architagr/lognugget`, configures it once at startup, calls `entry.NewLogEntry().Info(ctx, msg, fields...)` on hot paths. |
| **Operations / SRE team** | Indirect | Reads the rendered log lines that LogNugget writes to the consumer's configured `io.Writer` (typically stdout in containerized environments). |
| **Observability platform** (ELK, Loki, Datadog, custom) | Indirect, integration | Receives events via registered hooks (`PublishLogMessageHookContract`) when the consumer attaches a forwarder. Or scrapes stdout. |
| **Process supervisor** (k8s, systemd, the host OS) | Indirect | Sends `SIGTERM`; the consumer's shutdown handler must call `Stop()` on the post-processor to drain pending logs. |

---

## 5. User Flows

### 5.1 Zero-config bootstrap

The consumer imports the library, performs no configuration, and starts logging. Result: events are encoded as JSON, batched, and written to `os.Stdout` with default field names, RFC822 UTC timestamps, source capture enabled, min level Info, buffer size 20, flush rate 1 s.

```go
import "github.com/architagr/lognugget/entry"
entry.NewLogEntry().Info(ctx, "ready")
```

*Decision: zero-config must work without the consumer ever touching `config.Init*` or `pipelineStage.RegisterHook`. Rationale: matches G7 and lowers adoption friction. The library's `init()` functions are responsible for wiring a default singleton pipeline that flushes to `os.Stdout`.*

### 5.2 Custom-config bootstrap

The consumer overrides defaults during their own `init()` or `main()`:

```go
config.SetMinLevel(enum.LevelDebug)
config.SetEncoderType(enum.EncoderJSON)
config.SetOutput(os.Stdout)
config.SetLogBufferMaxSize(500)
config.SetRate(2 * time.Second)
config.SetTimeFormat(time.RFC3339)
config.SetAddSource(true)
config.SetStaticEnvFieldsParser(func() map[string]any { return map[string]any{"service": "checkout"} })
config.SetContextFieldsParser(func(ctx context.Context) map[string]any { /* extract trace_id, span_id, request_id */ })
config.SetDefaultFields(map[enum.DefaultLogKey]string{enum.DefaultLogKeyMessage: "msg", enum.DefaultLogKeyTime: "ts"})
entry.GenerateInitialPool(10_000) // optional pool warm-up
```

Setter coverage in v1 must equal the README's published list. The README names `SetLogBuffer`; the code names `SetLogBufferMaxSize`. *Decision: the v1 public API is `SetLogBufferMaxSize`. Rationale: the code is canonical and the more explicit name is clearer. The README will be updated to match.*

### 5.3 Hot-path log emission

The dominant flow. Per call:

1. Consumer calls `entry.NewLogEntry()` → pulls a `*LogEntry` from `sync.Pool` (or allocates if pool empty).
2. Calls `entryObj.Info(ctx, "message", fields...)`.
3. `LogEntry.Log` checks min-level gate; if filtered, returns immediately and `Put`s the entry back.
4. If accepted: builds a flat string of `"key": "value"` pairs (time, level, message, user fields, context-parser fields, error if any, caller if `addSource`), appends configured static fields, runs the configured encoder (JSON wraps with `{}`; text passes through), and pushes the resulting `[]byte` onto a buffered channel via `config.PublishLog`.
5. `LogEntry.Put()` returns the entry to the pool.
6. Caller goroutine returns. **No IO has occurred on this goroutine.**

A separate goroutine drains the channel through the `EventPreProcessors` (which dispatch to per-level and `LevelUnSet` hooks). The default hook is the `unsetLogEventPostProcessor`, which batches `[]byte` entries until either `logBufferMaxSize` is reached or the `rate` ticker fires, then flushes asynchronously to the configured `io.Writer`.

### 5.4 Hook-based forwarding (ELK / Loki / Datadog)

The consumer implements `config.PublishLogMessageHookContract` (`PublishLogMessage(entry []byte)` and `Name() string`) and registers it:

```go
pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelError, datadogHook)
pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, lokiHook) // fires for every level
```

`LevelUnSet` semantics: hooks registered against `LevelUnSet` receive **every** event regardless of level. Hooks registered against a specific level receive only events at that exact level. *Decision: this matches the current `pre_processing_stage.go` behavior (`publish(logMsg, e.hooks[enum.LevelUnSet]); publish(logMsg, e.hooks[level])`). Rationale: the code is canonical; the README's "default collector" maps to the consumer registering an `unsetLogEventPostProcessor` against `LevelUnSet`, which is exactly what `logger.go` demonstrates.*

Hooks run **synchronously inside the dispatcher goroutine**, not the caller goroutine — they do not affect the < 1 ms hot-path budget, but a slow hook will backpressure the channel. *Decision: v1 documents that hook implementations must be fast and non-blocking, or must internally fan out to their own goroutines. Rationale: simplest contract; matches current code; per-hook isolation can come in v2.*

### 5.5 Context-field extraction (trace / span / request IDs)

The consumer registers a single parser via `config.SetContextFieldsParser(fn)`. The parser is invoked on every log call where `ctx != nil`. Returned keys are emitted as top-level fields in the log line. If a key collides with one of the reserved default keys (e.g., `time`, `level`, `message`, `error`, `caller`), it is automatically prefixed with `custom.` to avoid silently overwriting reserved fields. This is enforced by `config.ValidateandParseLogField` and `restrictedFields`.

### 5.6 Graceful shutdown / final flush

The consumer is responsible for calling `Stop()` on the post-processor it registered. `Stop()` closes the stop channel, the watcher goroutine performs one last `flushLogMessages()`, waits for the active bucket to drain, then stops the ticker.

*Decision: in v1, the consumer holds the `*unsetLogEventPostProcessor` reference returned by `NewUnsetLogEventPostProcessor` and calls `Stop()` from their own shutdown path (e.g., on `SIGTERM`). Rationale: this matches the existing API in `logger.go`. A package-level convenience `lognugget.Shutdown()` that stops the default post-processor is in scope as a thin wrapper for the zero-config case.*

---

## 6. Functional Requirements

Notation: **MUST** = v1 blocker, **SHOULD** = v1 target, **COULD** = v1 stretch.

### 6.1 API surface

- F1 (MUST). Public package paths exposed to consumers: `entry` (call-site API), `config` (configuration setters), `enum` (level + key + encoder constants), `model` (`LogAttr`), `pipeline_stage` (hook registration + post-processor constructor).
- F2 (MUST). Log methods on `*entry.LogEntry`: `Debug`, `Info`, `Warn`, `Error(ctx, err, msg, ...fields)`, `Fatal`, `Panic`. `Fatal` calls `runtime.Goexit()`; `Panic` calls `panic(err)`. `Error/Fatal/Panic` accept an `error` argument; `Debug/Info/Warn` do not.
- F3 (MUST). `entry.NewLogEntry()` returns a pooled `*LogEntry`. After the log call completes, the entry is returned to the pool by `LogEntry.Put()` (currently invoked at the end of `Log`).
- F4 (MUST). `entry.GenerateInitialPool(n int)` pre-populates the pool with `n` entries. Useful at startup to avoid first-N allocations.

### 6.2 Levels

- F5 (MUST). Levels defined as bitmask in `enum/level.go`: `LevelUnSet`, `LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError`, `LevelFatal`. `LevelUnSet` is reserved for hook registration meaning "all levels"; it is **not** a level a consumer logs at.
- F6 (MUST). Min-level gate: `LogEntry.Log` returns early if `config.MinLevel() > level`. The early-return path must avoid all unnecessary allocations and must be < 100 ns under benchmark.

### 6.3 Encoders

- F7 (MUST). Two encoders shipped: `enum.EncoderJSON` and `enum.EncoderText`, selected via `config.SetEncoderType` and constructed by `encoder.DefaultEncoderFactory`.
- F8 (MUST). Encoder interface: `Write(string) ([]byte, error)`.
- F9 (MUST). JSON encoder output wraps the rendered key-value string with `{` and `}`. Text encoder passes through.
- F10 (SHOULD). v1 ships full JSON-spec compliance for the JSON encoder (escaping of `"`, `\`, control characters, non-ASCII, numeric values emitted unquoted). *Decision: the current `JSONEncoder.Write` implementation is a string-concatenation shortcut that does not escape values and quotes all values including numerics. v1 must replace it with a spec-compliant encoder. Rationale: downstream observability platforms reject malformed JSON; without this LogNugget cannot meet G5.*
- F11 (COULD). A third encoder (`logfmt` style) for Prometheus-friendly output.

### 6.4 Field shape & default keys

- F12 (MUST). Standard fields emitted on every event: `time`, `level`, `message`. Optional based on conditions: `error` (when an error is passed), `caller` (when `addSource` is true).
- F13 (MUST). The full default-key vocabulary (40+ keys including `trace_id`, `span_id`, `correlation_id`, `request_id`, `user`, `session`, `latency`, `status_code`, etc.) is defined in `enum/default_log_key.go`. `config.SetDefaultFields(map[DefaultLogKey]string)` allows the consumer to rename any of them in the rendered output.
- F14 (MUST). Reserved-key collision protection: if a user-supplied field key (via `LogAttr` or `ContextFieldsParser`) matches the rendered name of any default key, the user key is prefixed with `config.DefaultPrefix` (`custom.`) before emission. This is implemented in `config.ValidateandParseLogField` and `LogEntry.Log`.

### 6.5 Context & static fields

- F15 (MUST). `config.SetStaticEnvFieldsParser(fn)` runs the function **once** at registration time and caches the rendered string as `parsedStaticFields`. The cached string is appended to every log line. *Decision: static fields are evaluated at configuration time, not per call. Rationale: matches current code, removes per-call allocation, and fits the "static" semantics.*
- F16 (MUST). `config.SetContextFieldsParser(fn)` registers a `func(ctx context.Context) map[string]any`. The function is invoked **on every log call** that provides a non-nil context. Returned values are emitted as top-level fields with reserved-key collision protection (F14).

### 6.6 Source capture

- F17 (MUST). When `config.SetAddSource(true)`, `LogEntry.Log` emits a `caller` field containing the function name of the call site. *Decision: v1 uses `runtime.Caller`/`runtime.CallersFrames` to populate `LogEntry.caller`. Rationale: the current code declares the field and the field-emission branch but has a `TODO` and never sets `caller`, leaving the feature non-functional. v1 must close this gap.* Source capture is opt-in-cost: must be skipped entirely when `addSource` is false.

### 6.7 Hooks

- F18 (MUST). `pipelineStage.EventPreProcessorObj.RegisterHook(level, hook)` and `DeRegisterHook(level, name)` are the public surface for adding/removing log destinations. Hooks implement `PublishLogMessage(entry []byte)` and `Name() string`.
- F19 (MUST). A hook registered at `LevelUnSet` receives every event. A hook registered at a specific level receives only events at that level. (Matches `pre_processing_stage.go`.)
- F20 (MUST). The default sink shipped with the library is `pipelineStage.NewUnsetLogEventPostProcessor(rate, maxBufferSize, output)`, which batches and flushes asynchronously. This is the realization of the README's "default collector".

### 6.8 Batching & async pipeline

- F21 (MUST). The hot path emits via `config.PublishLog` onto a buffered channel (`ch`). A single dispatcher goroutine (`config.ProcessLogEvent`) drains the channel and fans out to `EventPreProcessors`.
- F22 (MUST). The default channel buffer is 10 events. *Decision: keep the default at 10. Rationale: matches current code; consumers expecting heavier load configure their own pipeline with `entry.GenerateInitialPool` and a larger post-processor `maxBufferSize`. The dispatch channel is a hand-off, not a buffer.*
- F23 (MUST). Channel send (`ch <- LogEvent{…}`) blocks the caller if the dispatcher is stalled. *Decision: blocking send in v1 (matches current code). Rationale: silent drop is worse than caller-visible backpressure for v1; observability of "logs lost" is a v2 feature.* Documentation must clearly call this out.
- F24 (MUST). The post-processor (`unsetLogEventPostProcessor`) flushes when either: (a) the active bucket reaches `maxBucketSize`, or (b) the configured `rate` ticker fires. Flushing creates a fresh active bucket and writes the swapped bucket asynchronously via a new goroutine.

### 6.9 Memory reuse

- F25 (MUST). `LogEntry` instances are recycled via `entry.entryPool` (`sync.Pool`).
- F26 (MUST). `LogEntry.reset()` clears all per-call state before reuse; this is invoked on every `NewLogEntry()` call.
- F27 (SHOULD). The string-builder used by `config.ParseLogField` is sized via `Grow(100 + len(key))`. v1 may further reduce allocations by replacing per-call `make([]string, ...)` slices in `LogEntry.Log` with pooled buffers.

### 6.10 Time handling

- F28 (MUST). Timestamps are produced in **UTC** by `customTime.TimeNow()`. The default format is `time.RFC822` (current code). *Decision: the v1 default time format is `time.RFC3339` to match the README and to match every observability platform's expected ISO-8601 ingestion format. Rationale: README is canonical for intent, and RFC3339 is industry-standard. Update `config.DefaultTimeFormat`.*
- F29 (MUST). `config.SetTimeFormat(format)` accepts any Go time-layout string and uses it for all subsequent log events.

### 6.11 Lifecycle

- F30 (MUST). `unsetLogEventPostProcessor.Stop()` triggers a final flush, waits for the active bucket to drain, stops the ticker, and exits the watcher goroutine.
- F31 (SHOULD). The library exposes a top-level `lognugget.Shutdown()` convenience that stops the default post-processor created by zero-config bootstrap. *Decision: ship this as a thin wrapper in v1. Rationale: zero-config consumers have no handle to `Stop()` otherwise; without this, log loss on `SIGTERM` is the default for the easiest-to-adopt path.*

---

## 7. Non-Functional Requirements

### 7.1 Performance (the hard SLO)

- NF1 (MUST). **`LogEntry.Log` returns to caller in < 1 ms p99.** This is the project-wide SLO from `CLAUDE.md` and the canonical LogNugget performance budget. The hot path is the call returning to the caller, **not** IO flush completion (flushing is async/batched and excluded from the budget by design).
- NF2 (MUST). A `*_bench_test.go` benchmarks the hot path end-to-end with `b.ReportAllocs()` enabled, per `CLAUDE.md` Performance Gate. `test/benchmark/entry_benchmark_test.go` is the existing harness; its current best is ~1246 ns/op @ ~25 allocs/op, well within budget. v1 must keep that under 1,000,000 ns/op even after the changes called out in F10 (spec-compliant JSON), F17 (source capture), F28 (RFC3339 default).
- NF3 (MUST). Allocations on the hot path target ≤ 30 allocs/op steady-state. New work added in v1 (real source capture, real JSON escaping) must not regress the existing baseline by more than the benchstat-significance threshold; otherwise the code path must be optimized (pooled buffers, escape-precomputation) until it does not.
- NF4 (MUST). The build gate `./.claude/scripts/bench-check.sh` blocks merges that exceed the budget or regress baseline. No exceptions.

### 7.2 Concurrency & safety

- NF5 (MUST). All public setters in `config` are documented as **startup-only**; they are not safe to call concurrently with active logging in v1.
- NF6 (MUST). The dispatcher goroutine and post-processor goroutine are started exactly once via `init()` / `sync.Once` patterns. `pipelineStage.EventPreProcessorObj` is initialized inside a `sync.Once` block.
- NF7 (MUST). The post-processor's active bucket is guarded by a `sync.Mutex` (`unsetLogEventPostProcessor.mu`). Concurrent `PublishLogMessage` calls must remain race-free under `go test -race`.

### 7.3 Backpressure & loss

- NF8 (MUST). Backpressure mode in v1 is **block the caller** (F23). The library does not silently drop. Documentation must declare this so that consumers operating at extreme throughput either size buffers appropriately or pre-filter at call sites.
- NF9 (SHOULD). Future versions may add a `SetOverflowPolicy(BlockOrDrop)` option; not in v1.

### 7.4 Compatibility

- NF10 (MUST). Go 1.21+ (matches `go.mod` and `CLAUDE.md` tech stack).
- NF11 (MUST). Zero required runtime dependencies on observability vendors. The library's `go.mod` keeps third-party deps to a minimum (`gin` and `zerolog` in the current root `logger.go` are the demo's deps, **not** the library's — they will be removed when `logger.go` is moved to `cmd/demo` or `examples/`).

### 7.5 Compliance

- NF12 (MUST). The library does not collect, transmit, or persist any data outside the consumer's configured `io.Writer` and registered hooks. There is no telemetry, no phone-home, no remote configuration.
- NF13 (MUST). LogNugget itself is unopinionated about PII. Consumers are responsible for redaction in their `ContextFieldsParser`, their `LogAttr` values, and their hook implementations.

### 7.6 Operability

- NF14 (MUST). When `Stop()` is called and IO writes fail (e.g., closed stdout), the library must not panic; failed writes are silently dropped by the current `printMessage` which discards the `Write` return values. *Decision: keep the current behavior in v1 but document it. Rationale: any log-failure escalation policy (write to stderr-of-last-resort, etc.) is design work for v2; the v1 consumer has the option to wrap their `io.Writer` in a guarded one.*

---

## 8. Success Criteria

A v1 release is successful when **all** of the following are true:

- SC1. `entry.NewLogEntry().Info(ctx, "msg")` against zero-config defaults produces a valid JSON log line on `os.Stdout` ending with `\n`.
- SC2. Hot-path benchmark `Benchmark_Log` passes `./.claude/scripts/bench-check.sh` (mean ns/op < 1,000,000; no benchstat-significant regression vs `bench-baseline.txt`).
- SC3. Source capture (F17) is implemented and emits the call-site function name when `addSource=true`.
- SC4. JSON output (F10) is RFC 8259 conformant and accepted by `encoding/json.Unmarshal` round-trip on a sample of 1,000 events covering string, int, float, error, nested-context, unicode, and embedded-quote inputs.
- SC5. `Stop()` (F30) drains all queued events on graceful shutdown; an integration test asserts no event loss for ≤ `maxBufferSize * 2` events queued at the moment of `Stop()`.
- SC6. `go test ./...` and `golangci-lint run` are green.
- SC7. Reserved-key collision protection (F14) verified by test: a user field named `time` is rendered as `custom.time`.
- SC8. Hook registration at `LevelUnSet` and at a specific level both work and are exercised by integration tests.

---

## 9. Glossary

| Term | Definition |
|---|---|
| **Event** | One instance of a log call. Carries a level, message, optional error, user fields, context-derived fields, static fields, and metadata (time, caller). |
| **Entry** (`*entry.LogEntry`) | The pooled struct that builds a single event. Pulled from `sync.Pool` per call and returned via `Put()`. |
| **Encoder** | A strategy (`encoder.Encoder`) that turns the rendered key-value string into the final `[]byte` payload. Two implementations in v1: `JSONEncoder`, `TextEncoder`. |
| **Pipeline stage** | A processing step between log emission and output. v1 has one explicit stage type, the **pre-processor** (`eventPreProcessorObserver`), which fans events out to hooks. |
| **Pre-processor** | The dispatcher that consumes the channel, looks up hooks for the event's level (and `LevelUnSet`), and calls each hook's `PublishLogMessage`. |
| **Hook** | An object implementing `PublishLogMessage(entry []byte)` that consumes events. Hooks are how external destinations (ELK, Loki, Datadog) are integrated. |
| **Collector** (README term) | Synonymous with the `unsetLogEventPostProcessor` shipped as the default hook: a buffered, time/size-flushed sink writing to an `io.Writer`. |
| **Default keys** | The 40+ reserved field names in `enum/default_log_key.go` (`time`, `level`, `message`, `trace_id`, …) whose rendered names can be remapped via `SetDefaultFields`. |
| **Static fields** | Fields evaluated **once** at configuration time (`SetStaticEnvFieldsParser`) and appended verbatim to every log line. |
| **Context fields** | Fields evaluated **per call** by the function registered via `SetContextFieldsParser`, using the `context.Context` of the log call. |
| **Reserved-key collision protection** | Automatic prefixing of user-supplied keys with `custom.` when they would shadow a default-key rendered name. |
| **Min level / level gate** | `config.MinLevel()`; events at lower levels short-circuit out of `LogEntry.Log` before any allocation work. |
| **Backpressure** | What happens when the dispatch channel is full: in v1, the caller goroutine blocks on `ch <- event` until space is available. |

---

## 10. Handoff

This Wiki is final input for the **Delivery Manager**. The Delivery Manager will translate it into a PRD; the Architect will then produce LLD + Epics + Stories directly (no HLD step).

Key carry-forwards the Architect must respect:
- The hot-path < 1 ms p99 SLO is non-negotiable and gated by `bench-check.sh`.
- Three concrete behavior gaps in current code that v1 must close: (a) wire `runtime.Caller` into `LogEntry.caller` (F17), (b) replace the JSON encoder with a spec-compliant implementation (F10), (c) change `DefaultTimeFormat` to `time.RFC3339` (F28).
- `logger.go` at the repo root is a Gin-based demo, not the library entry point. v1 work should move it to `examples/` or `cmd/demo/` so the library module surface is clean.
- Singleton design is intentional for v1 (N6).

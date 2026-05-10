# LogNugget — Product Requirements Document (v1)

EM-APPROVED: 2026-05-08 — LLD + test framework + defect register reviewed; coverage complete, SLO arithmetic plausible (30 ns headroom on addSource=on flagged), 3-of-13 D-defects spot-checked against source and confirmed real. Phase 2 unblocked.

Status: Final, ready for Architect hand-off.
Owner: Delivery Manager.
Source Wiki: `/Users/architagarwal/code/LogNugget/docs/wiki/lognugget.md` (final, zero open questions).
Today: 2026-05-07.

---

## 1. Executive Summary & Business Intent

LogNugget is a Go logging library (not a service) that delivers three things existing Go loggers do not give simultaneously: a non-blocking caller-side hot path, first-class `context.Context` extraction of correlation IDs (trace, span, request, user), and near-zero steady-state allocation through `sync.Pool` recycling of log entries.

The business intent of v1 is to publish a tagged `v1.0.0` library that Go service teams can adopt as a drop-in replacement for `slog`/`zap`/`zerolog` when their service is sensitive to tail latency on the request path and to GC pause frequency at high event rates. The acceptance bar is the project-wide < 1 µs p99 hot-path SLO declared in `CLAUDE.md`, enforced by `./scripts/bench-check.sh`.

---

## 2. Goals & Non-Goals

### 2.1 Goals (lifted directly from Wiki §2)

- G1. Idiomatic `Debug/Info/Warn/Error/Fatal/Panic` API that is non-blocking on the caller's goroutine with respect to IO.
- G2. Hot-path call returns to caller in < 1 µs p99 under realistic field volumes (≤ 16 fields per event).
- G3. Drive steady-state hot-path allocations toward zero via `sync.Pool` reuse of `LogEntry` and pre-sized buffers.
- G4. First-class structured context: a single `SetContextFieldsParser` registration extracts trace/span/request/user IDs on every log call without per-call boilerplate.
- G5. Pluggable output via hooks (per-level and `LevelUnSet`) for ELK / Loki / Datadog / arbitrary `io.Writer` sinks.
- G6. Pluggable encoding: JSON and plain text, switched by `SetEncoderType`.
- G7. Zero-config bootstrap: import + `entry.NewLogEntry().Info(...)` works against `os.Stdout` with sensible defaults.
- G8. Configurable rename of standard log keys via `SetDefaultFields`.
- G9. Deterministic graceful shutdown via a final flush of pending batched events.

### 2.2 Non-goals (lifted from Wiki §3, no additions)

- N1. OpenTelemetry SDK integration (consumer wires `trace_id`/`span_id` themselves).
- N2. Log rotation (caller owns the `io.Writer`).
- N3. JSON parsing/filtering of inbound log streams.
- N4. Sampling / rate-limiting of events.
- N5. Dynamic runtime reconfiguration after first emit.
- N6. Multiple independent logger instances — v1 is a process-wide singleton (Wiki Decision §3.N6 and §5.1).
- N7. A service / daemon / HTTP API. The repo-root `logger.go` is a demo, not a deliverable.

---

## 3. Functional Requirements (traced from Wiki §6)

IDs are stable. The Architect may cite `F<n>` directly. Each item carries a MUST / SHOULD / COULD priority lifted verbatim from the Wiki.

### 3.1 API surface

- **F1 (MUST)** — Public packages: `entry`, `config`, `enum`, `model`, `pipeline_stage`.
- **F2 (MUST)** — `*entry.LogEntry` exposes `Debug`, `Info`, `Warn`, `Error(ctx, err, msg, ...fields)`, `Fatal`, `Panic`. `Fatal` calls `runtime.Goexit()`. `Panic` calls `panic(err)`. Only `Error/Fatal/Panic` accept an `error` argument.
- **F3 (MUST)** — `entry.NewLogEntry()` returns a pooled `*LogEntry`; the entry is returned to pool by `LogEntry.Put()` at end of `Log`.
- **F4 (MUST)** — `entry.GenerateInitialPool(n int)` pre-populates the pool with `n` entries.

### 3.2 Levels

- **F5 (MUST)** — Bitmask levels in `enum/level.go`: `LevelUnSet`, `LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError`, `LevelFatal`. `LevelUnSet` is a hook-only meta-level meaning "all levels".
- **F6 (MUST)** — Min-level gate in `LogEntry.Log` short-circuits below `config.MinLevel()`. Filtered path < 100 ns / zero allocs under benchmark.

### 3.3 Encoders

- **F7 (MUST)** — Two encoders: `enum.EncoderJSON`, `enum.EncoderText`, selected via `config.SetEncoderType`, built by `encoder.DefaultEncoderFactory`.
- **F8 (MUST)** — Encoder interface: `Write(string) ([]byte, error)`.
- **F9 (MUST)** — JSON encoder wraps the rendered key-value string with `{` / `}`. Text encoder passes through.
- **F10 (SHOULD, v1 blocker)** — JSON encoder is RFC 8259 compliant: escapes `"`, `\`, control chars, non-ASCII; numerics emitted unquoted. Replaces the current concatenation-shortcut implementation.
- **F11 (COULD)** — `logfmt`-style third encoder. Out of scope for v1 unless capacity allows.

### 3.4 Field shape & default keys

- **F12 (MUST)** — Always-emitted standard fields: `time`, `level`, `message`. Conditional: `error` (when an error is passed), `caller` (when `addSource` true).
- **F13 (MUST)** — The 40+ default-key vocabulary in `enum/default_log_key.go` is renamable via `config.SetDefaultFields(map[DefaultLogKey]string)`.
- **F14 (MUST)** — Reserved-key collision protection: any user-supplied key whose rendered name matches a default-key rendered name is auto-prefixed with `config.DefaultPrefix` (`custom.`).

### 3.5 Context & static fields

- **F15 (MUST)** — `config.SetStaticEnvFieldsParser` evaluates the function once at registration time and caches the rendered string. Cached value is appended to every log line.
- **F16 (MUST)** — `config.SetContextFieldsParser` registers `func(ctx context.Context) map[string]any`, invoked on every log call where `ctx != nil`. F14 collision protection applies.

### 3.6 Source capture

- **F17 (MUST)** — `addSource = true` populates the `caller` field with the call-site function name, using `runtime.Caller` / `runtime.CallersFrames`. Fully skipped when `addSource = false`. Closes the existing `TODO` in `LogEntry.Log`.

### 3.7 Hooks

- **F18 (MUST)** — `pipelineStage.EventPreProcessorObj.RegisterHook(level, hook)` and `DeRegisterHook(level, name)` are the public surface. Hook contract: `PublishLogMessage(entry []byte)` and `Name() string`.
- **F19 (MUST)** — Hook level semantics: `LevelUnSet` receives every event; a specific level receives only that level.
- **F20 (MUST)** — Default sink is `pipelineStage.NewUnsetLogEventPostProcessor(rate, maxBufferSize, output)`.

### 3.8 Batching & async pipeline

- **F21 (MUST)** — Hot path emits via `config.PublishLog` onto a buffered channel; a single dispatcher goroutine `config.ProcessLogEvent` drains it and fans out to `EventPreProcessors`.
- **F22 (MUST)** — Default dispatch channel buffer is 10 events (hand-off, not user buffer).
- **F23 (MUST)** — Channel send blocks the caller if the dispatcher is stalled. Documented backpressure mode.
- **F24 (MUST)** — Post-processor flushes when (a) the active bucket reaches `maxBucketSize`, or (b) the `rate` ticker fires. Flush swaps buckets and writes the swapped bucket asynchronously.

### 3.9 Memory reuse

- **F25 (MUST)** — `LogEntry` recycled via `entry.entryPool` (`sync.Pool`).
- **F26 (MUST)** — `LogEntry.reset()` clears all per-call state on every `NewLogEntry()`.
- **F27 (SHOULD)** — Replace per-call `make([]string, ...)` slices in `LogEntry.Log` with pooled buffers if the bench gate flags regression after F10 / F17 land.

### 3.10 Time handling

- **F28 (MUST)** — Timestamps in UTC via `customTime.TimeNow()`. **Default format is `time.RFC3339`** (Wiki Decision §6.10 — overrides the current `RFC822` constant).
- **F29 (MUST)** — `config.SetTimeFormat(format)` accepts any Go time-layout string.

### 3.11 Lifecycle

- **F30 (MUST)** — `unsetLogEventPostProcessor.Stop()` performs a final flush, drains the active bucket, stops the ticker, and exits the watcher goroutine.
- **F31 (SHOULD)** — Top-level `lognugget.Shutdown()` thin wrapper that stops the default zero-config post-processor.

---

## 4. Non-Functional Requirements

### 4.1 Performance (the hard SLO)

- **NF1 (MUST)** — Hot path returns to caller in < 1 µs p99. The hot path is the caller-returning portion of `Info/Warn/Error` (i.e., everything up to and including the channel send), **not** IO flush. IO is async / batched and excluded by design.
- **NF2 (MUST)** — `*_bench_test.go` covers the hot path end-to-end with `b.ReportAllocs()`. The existing `test/benchmark/entry_benchmark_test.go` is the harness; current best is ~1246 ns/op @ ~25 allocs/op. v1 must bring mean ns/op < 1,000 (1 µs) by the time Epic C pool-reuse lands; F10, F17, F28 must not regress further.

- **NF3 (MUST) — Allocation ceiling.** Hot path target: **≤ 30 allocs/op and ≤ 2,048 bytes/op** at steady state for an event with ≤ 16 user fields plus context-parser fields.
  *Decision: 30 allocs/op and 2 KB/op. Rationale: the existing baseline is ~25 allocs/op, leaving a 5-alloc headroom for spec-compliant JSON escaping (F10) and source capture (F17) before the gate trips. 2 KB/op covers typical event size (16 fields × ~64 bytes rendered + overhead) without wasting headroom — both are checked by `bench-check.sh` via benchstat against `bench-baseline.txt`.*

- **NF4 (MUST) — Throughput target.** **≥ 500,000 events/sec sustained on a single goroutine** producing into the dispatch channel, on commodity x86_64 hardware (Apple M-series or equivalent), with `addSource=false`, JSON encoder, and a no-op hook.
  *Decision: 500k events/sec. Rationale: at 1246 ns/op the theoretical ceiling on a single goroutine is ~800k/sec; 500k leaves margin for the F10 / F17 work and is well above the realistic per-instance application logging ceiling (typical Go services log 5k–50k events/sec/instance).*

- **NF5 (MUST) — Backpressure on buffer overflow.** When the dispatch channel is full, the caller goroutine blocks on `ch <- event` (Wiki F23). The library does not silently drop in v1. Documentation on every public log method must call this out so high-throughput consumers either size buffers (via post-processor `maxBufferSize`) or pre-filter at call sites. A `SetOverflowPolicy(BlockOrDrop)` option is explicitly v2.

- **NF6 (MUST)** — The `bench-check.sh` build gate blocks merges that exceed any of: 1 µs mean (NF1), allocs/op or bytes/op ceiling (NF3), or benchstat-significant regression vs `bench-baseline.txt`. No exceptions; bypass is not permitted (per `CLAUDE.md` Performance Gate).

### 4.2 Concurrency & safety

- **NF7 (MUST)** — All `config` setters are documented startup-only; not safe to call concurrently with active logging in v1.
- **NF8 (MUST)** — Dispatcher and post-processor goroutines start exactly once via `init()` / `sync.Once`. `pipelineStage.EventPreProcessorObj` is `sync.Once`-initialized.
- **NF9 (MUST)** — Post-processor's active bucket guarded by `sync.Mutex`. All concurrent paths must remain race-free under `go test -race`.

### 4.3 Availability

- **NF10 (MUST)** — As an in-process library, availability == process availability. No additional failure domain. `Stop()` must not panic on a closed `io.Writer` — failed writes are silently dropped (Wiki §7.6).

### 4.4 Compatibility

- **NF11 (MUST)** — Go 1.21+. The current `go.mod` declares `go 1.18`; v1 must bump to `1.21` to align with `CLAUDE.md` and to use 1.21 stdlib features (`slog` interop is post-v1, but the toolchain floor is fixed now).
- **NF12 (MUST)** — Zero required runtime dependencies on observability vendors. The library `go.mod` carries only stdlib at runtime. The current `gin` / `zerolog` dependencies belong to the demo `logger.go` and must be moved out of the library module surface (relocated to `examples/` or `cmd/demo/`).

### 4.5 Compliance

- **NF13 (MUST)** — No telemetry, no phone-home, no remote config. Data leaves only via the consumer's `io.Writer` and registered hooks.
- **NF14 (MUST)** — PII is the consumer's concern; the library performs no redaction.

### 4.6 Observability

- **NF15 (SHOULD)** — OpenTelemetry trace/metric instrumentation of the library's own internals (channel depth, flush latency) is **out of scope for v1** (deferred per Wiki N1). Consumers can wrap their hook to count drops/flushes if needed.

---

## 5. Stack & Tooling

- **Language**: Go 1.21+ (NF11). Bump `go.mod`'s `go` directive as part of v1 cleanup.
- **Runtime dependencies (library module)**: stdlib only. Remove `gin` and `zerolog` from the library `go.mod`; they survive in the demo subtree under its own `go.mod` or as a `// +build ignore` example file.
- **Test dependency**: `github.com/stretchr/testify` (already present, fine to keep).
- **Build & test**: `go build ./...`, `go test ./...`, `go test -race ./...` (per `CLAUDE.md`).
- **Lint**: `golangci-lint run` (per `CLAUDE.md`).
- **Bench gate**: `./scripts/bench-check.sh` — canonical, blocks merges on > 1 µs mean (1,000 ns/op), allocs/op > 30, bytes/op > 2,048, or benchstat-significant regression (`CLAUDE.md` Performance Gate, NF6).
- **Existing stack alignment**: the project's broader stack (PostgreSQL, Redis, gRPC, fiber, OpenTelemetry per `CLAUDE.md`) is **not relevant to LogNugget**; LogNugget is a pure Go library with no DB / network surface. The bench gate, branching, and Go standards from `CLAUDE.md` apply unchanged.

---

## 6. Major Components (HLD substitute)

> **Design hand-off note.** The HLD step is skipped for this project; the Architect produces LLD + Epics + Stories directly from this PRD. To compensate, this section enumerates the major packages with one-line ownership only — names, boundaries, and what each package owns. Interface signatures, struct fields, and concurrency primitives are LLD territory and are intentionally excluded.

| Package | Owns |
|---|---|
| `entry` | The pooled `LogEntry` and its level methods (`Debug/Info/Warn/Error/Fatal/Panic`); the call-site API; pool warm-up via `GenerateInitialPool`. The hot path begins and ends here. |
| `config` | All startup-only setters (min level, encoder, output, buffer, rate, time format, source capture, static parser, context parser, default key renames); the dispatch channel and the dispatcher goroutine; reserved-key collision logic (`ValidateandParseLogField`). |
| `pipeline_stage` | The pre-processor (event fan-out to per-level and `LevelUnSet` hooks); the default `unsetLogEventPostProcessor` (batched flush to `io.Writer`, ticker, bucket swap, `Stop()`); hook registration / deregistration. |
| `encoder` | Encoder interface and the two implementations (`JSONEncoder`, `TextEncoder`); the factory selecting by `enum.EncoderType`. F10 spec-compliant JSON lives here. |
| `entry` (pool sub-concern) | Pool reset semantics — covered by `entry`, called out explicitly because F25–F27 are the allocation discipline that keeps NF1/NF3 green. |
| `model` | The user-facing field type (`LogAttr`) used by call sites to attach key/value pairs. |
| `enum` | Levels (bitmask), encoder type, default-key vocabulary. |
| `custom_time` | UTC-clamped `TimeNow()`. Single source of truth for timestamp generation. |
| `examples/` or `cmd/demo/` (new) | Where the existing repo-root `logger.go` (Gin demo) is relocated, so the library module surface contains no demo dependencies (see NF12). |

The package-to-feature trace the Architect can use to scope the LLD:

- F1, F2, F3, F4, F25, F26, F27 → `entry`.
- F5, F6 → `enum` + `entry`.
- F7, F8, F9, F10 → `encoder`.
- F12, F13, F14 → `enum` + `config`.
- F15, F16 → `config`.
- F17 → `entry` (calls `runtime.Caller`).
- F18, F19, F20 → `pipeline_stage`.
- F21, F22, F23, F24 → `config` (channel + dispatcher) + `pipeline_stage` (post-processor).
- F28, F29 → `custom_time` + `config`.
- F30, F31 → `pipeline_stage` + a new top-level `lognugget` package (one file) for `Shutdown()`.

---

## 7. Delivery Plan

### 7.1 Assumptions

- *Decision: team size = 1 Architect + 2 Engineers + 1 QA Lead, working serially per the Phase 2 TDD loop in `.claude/TEAM_WORKFLOW.md`. Rationale: this is a focused library with a single feature branch; parallelism beyond 2 engineers fragments the small package surface and adds merge cost.*
- *Decision: 2-week milestone cadence. Rationale: matches the size of each milestone's scope; library work is bench-gated which adds rework cycles that benefit from a longer cadence than 1 week.*

### 7.2 Milestones (dates calculated from today, 2026-05-07)

| # | Milestone | Window | Exit criteria |
|---|---|---|---|
| **M0** | LLD + Epics + Stories | 2026-05-07 → 2026-05-13 (1 week) | Architect produces LLD, epic breakdown, and stories ≤ 300 LOC each. EM-APPROVED stamp on this PRD. |
| **M1** | Hygiene & module cleanup | 2026-05-14 → 2026-05-27 | `go.mod` bumped to 1.21; `gin`/`zerolog` removed from library module; demo relocated to `examples/`; `bench-baseline.txt` re-captured on the cleaned tree. Closes pre-conditions for F10/F17/F28. |
| **M2** | Behavior gap closure | 2026-05-28 → 2026-06-10 | F17 (real source capture), F28 (RFC3339 default), F10 (RFC 8259 JSON) merged; bench gate green; SC3, SC4 pass. |
| **M3** | Pool & allocation discipline | 2026-06-11 → 2026-06-24 | F27 (pooled string-builder buffers in `LogEntry.Log`) implemented if NF3 ceiling is in jeopardy after M2; `entry.GenerateInitialPool` covered by tests; `LogEntry.reset` exhaustive. |
| **M4** | Lifecycle & convenience | 2026-06-25 → 2026-07-08 | F31 (`lognugget.Shutdown()` thin wrapper) shipped; SC5 graceful-shutdown integration test asserting zero loss for ≤ `maxBufferSize × 2` queued events; final flush on Ctrl-C / `SIGTERM` example. |
| **M5** | Hardening, docs, release | 2026-07-09 → 2026-07-22 | All SC1–SC8 pass on `develop`; README updated to reflect `SetLogBufferMaxSize` (Wiki Decision §5.2); `release/v1.0.0` cut from `develop`; merged into `main`; signed annotated tag `v1.0.0` per `CLAUDE.md` release flow. |

**Target GA**: 2026-07-22 (`v1.0.0` tag).

### 7.3 Dependencies

- M1 must complete before M2: F10's JSON-spec changes need a clean baseline so benchstat regressions are attributable to F10, not to demo dependency churn.
- M3 is conditionally-staffed: only enters if M2 closes with allocs/op ≥ 28 (90% of NF3 ceiling). Otherwise the milestone shrinks to NF3 documentation + tests.
- M5 depends on every preceding SC being green; cannot start the release branch with red gates.

---

## 8. Risk Register

| # | Risk | Likelihood × Impact | Mitigation |
|---|---|---|---|
| **R1** | RFC-8259 JSON encoder (F10) regresses the bench gate (NF1 / NF3) because escaping is per-byte work and inflates allocs/op. | Med × High | Pre-allocate the output buffer with a length hint; precompute escape tables; replace `bytes.Buffer` with a pooled `[]byte` slice; if the gate still trips, F27 (pooled string-builder) is pulled forward into M2. The gate is the source of truth — no waiver. |
| **R2** | Hook back-pressure: a slow user-registered hook (e.g., a synchronous HTTP POST to Datadog) stalls the dispatcher goroutine, the dispatch channel fills, and every caller goroutine blocks (NF5). | Med × High | Document on `RegisterHook` that hooks must be fast or fan out internally to their own goroutines (Wiki §5.4). Provide an example "buffered hook" in `examples/`. SC8 integration test exercises a slow hook and asserts the documented blocking behavior — no surprise drops. |
| **R3** | `sync.Pool` false-sharing or pool churn on hot path: high concurrency causes pool entries to bounce between P-local pools and the central pool, eroding the alloc savings. | Med × Med | Bench harness must include a parallel `b.RunParallel` variant; if the parallel benchmark regresses by > 10% vs serial, switch to a per-P slab pattern. Track via `bench-check.sh` parallel-bench results. |
| **R4** | `runtime.Caller` cost in F17 dominates the hot-path budget (commonly 200–500 ns alone) and pushes NF1/NF3 toward the ceiling. | High × Med | Default `addSource = false` (already the case). Use `runtime.Caller` (single frame) not `runtime.Callers + CallersFrames` (multi-frame iterator) when only the immediate caller is needed. Benchmark `addSource=true` and `addSource=false` separately; the gate applies to both. |
| **R5** | Ticker drift on idle loggers: if no events arrive between ticks, the ticker still fires every `rate` interval and a goroutine still wakes up — wasted CPU on idle services and possible empty-flush IO. | Low × Med | Bucket-flush short-circuits when the active bucket is empty (current `flushLogMessages` skips when len == 0). Keep this short-circuit covered by a unit test so a future refactor doesn't accidentally restore the wakeup cost. |

---

## 9. Success Metrics (acceptance criteria, lifted from Wiki §8)

A v1 release is complete and shippable when **every** criterion below is measurably true on `develop` and on the `release/v1.0.0` branch:

- **SC1** — `entry.NewLogEntry().Info(ctx, "msg")` against zero-config defaults produces a valid JSON log line on `os.Stdout` ending with `\n`. Verified by integration test.
- **SC2** — `Benchmark_Log` passes `./scripts/bench-check.sh`: mean ns/op < 1,000 = 1 µs (NF1), allocs/op ≤ 30 and bytes/op ≤ 2,048 (NF3), no benchstat-significant regression vs `bench-baseline.txt`.
- **SC3** — F17 source capture functional: with `addSource=true`, every log line carries `caller` set to the call-site function name; `TODO` removed from `LogEntry.Log`.
- **SC4** — F10 JSON encoder is RFC 8259 conformant: `encoding/json.Unmarshal` round-trips a 1,000-event corpus covering string, int, float, error, nested context, unicode, and embedded-quote inputs without error.
- **SC5** — F30 `Stop()` drains all queued events on shutdown; integration test asserts zero loss for ≤ `maxBufferSize × 2` events queued at the moment of `Stop()`.
- **SC6** — `go test ./...`, `go test -race ./...`, and `golangci-lint run` are green on the release branch.
- **SC7** — F14 reserved-key collision: a user-supplied field named `time` is rendered as `custom.time`. Unit-tested.
- **SC8** — Hook registration at `LevelUnSet` and at a specific level both work and are exercised by integration tests covering: (a) `LevelUnSet` hook receives every level; (b) a level-specific hook receives only that level; (c) `DeRegisterHook` removes a hook cleanly.

Success metrics that are explicitly **not** v1 acceptance criteria (deferred per non-goals):

- OTel trace integration (N1).
- Sampling / drop-on-overflow telemetry (N4 + NF15 deferral).
- Multi-instance benchmark (N6).

---

## 10. Rollout / Release Plan

- **Distribution**: Go module published as `github.com/architagr/lognugget`. Consumers pin via `go get`. There is no service deployment, no staged rollout, no feature-flag system. The Architect should not design feature flags.
- **Versioning**: SemVer. `v1.0.0` is the GA target tagged at the end of M5 (2026-07-22). All v1 changes that follow will be `v1.x.y` with strict semver discipline.
- **Branch & tag flow**: per `CLAUDE.md` "Git Branching & Release Conventions":
  - All v1 work flows through `feat/<n>-<slug>` feature branches off `develop`.
  - Engineers cut sub-branches `feat/<n>-<slug>/<task-slug>` per story (≤ 300 LOC).
  - Release branch is `release/v1.0.0` cut from `develop`; merged to `main`; tagged `v1.0.0` (signed annotated tag); back-merged to `develop`.
  - Hotfixes follow `hotfix/v1.0.<patch>` per `CLAUDE.md`.
- **No staged rollout** — there are no users to roll forward incrementally; consumers update at their own cadence by bumping their `go.mod`.
- **Pre-release**: a `v1.0.0-rc.1` tag may be cut at the start of M5 if the Project Lead wants external feedback before GA. This is optional and at Project Lead's discretion; not a v1 blocker.

---

## 11. Design Hand-off / Downstream Artifacts

**HLD step is skipped for this project; the Architect produces LLD + Epics + Stories directly from this PRD.**

The Architect's hand-off, per `.claude/TEAM_WORKFLOW.md` Phase 1 (HLD step omitted by user direction):

1. **LLD** at `docs/lld/lognugget.md` using the `go-lld-designer` skill. Consumes §6 (Major Components) of this PRD as the equivalent of an HLD's "major components" list — package layout, interfaces, request lifecycle, pool placement, concurrency strategy expand from there.
2. **Epics** at `docs/epics/lognugget/` using `superpowers:writing-plans`. Suggested epic decomposition (the Architect may resequence):
   - Epic A — Module hygiene (M1 scope: go.mod bump, demo relocation, baseline recapture).
   - Epic B — Behavior gap closure (M2 scope: F17, F28, F10).
   - Epic C — Allocation discipline (M3 scope: F27, pool tests).
   - Epic D — Lifecycle & convenience (M4 scope: F31, SC5 integration test).
   - Epic E — Release hardening (M5 scope: README sync, examples polish, release branch).
3. **Stories** at `docs/stories/lognugget/<epic>/`, each ≤ 300 LOC, using the `task-slicer` skill. Every story must reference one or more `F<n>` IDs from §3 of this PRD so traceability is preserved.
4. **EM gate**: Engineering Manager reviews the Architect's LLD + Epics + Stories + this PRD as a bundle. EM either stamps `EM-APPROVED: <date>` at the top of this PRD or routes back to the Architect with specific issues.

No story enters Phase 2 (implementation) until EM has approved.

---

## 12. Confirmation of Closure

This PRD contains zero open questions, zero TBDs, and zero unresolved decisions. Every choice the Wiki delegated to delivery is recorded inline as a `Decision: X. Rationale: Y.` block in §4 (NF3, NF4), §7.1 (team size, cadence), and §7.2 (milestone dates). The Architect has everything needed to begin LLD work.

# Epic V2 — Performance: Close the 31× Throughput Gap vs zerolog

Status: PLANNED (post-v1.0.0)
Owner: Business Analyst
Umbrella issue: [#81](https://github.com/architagr/LogNugget/issues/81)
Sources of truth: `docs/STATUS.md` lines 201–243, GitHub issues #81–#86, `entry/entry.go`, `config/config.go`, `model/attr.go`

---

## 1. Problem Statement

LogNugget v1.0.0 ships with a 31× throughput deficit against zerolog on the parallel hot path:

| Metric | zerolog (GOMAXPROCS=8) | LogNugget (GOMAXPROCS=8) | Gap |
|--------|------------------------|--------------------------|-----|
| ns/op | ~110 | ~3,440 | 31× slower |
| ops/sec | ~9.09 M | ~290 K | 31× fewer |
| allocs/op | 0 | 55 | unbounded |
| B/op | 0 | 2,909 | unbounded |

The project-wide SLO (declared in `CLAUDE.md`) requires every cache/log call path to complete in **< 1 µs p99**. At ~3,440 ns/op the parallel log path is **3.4× over the SLO**. The SLO miss was explicitly accepted as a post-v1.0.0 item (see `docs/STATUS.md` line 268).

The gap is **caller-side cost only**. It excludes IO flush time, which is handled asynchronously by LogNugget's background pipeline. This means LogNugget retains a structural advantage over synchronous loggers (zerolog, logrus) under real IO load — the gap narrows as IO latency grows. Addressing caller-side cost is the scope of this epic.

Root causes, ranked by estimated savings:

| # | Root cause | Code location | Estimated savings |
|---|-----------|---------------|-------------------|
| RC-1 | `map[string]any` context iteration + `[]string` intermediate slice in `setLogContextFields` | `entry/entry.go:249–258` | ~600 ns, ~23 allocs |
| RC-2 | `interface{}` boxing in `model.LogAttr.Value` dispatched via type-switch in `AppendField` | `model/attr.go`, `entry/entry.go:169–178` | ~30 allocs |
| RC-3 | 6× `configMu.RLock` acquisitions per log call (`GetConfig()` called at lines 117, 129, 142, 143, 144, 250) | `entry/entry.go:117–258` | ~150–250 ns |
| RC-4 | Double-buffer: `en.Append(nil, e.buf)` allocates a new slice to frame the body, then `append([]byte(nil), Data...)` in `PublishLog` makes a second defensive copy | `entry/entry.go:208`, `config/config.go:204` | 2 allocs/event |
| RC-5 | Channel cap=10 blocks all goroutines under 8-goroutine parallel load; callers spend the majority of wall time waiting on `ch <-` rather than encoding | `config/config.go:456` | unblocks caller |

---

## 2. Goals

- G1. Achieve `BenchmarkLognugget_Parallel_10CtxFields` >= 3 M ops/sec (from ~290 K) — a 10× improvement toward zerolog parity.
- G2. Bring the parallel hot path within the < 1 µs p99 SLO defined in `CLAUDE.md`.
- G3. Drive allocs/op to single digits on the parallel 10-field path.
- G4. Preserve full backward compatibility: existing `model.LogAttr` struct-literal call sites (`{Key: "k", Value: v}`) must continue to compile and produce correct output without change.
- G5. Preserve full backward compatibility: the existing `SetContextFieldsParser(func(context.Context) map[string]any)` signature and `ContextFieldsParser` contract must continue to work unchanged.
- G6. The filtered path (level gate rejects call) must stay at its current best-in-class 35 ns/op, 0 allocs — no regression permitted.
- G7. All changes must pass `-race`, `go test ./...`, `golangci-lint run`, and `./scripts/bench-check.sh`.

## 3. Non-Goals

- N1. Changes to the async pipeline architecture (background goroutine, `LogEvent` channel, flush loop). The pipeline design is out of scope for this epic.
- N2. New encoder formats (CBOR, Protobuf, plain-text variants). Only the existing JSON and text encoders are in scope.
- N3. New log levels or changes to the `enum.LogLevel` taxonomy.
- N4. Lock-free ring buffer (zerolog/diode pattern) as a channel replacement — listed as a future Option C in issue #86; deferred beyond this epic.
- N5. OpenTelemetry SDK integration.
- N6. Changes to the `Fatal`/`Panic` semantics (`runtime.Goexit` / `panic`).
- N7. Raising or bypassing the `< 1 µs` bench-check gate threshold. The gate threshold is immutable without a Project Lead baseline update.

---

## 4. Actors

| Actor | Description |
|-------|-------------|
| Library caller (hot path) | Any Go goroutine invoking `entry.NewLogEntry().Info(ctx, msg, fields...)` or equivalent on a throughput-sensitive code path (HTTP handlers, gRPC interceptors, event loops). This actor pays the full caller-side cost measured by the benchmarks. |
| Library caller (filtered path) | A goroutine that calls a log method at a level below `MinLevel`. The level gate must reject the call before any heap allocation; this path is already best-in-class and must not regress. |
| Parallel callers | 8+ goroutines sharing a single logger instance under GOMAXPROCS=8. The channel cap=10 currently causes these callers to block on `ch <-`, dominating wall time. |
| Ops / SRE | Teams monitoring throughput and tail latency of services using LogNugget. Their SLO concern is p99 latency of the calling service, not of the log sink. |
| Library maintainer | Engineers updating LogNugget. They run `./scripts/bench-check.sh` as a build gate before marking any PR ready for review. |

---

## 5. Primary User Flows

### Flow A — Non-filtered hot path (message is logged)

1. Caller acquires a `LogEntry` from `sync.Pool` via `entry.NewLogEntry()`.
2. Caller invokes `entry.Info(ctx, "request processed", fields...)`.
3. Library reads `atomicMinLevel` (after P1) — no mutex; level gate passes.
4. Library takes a single config snapshot (after P1) — one `configMu.RLock` instead of six.
5. Library appends time, level, message fields into `e.buf` using pre-rendered key prefixes.
6. Library appends per-call typed fields directly to `e.buf` via `Str`/`Int`/`Bool`/`Float64` methods (after P2) — no `interface{}` boxing.
7. Library appends context fields directly to `e.buf` via a new append-to-buf context API (after P3) — no `map[string]any`, no `[]string` intermediate.
8. Library writes `{`, `}`, and `\n` framing directly into `e.buf` (after P4) — no secondary allocation.
9. `PublishLog` sends a single copy of `e.buf` to the channel (after P4).
10. Channel has capacity >= 1000 (after P5); send is non-blocking under 8-goroutine load.
11. Library returns `LogEntry` to pool via `e.Put()`.
12. Caller's goroutine is unblocked; IO occurs asynchronously in the background pipeline.

### Flow B — Filtered path (level gate rejects call)

1. Caller acquires `LogEntry` from pool via `entry.NewLogEntry()`.
2. Caller invokes `entry.Debug(ctx, "verbose detail", fields...)` when `MinLevel` is `Info`.
3. Library reads `atomicMinLevel` — single atomic load, no mutex, no allocation.
4. Level gate fires; library calls `e.Put()` and returns immediately.
5. No encoding, no channel send, no context extraction. Total cost: ~35 ns, 0 allocs (existing, must not regress).

### Flow C — Parallel callers (8 goroutines, GOMAXPROCS=8)

1. 8 goroutines concurrently invoke `entry.Info(...)` with 10 context fields each.
2. After P1–P4, each goroutine completes encoding in < 500 ns (target).
3. After P5, channel capacity >= 1000; all 8 goroutines send without blocking.
4. Background pipeline drains the channel asynchronously.
5. Aggregate throughput target: >= 3 M ops/sec (from ~290 K).

---

## 6. Functional Requirements

### MUST (blocking for Epic V2 completion)

| ID | Requirement | Story |
|----|-------------|-------|
| F1 | `MinLevel` check on the hot path uses an `atomic.Int32` load; no mutex is acquired for the level gate. | P1 / #82 |
| F2 | Each log call acquires `configMu.RLock` at most once per call (single config snapshot); the current 6-acquisition pattern is eliminated. | P1 / #82 |
| F3 | `LogEntry` exposes typed field methods `Str(key, val string)`, `Int(key string, val int64)`, `Bool(key string, val bool)`, `Float64(key string, val float64)` that append directly to `e.buf` without `interface{}` boxing. | P2 / #83 |
| F4 | Existing `model.LogAttr` struct literal call sites (`[]model.LogAttr{{Key: "k", Value: v}}`) continue to compile and produce identical log output without any source change. | P2 / #83 |
| F5 | Context field extraction no longer allocates a `map[string]any` or `[]string` per call on the hot path; context fields are appended directly to `e.buf`. | P3 / #84 |
| F6 | The existing `SetContextFieldsParser(func(context.Context) map[string]any)` public API remains callable and continues to work; the library adapts its output internally. | P3 / #84 |
| F7 | Log event framing (`{`, `}`, `\n`) is written directly into `e.buf`; `en.Append(nil, e.buf)` allocation is eliminated. | P4 / #85 |
| F8 | `PublishLog` sends a single copy of the buffer to the channel; the second `append([]byte(nil), Data...)` defensive copy is eliminated. | P4 / #85 |
| F9 | Default async channel capacity is increased from 10 to >= 1000. | P5 / #86 |
| F10 | Channel capacity is configurable by the caller via `config.SetChannelCapacity(n int)` with a maximum of 100,000 to prevent OOM. | P5 / #86 |
| F11 | All five stories ship with a `*_bench_test.go` covering the changed hot path with `b.ReportAllocs()` enabled, per the project Performance Gate rules. | All stories |
| F12 | `./scripts/bench-check.sh` exits 0 after all stories are merged (no benchmark mean > 1,000 ns/op, no statistically significant regression vs baseline). | All stories |

### SHOULD

| ID | Requirement | Story |
|----|-------------|-------|
| F13 | The typed field API (`Str`/`Int`/`Bool`/`Float64`) is documented in godoc with usage examples. | P2 / #83 |
| F14 | `SetChannelCapacity` validates its argument and returns or logs a warning for values <= 0 or > 100,000 rather than panicking. | P5 / #86 |
| F15 | A migration guide or code comment explains how to adopt the typed field API from `model.LogAttr` struct literals. | P2 / #83 |

### COULD

| ID | Requirement | Notes |
|----|-------------|-------|
| F16 | Expose `Float32` and `Uint64` typed field methods for completeness. | Deferred if they do not affect the benchmark target. |
| F17 | Provide a `config.SetChannelCapacity` option in the `ConfigBuilder` fluent API so capacity can be set at init time alongside other options. | Convenience; does not change the core fix. |

---

## 7. Non-Functional Requirements

### Latency

- The parallel hot path (`BenchmarkLognugget_Parallel_10CtxFields`) must achieve a mean ns/op <= 1,000 (1 µs), satisfying the project-wide SLO defined in `CLAUDE.md`.
- The filtered path (`BenchmarkLognugget_Filtered`) must remain <= 35 ns/op, 0 allocs.
- These are enforced as a build gate by `./scripts/bench-check.sh` on every PR. No merge is permitted over a red gate.

### Throughput

- `BenchmarkLognugget_Parallel_10CtxFields` must reach >= 3 M ops/sec at GOMAXPROCS=8 (from ~290 K). This is the Definition of Done for issue #81.

### Allocations

- Parallel 10-field hot path: allocs/op must reach single digits (target <= 5) after all five stories are complete, down from 55.
- Filtered path: 0 allocs/op, no regression.

### Scale

- The library is embedded in caller processes; it does not run as a standalone service. Scale is defined by the number of concurrent goroutines sharing a single `LogEntry` pool and channel. The channel capacity fix (P5) is designed to handle at least 1,000 in-flight events without blocking callers.

### Backward Compatibility

- No breaking changes to any exported symbol in `entry/`, `config/`, or `model/` packages. Existing callers using `model.LogAttr{Key: ..., Value: ...}` and `SetContextFieldsParser(func(context.Context) map[string]any)` must compile and run without modification.

### Race Safety

- All changes must pass `go test -race ./...`. The atomic `minLevel` and single-snapshot config pattern must not introduce new data races.

### Compliance

- No new external dependencies may be introduced. The library must remain importable as a standalone Go module.

---

## 8. Stories

| Priority | Issue | Title | Targets |
|----------|-------|-------|---------|
| P1 | [#82](https://github.com/architagr/LogNugget/issues/82) | Atomic minLevel + single config snapshot per call | RC-3: 6× RLock → 0 for gate, 1 for body |
| P2 | [#83](https://github.com/architagr/LogNugget/issues/83) | Typed field API — Str/Int/Bool/Float64 on LogEntry | RC-2: interface{} boxing eliminated |
| P3 | [#84](https://github.com/architagr/LogNugget/issues/84) | Append-to-buf context API — eliminate map[string]any | RC-1: largest single saving (~600 ns, ~23 allocs) |
| P4 | [#85](https://github.com/architagr/LogNugget/issues/85) | Inline framing — eliminate en.Append double-buffer | RC-4: 2 allocs/event → 0 |
| P5 | [#86](https://github.com/architagr/LogNugget/issues/86) | Channel capacity >= 1000 + configurable | RC-5: blocking callers → non-blocking |

Stories are independent; they may be implemented in parallel on sub-branches of a shared `feat/81-v2-performance` feature branch, with each PR targeting that feature branch.

---

## 9. Definition of Done (Epic)

All of the following must be true before the umbrella issue #81 is closed:

- `BenchmarkLognugget_Parallel_10CtxFields` >= 3 M ops/sec (ns/op <= ~333).
- `BenchmarkLognugget_Parallel_10CtxFields` allocs/op <= 5.
- `BenchmarkLognugget_Filtered` <= 35 ns/op, 0 allocs (no regression).
- `go test ./...` green with `-race`.
- `golangci-lint run` green.
- `./scripts/bench-check.sh` green (mean ns/op <= 1,000, no statistically significant regression vs baseline).
- All five story PRs (#82–#86) merged into the feature branch and then into `develop`.
- Existing call sites using `model.LogAttr` struct literals and `SetContextFieldsParser` compile and pass tests without modification.

---

## 10. Open Questions

| ID | Question | Owner | Status |
|----|----------|-------|--------|
| Q1 | Should the typed field API (`Str`/`Int`/`Bool`/`Float64`) be added directly to `LogEntry` as methods (zerolog style), or as a standalone `Fields` builder that is passed to the existing variadic `fields ...model.LogAttr` parameter? The former is a more invasive API addition; the latter preserves the existing call-site shape more closely. | Project Lead | Open |
| Q2 | P3 (append-to-buf context API) requires the context parser to write into a caller-supplied `[]byte` rather than return `map[string]any`. Should the new internal interface be opt-in (a second registration function like `SetContextFieldsAppender`) while keeping the existing `SetContextFieldsParser` as a deprecated but functional path, or should the library adapt the existing `map[string]any` return internally? | Project Lead | Open |
| Q3 | P4 (inline framing) requires that `e.buf` begins with `{` and ends with `}` before `PublishLog` is called. This couples the framing assumption to the JSON encoder. How should plain-text encoder framing be handled — should `Encoder.Append` become a no-op that returns its input, or should each encoder expose a `Frame(buf []byte) []byte` method called inline? | Encoder owner | Open |
| Q4 | For P5, should `SetChannelCapacity` take effect immediately (draining and replacing the current channel) or only on the next `resetConfig` call? Immediate replacement requires a careful channel drain to avoid dropping in-flight events. | Project Lead | Open |
| Q5 | The current `bench-baseline.txt` was established at v1.0.0 with ~3,440 ns/op. After P1–P5 land and the baseline is expected to drop to ~333 ns/op, the Project Lead must run `./scripts/bench-check.sh --update-baseline` and commit the new baseline. Who owns that baseline update commit and on which PR should it land? | Project Lead | Open |

---

## 11. Stated Assumptions

- A1. The parallel benchmark (`GOMAXPROCS=8, 10 ctx fields`) is the authoritative metric for Epic V2. Single-goroutine serial benchmarks are tracked but do not gate the epic's Definition of Done.
- A2. The 3 M ops/sec target is an intermediate milestone toward zerolog parity (~9 M ops/sec), not the final ceiling. Further optimisation (lock-free ring buffer, per-goroutine encoder pools) is deferred to a future epic.
- A3. The `model.LogAttr` struct type will not be removed or made internal in v2. Backward compatibility with struct literal syntax is a hard constraint.
- A4. The existing `ContextFieldsParser` function type (`func(context.Context) map[string]any`) is part of the public API and cannot be changed without a major version bump. Any P3 solution must preserve it.
- A5. `./scripts/bench-check.sh` remains the single source of truth for performance gates. No story may weaken or bypass the gate's 1,000 ns/op threshold or the benchstat regression check.
- A6. All five stories target the `develop` branch via a shared `feat/81-v2-performance` feature branch, consistent with the GitFlow model described in `CLAUDE.md`.

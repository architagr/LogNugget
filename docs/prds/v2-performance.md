# PRD: Epic V2 — Performance: Close the 31× Throughput Gap vs zerolog

> EM-APPROVED: 2026-05-21 — Full traceability from five root causes to LLD-level signatures; race-safety analysis complete; backward-compatibility constraints enforced by design; bench-gate contract is clear and immutable. Six minor gaps noted in docs/lld/EM_REVIEW_V2.md; two (GAP-1 atomic type width, GAP-2 RLock count invariant) must be resolved by the architect before P1 PR is marked ready for review.

| Field | Value |
|---|---|
| Feature slug | `v2-performance` |
| Umbrella issue | [#81](https://github.com/architagr/LogNugget/issues/81) |
| Status | READY FOR ARCHITECT |
| Author | Delivery Manager |
| Date | 2026-05-21 |
| Target branch | `feat/81-v2-performance` → `develop` |

---

## 1. Summary

LogNugget v1.0.0 ships at ~3,440 ns/op and 55 allocs/op on the parallel 10-field hot path, placing it 31× slower than zerolog (~110 ns/op, 0 allocs) and 3.4× over the project-wide < 1 µs p99 SLO declared in `CLAUDE.md`. The SLO miss was explicitly accepted as a post-v1.0.0 work item.

Epic V2 eliminates five discrete caller-side inefficiencies — all traceable to specific lines in the existing codebase — without touching the async pipeline architecture, adding external dependencies, or breaking any public API. The target outcome is >= 3 M ops/sec on `BenchmarkLognugget_Parallel_10CtxFields` (from ~290 K) with allocs/op <= 5 (from 55), bringing the parallel hot path within the < 1 µs gate.

The filtered path (level gate rejects call, currently 35 ns/op, 0 allocs) must not regress; it is already best-in-class and beats zerolog on this dimension.

---

## 2. Scope

### In scope

| Story | Issue | Root cause targeted |
|---|---|---|
| P1 — Atomic minLevel + single config snapshot | #82 | RC-3: 6× `configMu.RLock` per call |
| P2 — Typed field API (`Str`/`Int`/`Bool`/`Float64`) | #83 | RC-2: `interface{}` boxing in `model.LogAttr.Value` |
| P3 — Append-to-buf context API | #84 | RC-1: `map[string]any` + `[]string` intermediate in `setLogContextFields` |
| P4 — Inline framing, eliminate double-buffer | #85 | RC-4: `en.Append(nil, e.buf)` + `append([]byte(nil), Data...)` |
| P5 — Channel capacity >= 1000 + configurable | #86 | RC-5: channel cap=10 blocks 8-goroutine parallel callers |

### Out of scope

- New encoder formats (CBOR, Protobuf, plain-text variants).
- Changes to the async pipeline architecture (background goroutine, `LogEvent` channel shape, flush loop).
- Lock-free ring buffer as a channel replacement (Option C in issue #86; deferred to a future epic).
- New log levels or changes to `enum.LogLevel` taxonomy.
- OpenTelemetry SDK integration.
- Changes to `Fatal`/`Panic` semantics (`runtime.Goexit` / `panic`).
- Raising or bypassing the bench-check gate threshold.

---

## 3. Open Questions — Resolved

The wiki (section 10) surfaced five open questions. All are resolved here; the architect and engineers must not reopen them without explicit Project Lead approval.

### Q1 — Typed field shape (P2)

**Decision:** Add `Str`, `Int`, `Bool`, `Float64` as methods directly on `LogEntry` (zerolog style). Internally, the typed field union struct carries a `Kind` discriminant (`KindStr`, `KindInt`, `KindBool`, `KindFloat64`, `KindAny`) and four value fields; accessor methods dispatch on `Kind`. The existing `model.LogAttr{Key: "k", Value: v}` struct-literal syntax continues to work unchanged via `KindAny` with the existing `config.AppendField` type-switch path. No source change is required at existing call sites.

Rationale: Direct methods on `LogEntry` avoid variadic slice allocation and match the zerolog ergonomic model that callers on throughput-sensitive paths will already expect. The union struct approach avoids `interface{}` boxing while remaining backward-compatible.

### Q2 — Context parser migration (P3)

**Decision:** Keep `SetContextFieldsParser(func(context.Context) map[string]any)` exactly as-is, marked as the legacy path. Add a new registration function `SetContextFieldsAppender(func(context.Context, []byte) []byte)` (working name `ContextFieldsAppender`). On the hot path, if a `ContextFieldsAppender` is registered it is called and its output appended directly to `e.buf`; the old `ContextFieldsParser` path (which allocates `map[string]any` and `[]string`) is used only if no appender is registered. Both registrations can coexist; appender takes priority.

Rationale: A purely internal adapter (wrapping the old `map[string]any` return into a byte append) would impose an allocation on every call even when the caller has not registered any context parser. The dual-registration approach lets new callers opt into zero-alloc immediately while legacy callers continue to work without any source change.

### Q3 — Encoder framing coupling (P4)

**Decision:** Add two methods to the `encoder.Encoder` interface: `OpenBytes() []byte` and `CloseBytes() []byte`. Each encoder returns its frame delimiters as byte slices (`JSONEncoder`: `OpenBytes` = `{`, `CloseBytes` = `}\n`; `TextEncoder`: `OpenBytes` = `""`, `CloseBytes` = `\n`). `entry.logWithSkip` prepends `OpenBytes()` to `e.buf` before field encoding and appends `CloseBytes()` after, then passes `e.buf` directly to `PublishLog`. The `Encoder.Append` method is retained on the interface for backward compatibility (any external implementations satisfy it) but is no longer called on the hot path.

Rationale: Adding `OpenBytes`/`CloseBytes` keeps the `Encoder` abstraction intact, avoids hard-coding JSON brace logic in `entry`, and lets the text encoder continue to function correctly (empty open, newline-only close). It is a minimal, auditable interface extension.

Implementation note: existing implementations (`JSONEncoder`, `TextEncoder`) must add both methods. Any third-party `Encoder` implementation not in this repo will fail to compile until it adds the two methods — this is an acceptable, detectable break on a minor version bump boundary.

### Q4 — Channel resize behaviour (P5)

**Decision:** No dynamic resize. `config.SetChannelCapacity(n int)` is effective only before the first log call; it sets the capacity used by the next `resetConfig` invocation (which is called at `init()`). The intended usage pattern is: call `config.SetChannelCapacity(n)` at program startup, before any logger is used, then call `resetConfig` (or rely on `init()` if the package has not yet been initialised). There is no runtime channel drain-and-replace mechanism. This avoids the risk of dropping in-flight events.

`DafaultLogBuffer` (currently `20` in `config.go`) is increased to `1000` as the new package default. `SetChannelCapacity` validates its argument: values <= 0 are silently clamped to `1000`; values > 100,000 are silently clamped to `100,000`, with a `log.Printf` warning in both cases. No panic.

### Q5 — Baseline ownership (P5)

**Decision:** The Project Lead runs `./scripts/bench-check.sh --update-baseline` after all five story PRs (#82–#86) are merged into `feat/81-v2-performance` and the gate is green. The baseline update commit lands on the feature branch PR to `develop`, not on individual story PRs. The commit message references issue #81.

---

## 4. Functional Requirements

### MUST — blocking for Epic V2

| ID | Requirement | Story |
|---|---|---|
| F1 | `MinLevel` check on the hot path uses an `atomic.Int32` load; no mutex is acquired for the level gate. | P1 / #82 |
| F2 | Each log call acquires `configMu.RLock` at most once per call (single config snapshot at the top of `logWithSkip`); the current 6-acquisition pattern at `entry.go:117,129,142–144,250` is eliminated. | P1 / #82 |
| F3 | `LogEntry` exposes typed field methods `Str(key, val string)`, `Int(key string, val int64)`, `Bool(key string, val bool)`, `Float64(key string, val float64)` that append directly to `e.buf` without `interface{}` boxing. These are chainable (return `*LogEntry`). | P2 / #83 |
| F4 | Existing `model.LogAttr` struct literal call sites (`[]model.LogAttr{{Key: "k", Value: v}}`) continue to compile and produce identical log output without any source change. | P2 / #83 |
| F5 | A `ContextFieldsAppender` type (`func(context.Context, []byte) []byte`) and `SetContextFieldsAppender` registration function are added to the `config` package. When registered, context fields are appended directly to `e.buf`; no `map[string]any` or `[]string` is allocated on the hot path. | P3 / #84 |
| F6 | The existing `SetContextFieldsParser(func(context.Context) map[string]any)` public API remains callable and continues to produce correct output; it is the fallback when no `ContextFieldsAppender` is registered. | P3 / #84 |
| F7 | `encoder.Encoder` gains two new methods: `OpenBytes() []byte` and `CloseBytes() []byte`. `JSONEncoder` returns `{` and `}\n`; `TextEncoder` returns empty and `\n`. | P4 / #85 |
| F8 | `entry.logWithSkip` writes framing directly into `e.buf` using `OpenBytes`/`CloseBytes`; the call to `en.Append(nil, e.buf)` is removed. | P4 / #85 |
| F9 | `PublishLog` sends the buffer to the channel without a second `append([]byte(nil), Data...)` defensive copy. The single safe copy is now the framed `e.buf` itself, whose ownership is transferred to the channel on send; `e.Put()` resets the pool slot's backing slice independently. | P4 / #85 |
| F10 | The default async channel capacity (`DafaultLogBuffer` in `config.go`) is raised from `20` to `1000`. | P5 / #86 |
| F11 | `config.SetChannelCapacity(n int)` is added. It sets the channel size used by the next `resetConfig` call. Values <= 0 are clamped to 1000; values > 100,000 are clamped to 100,000. A `log.Printf` warning is emitted on clamp. | P5 / #86 |
| F12 | All five stories ship with a `*_bench_test.go` file covering the changed hot path with `b.ReportAllocs()` enabled. | All |
| F13 | `./scripts/bench-check.sh` exits 0 after all stories are merged: no benchmark mean > 1,000 ns/op; no statistically significant regression vs baseline as reported by `benchstat`. | All |

### SHOULD

| ID | Requirement | Story |
|---|---|---|
| F14 | `Str`/`Int`/`Bool`/`Float64` methods are documented in godoc with usage examples contrasting them with `model.LogAttr` struct literals. | P2 / #83 |
| F15 | A code comment at the top of `setLogContextFields` (or its replacement) explains the dual-registration model and when each path fires. | P3 / #84 |
| F16 | `SetChannelCapacity` is exposed in the `ConfigBuilder` fluent API so capacity can be set at init time alongside other options. | P5 / #86 |

### COULD

| ID | Requirement | Notes |
|---|---|---|
| F17 | `Float32` and `Uint64` typed field methods added to `LogEntry`. | Deferred if they do not affect the benchmark target; add only if alloc budget allows. |
| F18 | `config.ChannelCapacity() int` getter for observability in tests. | Convenience; no functional impact. |

---

## 5. Non-Functional Requirements

### 5.1 Latency (hard gate — enforced by `./scripts/bench-check.sh`)

| Path | Metric | Target | Current |
|---|---|---|---|
| Parallel 10-field hot path | mean ns/op | <= 1,000 | ~3,440 |
| Parallel 10-field hot path | p99 ns/op | < 1,000 | out of SLO |
| Filtered path (level gate) | mean ns/op | <= 35 | ~35 (must not regress) |
| Filtered path | allocs/op | 0 | 0 (must not regress) |

The 1,000 ns/op threshold in `bench-check.sh` is immutable for this epic. Any story that causes a benchmark to exceed this threshold must be fixed before marking the PR ready for review (see CLAUDE.md Performance Gate).

### 5.2 Throughput

`BenchmarkLognugget_Parallel_10CtxFields` at GOMAXPROCS=8 must reach >= 3 M ops/sec. This is the Definition of Done for umbrella issue #81.

### 5.3 Allocations

| Path | Target after P1–P4 complete |
|---|---|
| Parallel 10-field hot path | <= 5 allocs/op (from 55) |
| Filtered path | 0 allocs/op (no regression) |

### 5.4 Availability and Race Safety

- All changes must pass `go test -race ./...` with no new data races detected.
- The `atomic.Int32` minLevel field (P1) and single-snapshot config pattern must be verified under `-race` with `GOMAXPROCS=8` before the story PR is marked ready.
- The channel send in `PublishLog` (P4/P5) must remain non-blocking under steady-state load with the new default capacity. Callers that fill the channel will still block — this is acceptable and documented.

### 5.5 Backward Compatibility

No breaking change to any exported symbol in `entry/`, `config/`, `model/`, or `encoder/` packages, with one noted exception:

- The `encoder.Encoder` interface gains two methods (`OpenBytes`, `CloseBytes`). Any third-party implementation outside this repository will fail to compile. This is an acceptable minor-version interface extension; it must be called out in the release notes and the feature branch PR description. Internal implementations (`JSONEncoder`, `TextEncoder`) are updated as part of P4.

All other exported symbols retain their signatures. Existing call sites using `model.LogAttr` struct literals and `SetContextFieldsParser` must compile and pass tests without modification.

### 5.6 Compliance

- No new external dependencies may be introduced. The library must remain importable as a standalone Go module.
- `go mod tidy` must produce no diff after all stories are merged.
- `golangci-lint run` must be green on every story PR.

---

## 6. Stack and Tooling Proposal

All changes use the existing stack. No new packages or modules.

| Component | Current state | Change required |
|---|---|---|
| `sync/atomic` (stdlib) | Not used for minLevel | P1 adds `atomic.Int32` field to `Config` or as a package-level var in `config`; guards `minLevel` reads on the hot path |
| `sync.RWMutex` (`configMu`) | 6 `RLock` acquisitions per call in `entry.go` | P1 reduces to 1 acquisition per call (single snapshot); `SetMinLevel` retains `Lock` for write |
| `model.LogAttr` | `Value LogAttrValue` (`any`) | P2 adds `Kind` discriminant + typed value fields; existing `Value any` field retained for backward compat (`KindAny`) |
| `entry.LogEntry` | No typed append methods | P2 adds `Str`/`Int`/`Bool`/`Float64` chainable methods writing to `e.buf` via `config.Append*` helpers |
| `config.ContextFieldsParser` | `func(context.Context) map[string]any` | P3 adds `ContextFieldsAppender func(context.Context, []byte) []byte` alongside; hot path prefers appender |
| `encoder.Encoder` interface | Single `Append(dst, body []byte) []byte` method | P4 adds `OpenBytes() []byte` and `CloseBytes() []byte`; `Append` retained but not called on hot path |
| `config.DafaultLogBuffer` | `20` | P5 raises to `1000`; `SetChannelCapacity` added |
| Benchmarks | `source_capture_bench_test.go` in `entry/` | Each story adds or extends `*_bench_test.go` in its affected package; must include `BenchmarkLognugget_Parallel_10CtxFields` after P5 |

Rationale for no new dependencies: the root causes are all algorithmic (mutex hot-spotting, boxing, intermediate allocations, channel starvation). Each fix uses stdlib primitives already imported by the project.

---

## 7. Delivery Milestones and Implementation Order

The stories are ordered by dependency, not purely by impact. P1 must land first because P2, P3, and P4 all depend on the single-snapshot config pattern it establishes (they each read from the snapshot without acquiring a second lock). P5 is a 3-line change and is independent; it is sequenced last to keep the feature branch clean and to sequence the baseline update naturally after the encoding improvements.

```
feat/81-v2-performance
├── feat/81-v2-performance/p1-atomic-minlevel       → #82  (FIRST — unblocks P2, P3, P4)
├── feat/81-v2-performance/p2-typed-field-api       → #83  (after P1 merges to feature branch)
├── feat/81-v2-performance/p3-ctx-appender          → #84  (after P1 merges to feature branch)
├── feat/81-v2-performance/p4-inline-framing        → #85  (after P1 merges to feature branch)
└── feat/81-v2-performance/p5-channel-capacity      → #86  (independent; last)
```

P2, P3, P4 may be worked in parallel on their sub-branches once P1 is merged to `feat/81-v2-performance`. Each sub-branch PR targets the feature branch, not `develop`.

### Milestone targets

| Milestone | Gate |
|---|---|
| M1: P1 merged to feature branch | `go test -race ./...` green; `configMu.RLock` count per call verified at 1 via benchmark |
| M2: P2 + P3 + P4 merged to feature branch | allocs/op <= 5 on parallel path; `BenchmarkLognugget_Parallel_10CtxFields` <= 1,000 ns/op |
| M3: P5 merged to feature branch | `BenchmarkLognugget_Parallel_10CtxFields` >= 3 M ops/sec; `bench-check.sh` exits 0 |
| M4: Baseline update commit on feature branch | `bench-baseline.txt` updated by Project Lead; feature branch PR to `develop` opened |
| M5: Merge to `develop` | All story PRs (#82–#86) merged; umbrella issue #81 closed |

---

## 8. Per-Story Acceptance Criteria

### P1 — Atomic minLevel + single config snapshot (#82)

Files likely touched: `config/config.go`, `entry/entry.go`.

| Criterion | Measurable target |
|---|---|
| Level gate path | 0 `configMu.RLock` acquisitions (uses `atomic.Int32.Load` instead) |
| Log body path | Exactly 1 `configMu.RLock` acquisition per `logWithSkip` call |
| Filtered path benchmark | <= 35 ns/op, 0 allocs (no regression) |
| Race detector | `go test -race ./...` green |
| Estimated ns/op saving | ~150–250 ns on hot path (RC-3) |

### P2 — Typed field API (#83)

Files likely touched: `model/attr.go`, `entry/entry.go`, new `entry/typed_fields.go` or inline.

| Criterion | Measurable target |
|---|---|
| `Str("k","v")` bench | 0 allocs for the field append itself |
| Backward compat | `model.LogAttr{Key:"k", Value:"v"}` compiles, passes existing tests unchanged |
| Estimated alloc saving | ~30 allocs/op eliminated (RC-2) |
| Godoc | `Str`/`Int`/`Bool`/`Float64` documented with examples |

### P3 — Append-to-buf context API (#84)

Files likely touched: `config/config.go`, `entry/entry.go`.

| Criterion | Measurable target |
|---|---|
| Hot path (appender registered) | 0 `map[string]any` allocations, 0 `[]string` allocations per call |
| Legacy path (parser registered, no appender) | Existing behaviour unchanged; existing tests pass |
| Estimated saving | ~600 ns/op, ~23 allocs/op (RC-1 — largest single gain) |
| Test coverage | Unit test exercising appender path + legacy path coexistence |

### P4 — Inline framing (#85)

Files likely touched: `encoder/encoder.go`, `encoder/json_encoder.go`, `encoder/text_encoder.go`, `entry/entry.go`, `config/config.go`.

| Criterion | Measurable target |
|---|---|
| `en.Append(nil, e.buf)` call | Removed from `logWithSkip` |
| `append([]byte(nil), Data...)` in `PublishLog` | Removed |
| Net allocation change | 2 allocs/op eliminated (RC-4) |
| JSON output correctness | Existing JSON encoder tests pass; output is `{<body>}\n` as before |
| Text output correctness | Existing text encoder tests pass; output is `<body>\n` as before |
| Interface extension | `JSONEncoder` and `TextEncoder` both compile with `OpenBytes`/`CloseBytes` added |

### P5 — Channel capacity (#86)

Files likely touched: `config/config.go`.

| Criterion | Measurable target |
|---|---|
| `DafaultLogBuffer` default | 1000 (changed from 20) |
| `BenchmarkLognugget_Parallel_10CtxFields` | >= 3 M ops/sec at GOMAXPROCS=8 (RC-5 unblocks callers) |
| `SetChannelCapacity(0)` | Clamped to 1000, `log.Printf` warning emitted, no panic |
| `SetChannelCapacity(200000)` | Clamped to 100000, `log.Printf` warning emitted, no panic |
| Change size | Approximately 3 lines of logic + validation + `SetChannelCapacity` function |

---

## 9. Dependencies

| Dependency | Type | Blocking |
|---|---|---|
| P1 must merge to feature branch before P2, P3, P4 start implementation | Intra-epic, sequential | Yes — P2/P3/P4 read the config snapshot established in P1 |
| P5 is independent of P1–P4 | Intra-epic | No — may be started any time; sequenced last by convention |
| `encoder.Encoder` interface extension (P4) | Internal — no external consumers tracked | Confirm no external forks import `encoder.Encoder` before cutting the feature branch PR |
| `./scripts/bench-check.sh` baseline at v1.0.0 (~3,440 ns/op) | Baseline was established at v1.0.0 | Gate will pass once P1–P5 bring ns/op <= 1,000; baseline update (M4) is the final step |

---

## 10. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| P1 atomic minLevel introduces a TOCTOU window: level is read atomically but body encoding uses a stale snapshot | Low | Medium | The snapshot is taken immediately after the atomic gate in the same goroutine; no other goroutine modifies the snapshot mid-call. Document this as intentional eventual-consistency for level changes (consistent with zerolog). |
| P4 ownership transfer: `e.buf` backing array is sent to channel then returned to pool — pool recycler and background drain race | High without mitigation | High | P4 must ensure `e.buf` is reset to a fresh backing slice (e.g. `e.buf = make([]byte, 0, initBufCap)`) before `e.Put()`, not `e.buf[:0]`. The existing pool reset path must be audited. Run `-race` on the benchmark to confirm. |
| `encoder.Encoder` interface extension breaks external implementations | Medium (unknown forks) | Low (compile-time, not runtime) | Document in feature branch PR description and release notes. Provide a `NoopFramer` embed helper if needed. |
| P3 dual-registration complexity: callers that register both parser and appender get silent appender-wins behaviour | Low | Low | Document priority clearly in godoc. Add a test that registers both and asserts appender output wins. |
| P2 backward compat: `model.LogAttr.Value any` retained alongside new `Kind` field — struct size increase may affect `sync.Pool` alignment | Low | Low | Benchmark `e.Put()`/`NewLogEntry()` round-trip allocation count before and after P2; confirm 0 new allocs on filtered path. |
| Channel at cap=1000 under extreme burst (> 1000 in-flight) still blocks callers | Known, accepted | Low | RC-5 fix is a throughput fix, not a lossless-guarantee fix. Document in `SetChannelCapacity` godoc. Lock-free ring buffer deferred to future epic (N4). |
| Bench gate already fails at v1.0.0 baseline — P1 alone may not clear the 1,000 ns/op threshold | Medium | Medium | P1 + P3 together account for ~750–850 ns saving. Target: clear gate after P1+P3 merge; P2 and P4 provide margin. Gate is only evaluated at M3 (all five stories merged). |

---

## 11. Definition of Done (Epic)

All of the following must be true before umbrella issue #81 is closed and `feat/81-v2-performance` is merged to `develop`:

1. `BenchmarkLognugget_Parallel_10CtxFields` (GOMAXPROCS=8) >= 3 M ops/sec (ns/op <= ~333).
2. `BenchmarkLognugget_Parallel_10CtxFields` allocs/op <= 5.
3. `BenchmarkLognugget_Filtered` <= 35 ns/op, 0 allocs (no regression from v1.0.0).
4. `go test -race ./...` exits 0.
5. `golangci-lint run` exits 0.
6. `./scripts/bench-check.sh` exits 0 (mean ns/op <= 1,000; no statistically significant regression vs updated baseline).
7. All five story PRs (#82–#86) merged into `feat/81-v2-performance`.
8. `bench-baseline.txt` updated by Project Lead and committed to feature branch.
9. Existing call sites using `model.LogAttr` struct literals and `SetContextFieldsParser` compile and pass tests without modification.
10. `encoder.Encoder` interface extension (`OpenBytes`/`CloseBytes`) documented in the feature branch PR description.

---

## 12. Handoff to Architect

The architect (`agent-architect`) should produce a High-Level Design covering:

1. **P1 — atomic minLevel**: exact placement of `atomic.Int32` (package-level var vs field on `Config`); memory ordering guarantees needed; whether `SetMinLevel` write path requires a lock or can use `atomic.Store`.
2. **P2 — typed field union**: precise shape of the `LogAttr` union struct (field names, zero-value semantics for `KindAny`); whether `Str`/`Int`/`Bool`/`Float64` live on `LogEntry` directly or via a separate embedded `FieldAppender`; call-site shape with and without method chaining.
3. **P3 — context appender**: exact function signature for `ContextFieldsAppender`; how `setLogContextFields` is restructured; whether the function is stored on `Config` alongside `contextParser` or as a separate package-level variable.
4. **P4 — inline framing + ownership transfer**: how `e.buf` ownership is safely transferred to the channel before `e.Put()` resets the pool slot; whether `PublishLog` signature changes; how `OpenBytes`/`CloseBytes` are cached (called once per log line, not stored on the encoder call).
5. **P5 — channel capacity**: whether `SetChannelCapacity` stores its value on `Config` or as a separate package-level var; exact guard against post-init calls; how `resetConfig` reads the new capacity.
6. **Benchmark coverage plan**: which packages get new `*_bench_test.go` files; whether a single top-level `BenchmarkLognugget_Parallel_10CtxFields` bench covers all five stories or each story adds its own targeted bench.

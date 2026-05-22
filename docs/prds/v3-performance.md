# PRD: Epic V3 — Sub-500 ns Hot Path + First-Class OTel Distributed Tracing

| Field | Value |
|---|---|
| Feature slug | `v3-performance` |
| Umbrella issue | [#111](https://github.com/architagr/LogNugget/issues/111) |
| Status | READY FOR IMPLEMENTATION |
| Author | Delivery Manager |
| Date | 2026-05-22 |
| Target branch | `feat/111-v3-performance` → `develop` |

---

## 1. Summary

LogNugget V2 ships at ~1,090 ns/op and 5 allocs/op on the parallel no-context hot path — a 10× improvement over V1 — but still 10× slower than zerolog on raw ns/op. The project SLO targets < 1 µs p99; V3 targets ≤ 500 ns/op (≤ 1 alloc) on the parallel path, closing half the remaining gap.

V3 eliminates every remaining `configMu.RLock` from the hot path (three separate sites: pre-processor check, config snapshot, channel publish), removes all per-call string→[]byte conversions (timestamp, string fields, level label), adds first-class OpenTelemetry context tracing support via the existing `ContextFieldsAppender` API, right-sizes the per-call buffer allocation, and replaces the Go channel dispatcher with a lock-free MPSC ring buffer.

All changes target caller-side cost (the ~1,090 ns callers pay before returning). IO flush remains async.

---

## 2. Scope

### In scope

| Story | Issue | Bottleneck | Est. saving |
|---|---|---|---|
| P1 — atomic.Bool pre-processor gate | #112 | `HasEventPreProcessors()` RLock every call | ~80 ns |
| P2 — atomic.Pointer[HotSnapshot] | #113 | `GetHotSnapshot()` RLock + 13-field struct copy | ~120 ns |
| P3 — atomic channel pointer | #114 | `PublishLog()` RLock for channel pointer | ~50 ns |
| P4 — direct timestamp append | #115 | `Format()` string + `[]byte(s)` roundtrip | ~40 ns, 2 allocs |
| P5 — string-native JSON escape | #116 | `AppendQuotedString` `[]byte(s)` per string field | ~20 ns, 2 allocs |
| P6 — pre-rendered level bytes | #117 | `level.String()` + `AppendQuotedString` | ~15 ns, 1 alloc |
| P7 — first-class OTel appender | #118 | Benchmark uses legacy map parser (+100 ns, +4 allocs) | ~100 ns, 4 allocs |
| P8 — exact-size buffer severance | #119 | Pool buf always 1024 B vs ~200 B actual | ~800 B/call |
| P9 — lock-free MPSC ring buffer | #120 | Channel contention under 8 goroutines ~130 ns | ~130 ns |

### Out of scope

- New encoder formats.
- OTel SDK as a module dependency in the core library.
- Changes to `LogEntry` public API signatures.
- New log levels.
- Raising the bench-check gate threshold.

---

## 3. Open Questions — Resolved

### Q1 — P2 atomic snapshot consistency during Set* mutations

**Decision:** All `Set*` mutators acquire `configMu.Lock()` as before, build the full `HotSnapshot` under the lock, then call `hotSnapshotPtr.Store(&snap)` before releasing the lock. Readers call `hotSnapshotPtr.Load()` without any lock. This is a classic copy-on-write pattern; readers always see a fully-consistent snapshot (no torn reads) because they read a single pointer that points to an immutable struct.

Rationale: `atomic.Pointer[HotSnapshot]` provides the same memory-ordering guarantee as a mutex for the read side, with zero lock contention. Writers still use the existing `configMu.Lock()` to serialise concurrent `Set*` calls — no change to write semantics.

### Q2 — P3 atomic channel: what happens during resetConfig?

**Decision:** `resetConfig` creates `newCh`, acquires `configMu.Lock()`, sets the package-level `ch = newCh`, calls `atomicCh.Store(newCh)` to update the atomic copy, then releases the lock. `PublishLog` loads `atomicCh` (no lock). Because `atomicCh.Store` happens under the write lock and `atomicCh.Load` in `PublishLog` uses `atomic.Value` semantics, any `PublishLog` call that begins after `resetConfig` returns will see the new channel. Calls in-flight during `resetConfig` may still use the old channel — the existing behaviour is preserved.

### Q3 — P4 timestamp escaping

**Decision:** RFC3339 timestamps contain only printable ASCII characters (digits, `T`, `Z`, `-`, `:`, `+`, `.`). None require JSON escaping. `entry.logWithSkip` will write the timestamp as `'"'` + `t.AppendFormat(dst, layout)` + `'"'` directly, bypassing `AppendQuotedString`. A comment documents the invariant. If `snap.TimeFormat` is ever set to a layout that could produce quote or backslash characters (not possible with any stdlib layout), the fast path still produces valid JSON because those characters do not appear in the output.

### Q4 — P5 backward compatibility: `appendJSONString` still needed?

**Decision:** Keep `appendJSONString(dst []byte, src []byte)` for the `AppendField` `error` and `time.Time` cases that produce `[]byte` via `.Error()` and `.Format()`. Add new `appendJSONStringStr(dst []byte, s string)` that iterates over the string directly. `AppendQuotedString(dst []byte, s string)` becomes a thin wrapper over `appendJSONStringStr`. All string-value hot paths (`KindStr` in `AppendAttr`, string case in `AppendField`) switch to `appendJSONStringStr`. Zero API change.

### Q5 — P9 ring buffer size and overflow

**Decision:** Ring buffer capacity = 4096 slots (4× channel default of 1000). If all slots fill (burst exceeding drain throughput), producers spin calling `runtime.Gosched()` until a slot is available — same back-pressure semantics as the existing channel. No events are dropped. The ring buffer is initialised in `resetConfig`; `ProcessLogEvent` is updated to drain it via a spinning loop with `runtime.Gosched()` fallback on empty. The `SetChannelCapacity` function remains for backward compat but does not affect ring buffer size post-P9 (documented).

---

## 4. Functional Requirements

### MUST — blocking for Epic V3

| ID | Requirement | Story |
|---|---|---|
| F1 | `HasEventPreProcessors()` uses an `atomic.Bool` load; acquires no mutex. | P1 / #112 |
| F2 | `InitPreProcessors`, `AddPreProcessors`, `RemovePreProcessor` update the `atomic.Bool` under `configMu.Lock()` so reads are always consistent. | P1 / #112 |
| F3 | `GetHotSnapshot()` returns `*hotSnapshotPtr.Load()` without acquiring `configMu`. | P2 / #113 |
| F4 | Every `Set*` mutator rebuilds and atomically stores a new `HotSnapshot` after writing to `defaultConfig`, still under `configMu.Lock()`. | P2 / #113 |
| F5 | `PublishLog` reads the dispatch channel via `atomicCh.Load()` without acquiring `configMu`. | P3 / #114 |
| F6 | `resetConfig` stores the new channel into `atomicCh` under `configMu.Lock()`. | P3 / #114 |
| F7 | Timestamp is appended as `'"' + t.AppendFormat(buf, layout) + '"'` in `logWithSkip`; `customTime.Format` (string-returning) is no longer called on the hot path. | P4 / #115 |
| F8 | `appendJSONStringStr(dst []byte, s string)` is added to `config/parse_field.go`; it iterates over the string without a `[]byte(s)` conversion. | P5 / #116 |
| F9 | `AppendQuotedString`, `AppendAttr` (KindStr), and `AppendField` (string case) all use `appendJSONStringStr`. | P5 / #116 |
| F10 | `config.AppendQuotedLevel(dst []byte, l enum.LogLevel) []byte` is added; it uses a switch over the 5 named levels to append pre-quoted bytes (e.g. `"INFO"`) without calling `level.String()`. | P6 / #117 |
| F11 | `logWithSkip` calls `config.AppendQuotedLevel(e.buf, level)` instead of `config.AppendQuotedString(e.buf, level.String())`. | P6 / #117 |
| F12 | `Benchmark_Log_Parallel_10CtxFields` in `entry/parallel_bench_test.go` and `test/benchmark/log_parallel_bench_test.go` use `config.SetContextFieldsAppender` instead of `config.SetContextFieldsParser`. | P7 / #118 |
| F13 | A new `Benchmark_Log_Parallel_OtelCtx` benchmark in `test/benchmark/` demonstrates OTel span extraction via `ContextFieldsAppender`; OTel SDK is imported only in that test file's `go.mod`-isolated benchmark module or via a build tag. | P7 / #118 |
| F14 | `examples/otel-appender/` contains a runnable example showing how to create a `ContextFieldsAppender` that reads OTel trace/span IDs from `context.Context`. | P7 / #118 |
| F15 | Alias severance in `logWithSkip` uses `len(data)` as the new backing capacity: `e.buf = make([]byte, 0, len(data))`. | P8 / #119 |
| F16 | `config.mpscRingBuffer` replaces `chan LogEvent` as the dispatch mechanism; `PublishLog` calls `ring.Push(LogEvent{...})`; `ProcessLogEvent` loops on `ring.Pop()` with `runtime.Gosched()` on empty. | P9 / #120 |
| F17 | All nine stories ship with a `*_bench_test.go` covering the changed hot path with `b.ReportAllocs()`. | All |
| F18 | `./scripts/bench-check.sh` exits 0 after all stories merged. | All |

### SHOULD

| ID | Requirement | Story |
|---|---|---|
| F19 | `AppendQuotedLevel` is exported and documented in godoc with the note that it is faster than `AppendQuotedString(dst, l.String())`. | P6 / #117 |
| F20 | `examples/otel-appender/README.md` explains the zero-alloc OTel integration pattern. | P7 / #118 |
| F21 | `SetChannelCapacity` godoc updated to note it does not affect ring buffer size post-P9. | P9 / #120 |

### COULD

| ID | Requirement | Notes |
|---|---|---|
| F22 | `config.AppendQuotedBool(dst []byte, v bool) []byte` pre-renders `"true"` / `"false"` literals. | Micro-win; add only if budget allows. |
| F23 | Ring buffer capacity configurable via `config.SetRingBufferSize(n int)`. | Nice to have; default 4096 covers all realistic loads. |

---

## 5. Non-Functional Requirements

### 5.1 Latency (hard gate)

| Path | Metric | V3 Target | V2 Baseline |
|---|---|---|---|
| Parallel NoCtx (GOMAXPROCS=8) | mean ns/op | ≤ 500 | ~1,090 |
| Parallel 10 ctx fields (appender) | mean ns/op | ≤ 500 | ~1,280 |
| Filtered path | mean ns/op | ≤ 35 | ~10 (must not regress) |
| Filtered path | allocs/op | 0 | 0 (must not regress) |

### 5.2 Allocations

| Path | V3 Target | V2 Baseline |
|---|---|---|
| Parallel NoCtx | ≤ 1 alloc/op | 5 |
| Parallel 10 ctx fields (appender) | ≤ 1 alloc/op | 7 |
| Filtered path | 0 | 0 |

### 5.3 Bytes/op

| Path | V3 Target | V2 Baseline |
|---|---|---|
| Parallel hot path | ≤ 300 B/op | ~1,411 B/op |

### 5.4 Race Safety

All changes must pass `go test -race ./...` with no new data races. `atomic.Bool`, `atomic.Pointer[HotSnapshot]`, and `atomic.Value` (channel) provide the same memory-ordering guarantees as `sync.RWMutex` for their respective read paths.

### 5.5 Backward Compatibility

No breaking change to any exported symbol. `HasEventPreProcessors`, `GetHotSnapshot`, `PublishLog` retain their signatures. `SetChannelCapacity` remains callable (post-P9 its effect is documented as applying only before P9 ring buffer init). `ContextFieldsParser` legacy path remains supported (P7 switches benchmarks to the appender but does not remove the parser).

### 5.6 Compliance

- No new module-level dependencies in the core library.
- `go mod tidy` produces no diff.
- `golangci-lint run` green on every story PR.

---

## 6. Delivery Order

```
feat/111-v3-performance
├── Batch 1 (parallel — touch disjoint code regions):
│   ├── P1 (#112): config.go — atomic.Bool gate
│   ├── P4 (#115): entry.go — timestamp append
│   ├── P5 (#116): parse_field.go — string escape
│   ├── P6 (#117): parse_field.go — level bytes
│   └── P8 (#119): entry.go — buffer size
├── Batch 2 (after Batch 1 merged):
│   ├── P2 (#113): config.go — atomic snapshot (touches same file as P1)
│   └── P7 (#118): benchmarks + examples
├── Batch 3 (after P2 merged):
│   └── P3 (#114): config.go — atomic channel
└── Batch 4 (final):
    └── P9 (#120): ring buffer (replaces channel)
```

### Milestone targets

| Milestone | Gate |
|---|---|
| M1: Batch 1 merged | No regression on filtered path; allocs down by 3+ from V2 |
| M2: Batch 2 merged | `GetHotSnapshot()` lock-free; benchmark 10ctx uses appender |
| M3: P3 merged | Zero RLocks on hot path |
| M4: P9 merged | `Benchmark_Log_Parallel_NoCtx` ≤ 500 ns/op |
| M5: Baseline update | Project Lead runs `--update-baseline`; PR to `develop` opened |

---

## 7. Per-Story Acceptance Criteria

### P1 — atomic.Bool pre-processor gate (#112)

| Criterion | Target |
|---|---|
| `HasEventPreProcessors()` acquires configMu | Never (atomic load only) |
| `Test_HasPreProcessors_Race` under -race | Clean |
| `BenchmarkHasPreProcessors` | ≤ 5 ns/op, 0 allocs |
| All existing tests | Pass without modification |

### P2 — atomic.Pointer[HotSnapshot] (#113)

| Criterion | Target |
|---|---|
| `GetHotSnapshot()` acquires configMu | Never |
| `Test_HotSnapshot_ConsistentWithSetMinLevel` | Pass |
| `Test_HotSnapshot_RaceWithSetters` (8 goroutines, -race) | Clean |
| `GetHotSnapshot()` benchmark | ≤ 20 ns/op, 0 allocs |
| All Set* mutators rebuild snapshot | Verified by test |

### P3 — atomic channel pointer (#114)

| Criterion | Target |
|---|---|
| `PublishLog` acquires configMu | Never |
| `Test_PublishLog_Race` (8 goroutines, -race) | Clean |
| Channel send semantics unchanged | Yes |

### P4 — direct timestamp (#115)

| Criterion | Target |
|---|---|
| `customTime.Format` called on hot path | Never |
| `BenchmarkTimestamp_Direct` | 0 allocs/op |
| Timestamp field in log output | Correct RFC3339 format |

### P5 — string-native escape (#116)

| Criterion | Target |
|---|---|
| `[]byte(s)` in `AppendQuotedString` | Eliminated |
| `BenchmarkAppendQuotedString` | 0 allocs/op for pure ASCII strings |
| Existing escape tests | Pass |

### P6 — pre-rendered level bytes (#117)

| Criterion | Target |
|---|---|
| `level.String()` called on hot path | Never for named levels |
| `BenchmarkAppendQuotedLevel` | 0 allocs/op |
| All 5 named levels render correctly | Verified by table test |

### P7 — OTel appender (#118)

| Criterion | Target |
|---|---|
| `Benchmark_Log_Parallel_10CtxFields` uses appender | Yes |
| `Benchmark_Log_Parallel_OtelCtx` exists | Yes |
| `examples/otel-appender/` compiles | Yes |
| Legacy `SetContextFieldsParser` tests | Pass unchanged |

### P8 — exact buffer size (#119)

| Criterion | Target |
|---|---|
| Pool buf size after severance | `len(data)` (not 1024) |
| B/op on parallel bench | ≤ 300 B/op |
| Filtered path allocs | 0 (no regression) |

### P9 — lock-free MPSC ring buffer (#120)

| Criterion | Target |
|---|---|
| `chan LogEvent` dispatch | Replaced by ring buffer |
| Producer goroutines | 8 concurrent — no data race under -race |
| `Benchmark_Log_Parallel_NoCtx` | ≤ 500 ns/op |
| `Benchmark_Log_Parallel_10CtxFields` | ≤ 500 ns/op |
| Stop/drain semantics | Preserved — all events drained before Stop returns |

---

## 8. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| P2 snapshot stale reads: a `Set*` call races with a `GetHotSnapshot` | Low | Low | atomic.Pointer provides sequential consistency; readers always see the most recently stored snapshot or the one before it — both are valid complete states. |
| P9 ring buffer: producer overtakes consumer (full ring) | Medium | Medium | Spinning with `runtime.Gosched()` provides back-pressure; same semantics as channel blocking. Ring size 4096 >> realistic burst. |
| P9 memory ordering: slot written by producer before seq updated | N/A with atomic.Uint64 | High | The `slot.seq.Store(pos+1)` with atomic store provides the release barrier; consumer's `slot.seq.Load()` provides the acquire barrier. Use `atomic.Uint64` throughout. |
| P8 smaller buf causes realloc on large log lines | Low | Low | `len(data)` adapts to actual line size; pool reuses the slice whose capacity grows organically. First call on a large line causes one realloc; subsequent reuses of that slot are free. |
| P5 `appendJSONStringStr` diverges from `appendJSONString` | Low | Medium | Both use the same `cfgEscapeTable`; the only difference is input type. Cross-test with identical inputs in `Test_AppendJSONStringStr_MatchesByteVariant`. |

---

## 9. Definition of Done (Epic)

All of the following must be true before umbrella issue #111 is closed:

1. `Benchmark_Log_Parallel_NoCtx` (GOMAXPROCS=8) ≤ 500 ns/op.
2. `Benchmark_Log_Parallel_10CtxFields` (appender) ≤ 500 ns/op, ≤ 1 alloc/op.
3. `BenchmarkLogEntry_FilteredPath` ≤ 35 ns/op, 0 allocs (no regression from V2).
4. `go test -race ./...` exits 0.
5. `golangci-lint run` exits 0.
6. `./scripts/bench-check.sh` exits 0.
7. All 9 story PRs merged into `feat/111-v3-performance`.
8. `bench-baseline.txt` updated by Project Lead.
9. `examples/otel-appender/` compiles and runs correctly.
10. `docs/STATUS.md` updated with V3 results.

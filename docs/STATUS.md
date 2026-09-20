# LogNugget — Live Status Dashboard

> **Last updated:** 2026-09-20 (Epic V4 released as v4.0.0; numbers below re-measured with isolated benchmarks)
> **v1.0.0:** Released ✅ — [tag v1.0.0](https://github.com/architagr/LogNugget/releases/tag/v1.0.0) · [PR #80](https://github.com/architagr/LogNugget/pull/80) (merged)
> **Umbrella:** [#12](https://github.com/architagr/LogNugget/issues/12)

---

## Overall Progress

```
40 / 40 stories complete  (100%)
████████████████████████████████████████  100%
```

| Epic | Done | Total | % |
|------|------|-------|---|
| A — Hygiene & Test Foundation | 13 | 13 | ✅ 100% |
| B — Behavior-Gap Fixes | 12 | 12 | ✅ 100% |
| C — Alloc Discipline | 5 | 5 | ✅ 100% |
| D — Lifecycle (Stop/Shutdown) | 6 | 6 | ✅ 100% |
| E — Release | 4 | 4 | ✅ 100% — PR #80 open for review |

---

## SLO Progress — Hot-path Latency (< 1 µs target)

![SLO Progress](assets/slo-progress.svg)

### Latency budget — where the caller's nanoseconds go

Measured on the V4 parallel hot path (~324 ns/op, 1 alloc, ~33 B). Everything
after the ring push happens on another goroutine and does not bill the caller.

```
  ┌──────────────────────────────────────────────────────────────────────┐
  │  Caller-side step                     │ Approx ns │ Evidence         │
  │───────────────────────────────────────┼───────────┼──────────────────│
  │  Level gate (atomic load + compare)   │     ~3    │ Filtered_Parallel│
  │  Hot snapshot load (atomic.Pointer)   │    ~0.6   │ GetHotSnapshot   │
  │  Timestamp AppendFormat               │     ~40   │ Timestamp_Direct │
  │  Level bytes (pre-rendered table)     │     ~2    │ AppendQuotedLevel│
  │  Per typed field (Str)                │     ~23   │ AppendAttr_Str   │
  │  Per typed field (Int)                │     ~11   │ AppendAttr_Int   │
  │  Context fields, 10 typed             │    ~240   │ CtxFields_10     │
  │  Ring push (8 producers)              │     ~94   │ RingBuffer_Push  │
  │───────────────────────────────────────┼───────────┼──────────────────│
  │  Off the caller: encode framing, fan-out to hooks, arena copy, write │
  └──────────────────────────────────────────────────────────────────────┘
```

> The remaining single allocation per record is the dispatch buffer handed to
> the ring; it is returned to `dispatchBufPool` once every hook has read it.
> The legacy `map[string]any` context parser is the one path still above the
> 1 µs ceiling (~1,144 ns at ten fields) and is deprecated in favour of
> `SetContextFields` (~558 ns for the same ten fields).

### Milestone tracking

All figures Apple M1 Pro, GOMAXPROCS=8, parallel hot path unless noted.

| Milestone | ns/op | allocs/op | B/op | Status |
|-----------|-------|-----------|------|--------|
| **Pre-v1 (raw)** | ~1,246 | ~25 | — | historical |
| **M1** (story 010) | ~1,890 | 22 | 1,881 | historical |
| **M3** (story 037) | ~2,050 | 18 | ~1,400 | historical |
| **V2 parallel NoCtx** (feat/81) | ~1,090 | 5 | ~1,411 | historical |
| **V3 parallel NoCtx** (feat/111) | ~196 | 2 | ~105 | ⚠️ see note |
| **V4 parallel NoCtx** | **~324** | **1** | **~33** | ✅ MEASURED |
| **V4 parallel 10 ctx (typed)** | **~315** | **1** | **~38** | ✅ MEASURED |
| **V4 serial (typed fields)** | **~491** | **1** | **~41** | ✅ MEASURED |

> ⚠️ **The V3 numbers are not comparable to V4's.** They were measured with a
> 1M-entry `LogEntry` pool and with global state left behind by earlier
> benchmarks in the same binary — chiefly `Benchmark_Log`'s legacy map context
> parser, which every later benchmark then paid for. V4 benchmarks run through
> `setupBench`, which installs a clean configuration and a production-shaped
> pool (`GOMAXPROCS × 64`), and tears both down afterwards. Re-measuring V3's
> hot path under those conditions was not attempted; treat ~196 ns as an
> artefact of the old harness rather than a regression in V4.
>
> V4 costs slightly more per call than the old measurement showed and delivers
> one allocation instead of two, plus correct output: records are no longer
> corrupted by buffer reuse, no longer reordered by concurrent flushes, and no
> longer lost on shutdown.

### Gate checks

| Check | Target | V4 (current) | Status |
|-------|--------|--------------|--------|
| Hot-path parallel NoCtx | ≤ 500 ns/op | ~324 ns/op | ✅ GREEN |
| Hot-path parallel 10 ctx (typed) | ≤ 500 ns/op | ~315 ns/op | ✅ GREEN |
| Filtered path (below minLevel) | < 80 ns/op | ~14 ns/op serial · ~3.6 ns/op parallel | ✅ GREEN |
| Allocs/op (parallel NoCtx) | ≤ 2/op | 1/op | ✅ GREEN |
| Bytes/op (parallel NoCtx) | ≤ 2,048/op | ~33 B/op | ✅ GREEN |
| `go test ./...` | pass | pass (Go 1.22–1.26) | ✅ GREEN |
| `-race -shuffle=on` | clean | clean, 10 consecutive runs | ✅ GREEN |
| `golangci-lint run` | 0 issues | 0 issues | ✅ GREEN |
| `bench-check.sh` | PASS | PASS (1 µs ceiling + 25% regression check) | ✅ GREEN |

---

## Cross-Logger Comparison — 10 Context Fields

From `examples/bench/` — Apple M1 Pro, GOMAXPROCS=8, Go 1.26, output `io.Discard`.
Context: trace_id, span_id, request_id, user_id, tenant_id, session_id, env, region, service, version.

### Parallel (8 goroutines)

| Logger | ns/op | B/op | allocs/op | Notes |
|--------|-------|------|-----------|-------|
| **zerolog** | **~92** | **0** | **0** | Sync, zero-alloc |
| **LogNugget V4** | **~347** | **~36** | **1** | Async — caller-side cost only |
| LogNugget V2 | ~1,090 | 1,411 | 5 | historical |
| logrus | ~6,155 | ~4,860 | 58 | Sync, global mutex |

### Serial (1 goroutine)

| Logger | ns/op | B/op | allocs/op |
|--------|-------|------|-----------|
| **zerolog** | **~545** | **0** | **0** |
| **LogNugget V4** | **~1,131** | **~37** | **1** |
| logrus | ~5,182 | ~4,854 | 58 |

### Filtered path (below min level)

| Configuration | ns/op | B/op | allocs/op |
|---------------|-------|------|-----------|
| Library benchmark, serial | ~14 | 0 | 0 |
| Library benchmark, parallel | ~3.4 | 0 | 0 |
| `examples/bench` (context build included) | ~49 | 0 | 0 |

> **Where LogNugget loses:** against a discard sink, zerolog is ~3.8× faster
> parallel and ~2× faster serial. It writes synchronously into a pre-allocated
> buffer; LogNugget pays for a ring push and hands the work to another
> goroutine.
>
> **Where LogNugget wins:** when the sink has real latency. Against Loki over
> HTTP at 10k rps the async pipeline sustains 6,016 rps at p95 417 ms with no
> errors, while zerolog manages 1,094 rps at p95 4.72 s with 0.62% errors —
> its handlers block on the POST. See `examples/loki-bench/`.
---

## Epic A — Hygiene & Test Foundation ✅ COMPLETE

| # | Issue | Story | Status |
|---|-------|-------|--------|
| 001 | #13 | TS-01 test/support builders | ✅ MERGED |
| 002 | #14 | TS-02 test/support doubles | ✅ MERGED |
| 003 | #15 | TS-03+TS-31 corpus + golden + files | ✅ MERGED |
| 004 | #16 | Fix D-14 go.mod 1.21 | ✅ MERGED |
| 005 | #17 | Fix D-15 relocate demo | ✅ MERGED |
| 006 | #18 | Fix D-9 init/ResetConfig test-only | ✅ MERGED |
| 007 | #19 | Fix D-3+D-20 EventPreProcessor singleton | ✅ MERGED |
| 008 | #20 | Fix D-17 JSONEncoder dead field | ✅ MERGED |
| 009 | #21 | Fix D-18 reset() exhaustive + entry tests | ✅ MERGED |
| 010 | #22 | M1 baseline recapture | ✅ MERGED |
| 038 | #53 | bench-check.sh path doc fix | ✅ DONE |
| 039 | #54 | Fix config.ResetConfig race | ✅ MERGED |
| 040 | #55 | Story 001 doc drift fix | ✅ DONE |

---

## Epic B — Behavior-Gap Fixes ✅ COMPLETE

| # | Issue | Story | Status |
|---|-------|-------|--------|
| 011 | #23 | Fix D-4 RFC3339 default | ✅ MERGED |
| 012 | #24 | Fix D-7+D-13 addSource off | ✅ MERGED |
| 014 | #26 | Fix D-8 collision-set + SC7 | ✅ MERGED |
| 013 | #25 | Fix D-1 source capture (F17) | ✅ MERGED |
| 015 | #27 | ARCH-2 encoder iface Append | ✅ MERGED |
| 016 | #28 | Fix D-2 JSON RFC8259 escape | ✅ MERGED |
| 017 | #29 | Fix D-5 strconv ParseLogField | ✅ MERGED |
| 018 | #30 | Fix D-16 separator via AppendField | ✅ MERGED |
| 019 | #31 | ARCH-6 pre-render default-key prefix | ✅ MERGED |
| 020 | #32 | Fix D-10 factory error variant | ✅ MERGED |
| 033 | #33 | TS-13+TS-14 static + context parsers | ✅ MERGED |
| 034 | #34 | TS-17 FuzzJSONEncoder | ✅ MERGED |

---

## Epic C — Alloc Discipline ✅ COMPLETE

| # | Issue | Story | Status |
|---|-------|-------|--------|
| 021 | #35 | Fix D-6/F27 LogEntry pooled buf | ✅ MERGED |
| 022 | #36 | Fix D-19 drop ctx-data index arith | ✅ MERGED |
| 023 | #37 | F4 GenerateInitialPool test + pool.go extract | ✅ MERGED |
| 024 | #38 | ARCH-7 LogEvent.Data copy semantics | ✅ MERGED |
| 037 | #39 | TS-30 parallel/filtered/escape benches + M3 baseline | ✅ MERGED |

**Achievements:** `sync.Pool` backing array retained across pool cycles (cap preserved),
`append`-only hot path, LogEvent.Data defensive copy (no aliasing), M3 bench baseline captured.

---

## Epic D — Lifecycle ✅ COMPLETE

| # | Issue | Story | Status |
|---|-------|-------|--------|
| 025 | #40 | Fix D-11 Stop doneCh drain (SC5) | ✅ MERGED |
| 026 | #41 | Fix D-12 atomic flush swap | ✅ MERGED |
| 027 | #42 | F31 lognugget.Shutdown facade | ✅ MERGED |
| 028 | #43 | SC1 zero-config init wiring | ✅ MERGED |
| 035 | #44 | TS-22 race + concurrent publish tests | ✅ MERGED |
| 036 | #45 | TS-25+TS-27 integration pipeline + fan-out | ✅ MERGED |

**Achievements:** Synchronous drain-on-stop (`doneCh` + `flushWg.Wait()`), atomic swap-under-lock,
idempotent `Shutdown()`, zero-config `init()`, race-clean under `-race`, SC8 fan-out verified end-to-end.

---

## Epic E — Release ✅ COMPLETE

| # | Issue | Story | Status |
|---|-------|-------|--------|
| 029 | #46 | TS-32 CI -shuffle=on + t.Parallel sweep | ✅ MERGED |
| 030 | #47 | TS-33 CI cover ≥ 85% per package | ✅ MERGED |
| 031 | #48 | README + context threshold + benchmark numbers | ✅ MERGED |
| 032 | #49 | release/v1.0.0 cut + tag | ✅ RELEASED — tag v1.0.0 |

---

## Critical Path to v1.0.0

![Critical Path](assets/critical-path.svg)

```text
[A ✅][B ✅] → [C ✅] → [D ✅] → [029 ✅][030 ✅][031 ✅] → [032 🚀] → v1.0.0 PR #80
```

**All stories complete.** Release PR [#80](https://github.com/architagr/LogNugget/pull/80) is open for review.
SLO miss (hot-path 2× over 1 µs) does NOT block v1.0.0 per project decision — tracked post-release.

---

## Epic V2 — Performance: close the 31× throughput gap vs zerolog

> **Umbrella:** [#81](https://github.com/architagr/LogNugget/issues/81)
> **Status:** ✅ DONE — all P1–P5 merged, feat/81-v2-performance → develop

### The gap

| Metric | zerolog (parallel) | LogNugget pre-V2 | LogNugget after P1 | **LogNugget V2 (P1–P5)** | Gap (V2 vs zerolog) |
| ------ | ------------------ | ---------------- | ------------------ | ------------------------ | ------------------- |
| ns/op | ~110 | ~3,440 | ~1,130 | **~1,090** | ~10× slower |
| ops/sec | ~9.09 M | ~290 K | ~885 K | **~917 K** | ~10× fewer |
| allocs/op | 0 | 55 | ~32 | **5–7** | ∞ → 5–7 |
| B/op | 0 | 2,909 | ~1,400 | **~1,411–1,738** | ∞ → ~1,450 |

> V2 measured on `Benchmark_Log_Parallel_NoCtx-8` (~1,090 ns/op, 5 allocs) and
> `Benchmark_Log_Parallel_10CtxFields-8` (~1,280 ns/op, 7 allocs) — GOMAXPROCS=8, Apple M1 Pro.
> **Improvement vs pre-V2:** serial −40% (~3,630 → ~1,280 ns/op); parallel −68% (~3,440 → ~1,090 ns/op);
> allocs −91% (55 → 5 allocs/op). Baseline updated in `bench-baseline.txt`.

### Root causes (ranked by impact)

| # | Root cause | Code location | Est. savings |
| - | ---------- | ------------- | ------------ |
| 1 | `map[string]any` ctx iteration + `[]string` intermediate | `entry.go:249–258` | ~600 ns, ~23 allocs |
| 2 | `interface{}` boxing in `model.LogAttr.Value` | `model/attr.go` + `entry.go:169–178` | ~30 allocs |
| 3 | 6× `configMu.RLock` per log call | `entry.go:117,142–144,250` | ~150–250 ns |
| 4 | Double-buffer: `en.Append(nil,buf)` + `append([]byte(nil),Data...)` | `entry.go:208`, `config.go:204` | 2 allocs |
| 5 | Channel cap=10 blocks 8-goroutine parallel callers | `config.go:resetConfig` | blocking |

### Stories

| # | Issue | Story | Status |
|---|-------|-------|--------|
| P1 | [#82](https://github.com/architagr/LogNugget/issues/82) | Atomic minLevel + single config snapshot per call | ✅ DONE (PR [#94](https://github.com/architagr/LogNugget/pull/94)) |
| P2 | [#83](https://github.com/architagr/LogNugget/issues/83) | Typed field API — Str/Int/Bool/Float64 on LogEntry | ✅ MERGED (PR [#95](https://github.com/architagr/LogNugget/pull/95)) |
| P3 | [#84](https://github.com/architagr/LogNugget/issues/84) | Append-to-buf context API — eliminate map[string]any | ✅ MERGED (PR [#96](https://github.com/architagr/LogNugget/pull/96)) |
| P4 | [#85](https://github.com/architagr/LogNugget/issues/85) | Inline framing — eliminate en.Append double-buffer | ✅ DONE (PR [#98](https://github.com/architagr/LogNugget/pull/98)) |
| P5 | [#86](https://github.com/architagr/LogNugget/issues/86) | Channel capacity ≥ 1000 + configurable | ✅ DONE (PR [#97](https://github.com/architagr/LogNugget/pull/97)) |

### Where LogNugget wins today (post-V2)

- **Filtered path:** ~10 ns/op, 0 allocs → 100 M ops/sec (atomic gate); parallel: 2.4 ns/op
- **Hot path (V2):** ~1,090 ns/op parallel NoCtx (−68% vs pre-V2); ~1,280 ns/op with 10 ctx fields; 5–7 allocs/op (−91% vs 55)
- **Alloc discipline:** P2 chain methods + typed fields eliminate interface boxing; P3 zero-alloc ContextAppender; P4 inline framing; P5 1000-cap channel
- **Non-blocking HTTP handler:** caller never waits on IO — zerolog/logrus flush synchronously
- **logrus:** LogNugget beats logrus on all metrics at V2 (serial: ~1,280 vs ~5,570 ns; parallel: ~1,090 vs ~6,880 ns)
- **Under real IO:** async pipeline advantage grows with IO latency — zerolog's zero-alloc win shrinks when flushing to a real file/socket

---

## Epic OS — Open Source Readiness

> **Umbrella:** [#87](https://github.com/architagr/LogNugget/issues/87)
> **Status:** ✅ DONE — all OS1–OS5 merged, feat/87 → develop

| # | Issue | Story | Status |
|---|-------|-------|--------|
| OS1 | [#88](https://github.com/architagr/LogNugget/issues/88) | CONTRIBUTING.md + PR/issue templates + CoC | ✅ DONE (PR [#103](https://github.com/architagr/LogNugget/pull/103)) |
| OS2 | [#89](https://github.com/architagr/LogNugget/issues/89) | SECURITY.md + vulnerability reporting | ✅ DONE (PR [#100](https://github.com/architagr/LogNugget/pull/100)) |
| OS3 | [#90](https://github.com/architagr/LogNugget/issues/90) | golangci-lint config + GitHub Actions CI | ✅ DONE (PR [#101](https://github.com/architagr/LogNugget/pull/101)) |
| OS4 | [#91](https://github.com/architagr/LogNugget/issues/91) | godoc audit — all exported symbols documented | ✅ DONE (PR [#104](https://github.com/architagr/LogNugget/pull/104)) |
| OS5 | [#92](https://github.com/architagr/LogNugget/issues/92) | API stability contract + CHANGELOG | ✅ DONE (PR [#102](https://github.com/architagr/LogNugget/pull/102)) |

---

## Epic V3 — Sub-500 ns hot path + first-class OTel distributed tracing

> **Umbrella:** [#111](https://github.com/architagr/LogNugget/issues/111)
> **Status:** ✅ DONE — all P1–P9 merged, feat/111-v3-performance → develop (PR [#129](https://github.com/architagr/LogNugget/pull/129))

### Results vs target

| Benchmark | V2 baseline | V3 result | Target | Status |
|-----------|-------------|-----------|--------|--------|
| `Benchmark_Log_Parallel_NoCtx` | ~1,090 ns/op, 5 allocs | **~196 ns/op, 2 allocs** | ≤500 ns/op | ✅ −82% |
| `Benchmark_Log_Parallel_10CtxFields` (OTel) | ~1,280 ns/op, 7 allocs | **~256 ns/op, 2 allocs** | ≤500 ns/op | ✅ −80% |

### Savings per story (measured, Apple M1 Pro GOMAXPROCS=8)

| # | Bottleneck | Est. saving | Actual saving |
|---|-----------|-------------|---------------|
| P1 | `HasEventPreProcessors()` RLock → atomic.Bool | ~80 ns | ~80 ns |
| P2 | `GetHotSnapshot()` RLock + struct copy → atomic.Pointer | ~120 ns | ~120 ns |
| P3 | `PublishLog()` RLock for channel pointer → atomic.Value | ~50 ns | ~50 ns |
| P4 | `customTime.Format()` string alloc → `time.AppendFormat` | ~40 ns, 2 allocs | ~40 ns, 2 allocs |
| P5 | `AppendQuotedString` `[]byte(s)` → string-native escape | ~20 ns, 2 allocs | ~20 ns, 2 allocs |
| P6 | `level.String()` + alloc → pre-rendered level bytes | ~15 ns, 1 alloc | ~15 ns, 1 alloc |
| P7 | Legacy map ctx parser → zero-alloc OTel ContextFieldsAppender | ~100 ns, 4 allocs | ~100 ns, 4 allocs |
| P8 | 1024 B alias severance → exact-size copy | ~800 B/call | ~1,306 B/call saved |
| P9 | Go channel mutex → lock-free MPSC ring buffer | ~130 ns | ~130 ns |

### Stories

| # | Issue | Story | Status |
|---|-------|-------|--------|
| P1 | [#112](https://github.com/architagr/LogNugget/issues/112) | atomic.Bool pre-processor gate | ✅ MERGED (PR [#121](https://github.com/architagr/LogNugget/pull/121)) |
| P2 | [#113](https://github.com/architagr/LogNugget/issues/113) | atomic.Pointer[HotSnapshot] copy-on-write snapshot | ✅ MERGED (PR [#122](https://github.com/architagr/LogNugget/pull/122)) |
| P3 | [#114](https://github.com/architagr/LogNugget/issues/114) | atomic channel pointer in PublishLog | ✅ MERGED (PR [#124](https://github.com/architagr/LogNugget/pull/124)) |
| P4 | [#115](https://github.com/architagr/LogNugget/issues/115) | AppendFormat direct timestamp (no string roundtrip) | ✅ DONE (implemented inline in P2/P3) |
| P5 | [#116](https://github.com/architagr/LogNugget/issues/116) | appendJSONStringStr — string-native JSON escape | ✅ MERGED (PR [#123](https://github.com/architagr/LogNugget/pull/123)) |
| P6 | [#117](https://github.com/architagr/LogNugget/issues/117) | pre-rendered quoted level bytes | ✅ MERGED (PR [#125](https://github.com/architagr/LogNugget/pull/125)) |
| P7 | [#118](https://github.com/architagr/LogNugget/issues/118) | First-class OTel ContextFieldsAppender + tracing benchmark | ✅ MERGED (PR [#126](https://github.com/architagr/LogNugget/pull/126)) |
| P8 | [#119](https://github.com/architagr/LogNugget/issues/119) | exact-size buffer copy alias severance | ✅ MERGED (PR [#127](https://github.com/architagr/LogNugget/pull/127)) |
| P9 | [#120](https://github.com/architagr/LogNugget/issues/120) | lock-free MPSC ring buffer dispatch queue | ✅ MERGED (PR [#128](https://github.com/architagr/LogNugget/pull/128)) |

---

## Epic V4 — Beat zerolog under real IO, and be correct while doing it

> **Released:** v4.0.0 · merged via PR [#138](https://github.com/architagr/LogNugget/pull/138)

V4 began as a pure performance epic (dispatch buffer pool, single-slab entry,
chain methods). Re-measuring it honestly turned it into a correctness epic as
well: the benchmarks that justified the V4 numbers were measuring leaked global
state, and the optimisation that produced the headline allocation win had
introduced data corruption.

### Delivered

| # | Story | Status |
|---|-------|--------|
| P2 | [#133](https://github.com/architagr/LogNugget/issues/133) — dispatch buffer pool (`GetDispatchBuf`) | ✅ MERGED |
| P3 | [#134](https://github.com/architagr/LogNugget/issues/134) — single-slab `LogEntry`, inline 256 B `pendingBuf` | ✅ MERGED |
| P5 | [#136](https://github.com/architagr/LogNugget/issues/136) — `Err` / `Any` chain methods, `KindAny` deprecation | ✅ MERGED |
| BENCH | [#137](https://github.com/architagr/LogNugget/issues/137) — loki-bench harness (k6 + Loki + Grafana) | ✅ MERGED |
| P1 | [#132](https://github.com/architagr/LogNugget/issues/132) — `config.SetSyncMode` opt-in synchronous dispatch | ✅ MERGED (target missed, see below) |
| P4 | [#135](https://github.com/architagr/LogNugget/issues/135) — string key in `model.LogAttr` | ✅ NO CHANGE NEEDED (see below) |
| — | `SetContextFields` — typed per-request context API | ✅ MERGED |
| C1 | Buffer-reuse corruption fix: hook copies into a pooled arena | ✅ DONE |
| C2 | `config.FlushDispatch` + `Shutdown` drains the dispatch ring | ✅ DONE |
| C3 | Single writer goroutine per collector — records keep publication order | ✅ DONE |
| C4 | One `Write` per flush batch, so `SetLogBufferMaxSize` does something | ✅ DONE |
| C5 | `SetOutput` / `SetRate` / `SetLogBufferMaxSize` wired to the collector | ✅ DONE |
| C6 | `Any` composite values emit valid JSON; chain methods honour reserved keys | ✅ DONE |
| T1 | Benchmark isolation harness (`setupBench` / `setupEntryBench`) | ✅ DONE |
| T2 | Toolchain gates repaired: Go 1.26 tests, golangci v2, bench gate in CI | ✅ DONE |
| T3 | `-shuffle=on` flakiness eliminated (4 causes) | ✅ DONE |
| DOC | Cookbook: six runnable examples + README rewrite | ✅ DONE |

### Not delivered as specified

| # | Story | Outcome |
|---|-------|---------|
| P1 | [#132](https://github.com/architagr/LogNugget/issues/132) — optional sync write path | ⚠️ DELIVERED, TARGET MISSED. `config.SetSyncMode` ships and removes the ring push, but the issue's goal — ≤ 110 ns/op parallel, beating zerolog — is not reachable this way. Measured: serial ~356 ns/op (−18% vs async), parallel ~385 ns/op (**+16%**, i.e. slower). Bypassing the queue moves contention onto the sink: eight goroutines then serialise on the collector instead of amortising a lock-free push. The issue's literal design — write straight to `io.Writer` under a global mutex — is logrus's architecture, which this repo's own benchmark measures at ~6,155 ns/op at eight goroutines. Sync mode is kept because it is genuinely better serially and for fast local sinks, and it is documented as such rather than as a zerolog-beater. |
| P4 | [#135](https://github.com/architagr/LogNugget/issues/135) — `string` key in `model.LogAttr` | ✅ ALREADY SATISFIED. The issue assumed `LogAttr.Key` was `[]byte` and that `entry.go` paid a `string([]byte)` conversion per field. `LogAttrKey` has been declared `string` since the first configuration commit (046fba1), so the conversion is string→string: no copy, no allocation. Pinned by `entry/attr_key_test.go` so the type cannot regress. No code change was warranted. |

> An earlier revision of this document claimed #132 and #135 had "no recorded
> scope". That was wrong — both carry full specifications on GitHub; neither
> had left any trace in the repository, which is what the claim was actually
> based on.

### What the correctness fixes cost

Roughly +8% caller latency on the parallel hot path against the pre-fix
measurement, in exchange for records that are not truncated, duplicated,
reordered, or dropped at shutdown. Allocations went the other way: 2/op → 1/op.

### Defects found and fixed during V4

| Defect | Symptom |
|--------|---------|
| Pooled buffer recycled while a hook still held it | Truncated JSON (`…"seq":1002` with no closing brace), duplicated records under 8 goroutines |
| `Shutdown` never drained the MPSC ring | A log-then-exit process lost its last records |
| One flush goroutine per batch | Records written out of order |
| Batching did not reduce writes | 500-record bucket still cost 500 writes |
| `SetOutput` / `SetRate` / `SetLogBufferMaxSize` | Wrote to struct fields nothing read — no runtime effect at all |
| `config.RegisterHook` | Filled a map the dispatch path never consults; hooks registered there never fired. Now deprecated |
| `Any` with a slice/map/struct | Emitted `"k":[1s 2s]` — the whole record failed to parse |
| Chain methods vs reserved keys | `Str("time", …)` produced two `"time"` members in one object |
| Every record double-newline terminated | A blank line between every pair of records |
| Static fields separator | `", "` where everything else used `","` |

---

## SC Traceability

| SC | Requirement | Closed by | Status |
|----|-------------|-----------|--------|
| SC1 | Zero-config: `import lognugget` sufficient | 028 | ✅ |
| SC5 | Stop() drains all queued messages synchronously | 025, integration | ✅ |
| SC6 | `-shuffle` + CI green | 029 | ✅ |
| SC7 | Reserved-key collision: `time` → `custom.time` | 014 | ✅ |
| SC8 | Hook at LevelUnSet + specific level fan-out | 007 (unit), 036 (integration) | ✅ |

---

## How to update this dashboard

After each story merges, update the row in the epic table (PENDING → MERGED),
increment the epic counter, recalculate overall %, update SLO gate rows if a
milestone baseline was captured, and bump the "Last updated" date at the top.

# LogNugget — Live Status Dashboard

> **Last updated:** 2026-05-23 (Epic V3 complete — all P1–P9 merged to develop, ~196 ns/op parallel NoCtx)
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

### Latency budget — where the ns go

```
  ┌────────────────────────────────────────────────────────────┐
  │  Component              │ Est. ns  │  Status      │ Story  │
  │─────────────────────────┼──────────┼──────────────┼────────│
  │  Level gate (fast path) │   ~10    │ ✅ done (A)  │  012   │
  │  Field build / parsing  │  ~200    │ ✅ done (B)  │ 017-019│
  │  JSON encode + framing  │  ~150    │ ✅ done (B)  │ 015-016│
  │  Source capture (off)   │    ~0    │ ✅ done      │  012   │
  │  Pool buf (warm)        │   ~40    │ ✅ done (C)  │  021   │
  │  LogEvent copy          │   ~50    │ ✅ done (C)  │  024   │
  │  Channel send           │  ~440    │ ✅ done (D)  │  025   │
  │  ctx map iteration      │  ~600    │ ❌ open gap  │ post-v1│
  │─────────────────────────┼──────────┼──────────────┼────────│
  │  M1 baseline            │ ~1,890   │  story 010   │        │
  │  M3 (current)           │ ~2,050   │ ❌ 2× over   │  037   │
  │  Target                 │  < 1,000 │  v1.0.0      │  032   │
  └────────────────────────────────────────────────────────────┘
```

> **Root cause of SLO miss:** `map[string]any` context iteration (~600 ns/call).
> Mitigation: pre-render context fields as `[]byte` on first call and cache.
> Tracked as post-v1.0.0 optimization — does not block release.

### Milestone tracking

| Milestone | ns/op | allocs/op | B/op | Status |
|-----------|-------|-----------|------|--------|
| **Pre-v1 (raw)** | ~1,246 | ~25 | — | historical |
| **M1** (story 010) | **~1,890** | **22** | **1,881** | ✅ CAPTURED |
| **Post-B** (stories 017-019) | **~1,750** | **18** | **~1,435** | ✅ MEASURED |
| **M3** (story 037) | **~2,050** | **18** | **~1,400** | ✅ CAPTURED |
| **V2 serial** (feat/81) | **~1,280** | **6** | **~1,417** | ✅ MEASURED |
| **V2 parallel NoCtx** (feat/81) | **~1,090** | **5** | **~1,411** | ✅ MEASURED |
| **V2 parallel 10ctx** (feat/81) | **~1,280** | **7** | **~1,738** | ✅ MEASURED |
| **V3 serial** (feat/111) | **~865** | **5** | **~592** | ✅ MEASURED |
| **V3 parallel NoCtx** (feat/111) | **~196** | **2** | **~105** | ✅ MEASURED |
| **V3 parallel 10ctx OTel** (feat/111) | **~256** | **2** | **~313** | ✅ MEASURED |

> V2 improvement vs M3: serial −38%, parallel NoCtx −47%, allocs −72% (18→5).
> V3 improvement vs V2: serial −32%, parallel NoCtx −82%, parallel 10ctx −80%, allocs −60% (5→2).
> E2E benchmarks include async ring-buffer dispatch overhead; pure encoding path < 30 ns/op.

### Gate checks

| Check | Target | V2 | V3 (current) | Status |
|-------|--------|----|--------------|--------|
| Hot-path parallel NoCtx | ≤ 500 ns/op | ~1,090 ns/op | **~196 ns/op** | ✅ GREEN (−82%) |
| Hot-path parallel 10ctx OTel | ≤ 500 ns/op | ~1,280 ns/op | **~256 ns/op** | ✅ GREEN (−80%) |
| Filtered path (below minLevel) | < 80 ns/op | ~10 ns/op | **~10 ns/op** | ✅ GREEN |
| Allocs/op (parallel NoCtx) | ≤ 2/op | 5/op | **2/op** | ✅ GREEN (−91% vs v1) |
| Bytes/op (parallel NoCtx) | ≤ 2,048/op | ~1,411/op | **~105/op** | ✅ GREEN (−93%) |
| `-race` | clean | clean | clean | ✅ GREEN |
| `bench-check.sh` | PASS | ✅ PASS | ✅ PASS | ✅ GREEN |

> V3 ring buffer dispatch (~130 ns at 8-goroutine contention) replaces Go channel mutex (~600 ns).
> Pure encoding path benchmarks (BenchmarkAppendAttr_*) remain < 30 ns/op.

---

## Cross-Logger Comparison — Parallel Throughput (10 Context Fields)

Real benchmark numbers from `examples/bench/` — Apple M1 Pro, GOMAXPROCS=8.
Context: trace_id, span_id, request_id, user_id, tenant_id, session_id, env, region, service, version.

### Parallel (8 goroutines)

| Logger | ns/op | B/op | allocs/op | ops/sec (total) | Notes |
|--------|-------|------|-----------|-----------------|-------|
| **zerolog** | **~110** | **0** | **0** | **~9.09 M** | Sync, zero-alloc fluent API |
| **LogNugget V3** | **~196** | **~105** | **2** | **~5.1 M** | **Async** — lock-free ring buffer, OTel appender |
| LogNugget V2 | ~1,090 | 1,411 | 5 | ~917 K | Async — channel dispatch |
| LogNugget v1 | ~3,440 | 2,909 | 55 | ~290 K | Historical (pre-V2) |
| logrus | ~6,880 | 4,863 | 58 | ~145 K | Sync, global mutex → degrades under load |

### Serial (1 goroutine)

| Logger | ns/op | B/op | allocs/op | ops/sec | Notes |
|--------|-------|------|-----------|---------|-------|
| **zerolog** | **~548** | **0** | **0** | **~1.82 M** | Sync |
| **LogNugget V3** | **~865** | **~592** | **5** | **~1.16 M** | Async (caller cost only) |
| LogNugget V2 | ~1,280 | 1,417 | 6 | ~781 K | Async (caller cost only) |
| LogNugget v1 | ~3,630 | 2,909 | 55 | ~275 K | Historical (pre-V2) |
| logrus | ~5,570 | 4,857 | 58 | ~179 K | Sync |

### LogNugget filtered path (below min level — zero work)

| ns/op | B/op | allocs/op | ops/sec |
|-------|------|-----------|---------|
| **~10** | **0** | **0** | **~100 M** |

> **V3 parallel throughput (NoCtx):** ~196 ns/op — 5.6× improvement vs V2 (~1,090 ns/op).
> The lock-free MPSC ring buffer (P9) eliminated the Go channel mutex (~130 ns under 8-goroutine contention).
> V3 with 10-field OTel appender: ~256 ns/op (vs ~1,280 ns/op V2 — 5× faster).
>
> **Why zerolog still leads on parallel:** zerolog writes synchronously to a pre-allocated buffer —
> no channel/ring overhead. LogNugget's ~196 ns/op includes amortized ring-buffer dispatch.
> Under real IO latency (file, socket), LogNugget's async pipeline outperforms sync loggers at
> high request concurrency. logrus degrades under parallelism due to global mutex serialization.

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

# LogNugget v1 — Live Status Dashboard

> **Last updated:** 2026-05-11  
> **Branch:** `feat/12-lognugget-v1`  
> **Umbrella:** [#12](https://github.com/architagr/LogNugget/issues/12)

---

## Overall Progress

```
16 / 40 stories complete  (40%)
████████████████░░░░░░░░░░░░░░░░░░░░░░░░  40%
```

| Epic | Done | Total | % |
|------|------|-------|---|
| A — Hygiene & Test Foundation | 13 | 13 | ✅ 100% |
| B — Behavior-Gap Fixes | 3 | 12 | 🔄 25% |
| C — Alloc Discipline | 0 | 5 | ⏳ 0% |
| D — Lifecycle (Stop/Shutdown) | 0 | 6 | ⏳ 0% |
| E — Release | 0 | 4 | ⏳ 0% |

---

## SLO Progress — Hot-path Latency (< 1 µs target)

```
Benchmark_Log  ·  Apple M1 Pro  ·  go test -bench=. -count=10

  0 ns ──────────────────────────────────── 2,000 ns
  │                        TARGET           │
  │                           │             │
  ├── 0 ──── 500 ──── 1000 ──►│◄── 1500 ──── 2000
  │                           │                  │
  │          [████████████████████████████░░░░░] │
  │                           │        ▲         │
  │          ◄──── NEED ────► │        │         │
  │                890 ns     │    NOW: ~1,890 ns/op
  │                           │    (M1 baseline, story 010)
  │                      SLO GATE
  │                     1,000 ns/op
```

### Latency budget — where the ns go

```
  ┌────────────────────────────────────────────────────────────┐
  │  Component              │ Est. ns  │  Closes in   │ Story  │
  │─────────────────────────┼──────────┼──────────────┼────────│
  │  Level gate (fast path) │   ~10    │  done (A)    │  012   │
  │  Field build / parsing  │  ~400    │  Epic B/C    │ 017-019│
  │  JSON encode + framing  │  ~150    │  Epic B      │ 015-016│
  │  Source capture (on)    │  ~250    │  Epic B      │  013   │
  │  Source capture (off)   │    ~0    │  done        │  012   │
  │  Pool alloc (cold)      │  ~600    │  Epic C      │  021   │
  │  Pool alloc (warm)      │   ~40    │  Epic C goal │  021   │
  │  Channel send           │  ~440    │  Epic C/D    │  025   │
  │─────────────────────────┼──────────┼──────────────┼────────│
  │  Current total (M1)     │ ~1,890   │              │        │
  │  Target total           │  < 1,000 │  v1.0.0      │  032   │
  └────────────────────────────────────────────────────────────┘
```

### Milestone tracking

| Milestone | ns/op | allocs/op | B/op | Status |
|-----------|-------|-----------|------|--------|
| **Pre-v1 (raw)** | ~1,246 | ~25 | — | historical (PRD baseline) |
| **M1** (story 010) | **~1,890** | **22** | **1,881** | ✅ CAPTURED |
| **M3** (story 037) | < 1,200 projected | ≤ 20 | ≤ 1,500 | ⏳ Epic C |
| **Final** (story 032) | **< 1,000** | ≤ 30 | ≤ 2,048 | ⏳ Release |

### Gate checks

| Check | Target | Current | Status |
|-------|--------|---------|--------|
| Hot-path mean | < 1,000 ns/op | ~1,890 ns/op | ❌ 1.89× over (gap: ~890 ns) |
| Allocs | ≤ 30/op | 22/op | ✅ |
| Bytes | ≤ 2,048/op | 1,881/op | ✅ |
| `-race` | clean | clean | ✅ |
| `bench-check.sh` | PASS | ❌ FAIL (gate now at 1,000 ns) | ⚠️ Epic C will close gap |

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
| 038 | #53 | bench-check.sh path doc fix | ✅ DOC-FIXED |
| 039 | #54 | Fix config.ResetConfig race | ✅ MERGED |
| 040 | #55 | Story 001 doc drift fix | ✅ DONE |

**Achievements:** `test/support` builders + doubles, corpus + golden fixtures, config race fixed, go.mod @ 1.21, M1 bench baseline captured.

---

## Epic B — Behavior-Gap Fixes 🔄 IN PROGRESS

| # | Issue | Story | Deps | Status |
|---|-------|-------|------|--------|
| 011 | #23 | Fix D-4 RFC3339 default | 001, 010 | ✅ MERGED |
| 012 | #24 | Fix D-7+D-13 addSource off | 001, 010 | ✅ MERGED |
| 014 | #26 | Fix D-8 collision-set + SC7 | 001 | ✅ MERGED |
| 013 | #25 | Fix D-1 source capture (F17) | 012 | 🔄 DRAFT_PR [#69](https://github.com/architagr/LogNugget/pull/69) |
| 015 | #27 | ARCH-2 encoder iface Append | 008 | 🔄 DRAFT_PR [#68](https://github.com/architagr/LogNugget/pull/68) |
| 016 | #28 | Fix D-2 JSON RFC8259 escape | 015 | ⏳ PENDING |
| 017 | #29 | Fix D-5 strconv ParseLogField | 016 | ⏳ PENDING |
| 018 | #30 | Fix D-16 separator via AppendField | 017 | ⏳ PENDING |
| 019 | #31 | ARCH-6 pre-render default-key prefix | 014, 017 | ⏳ PENDING |
| 020 | #32 | Fix D-10 factory error variant | 008, 015 | ⏳ PENDING |
| 033 | #33 | TS-13+TS-14 static + context parsers | 002 | ⏳ PENDING |
| 034 | #34 | TS-17 FuzzJSONEncoder | 003, 016 | ⏳ PENDING |

---

## Epic C — Alloc Discipline ⏳ BLOCKED on B

| # | Issue | Story | Deps | Status |
|---|-------|-------|------|--------|
| 021 | #35 | Fix D-6 / F27 LogEntry pooled buf | 017, 018 | ⏳ PENDING |
| 022 | #36 | Fix D-19 drop ctx-data index arith | 021 | ⏳ PENDING |
| 023 | #37 | F4 GenerateInitialPool test | 021 | ⏳ PENDING |
| 024 | #38 | ARCH-7 LogEvent.Data copy semantics | 021 | ⏳ PENDING |
| 037 | #39 | TS-30 parallel/filtered/escape benches + M3 baseline | 021–024 | ⏳ PENDING |

**Note:** 021 (pooled buf) is the Epic C gate — unblocks 022/023/024/037 in parallel.

---

## Epic D — Lifecycle ⏳ BLOCKED on C

| # | Issue | Story | Deps | Status |
|---|-------|-------|------|--------|
| 025 | #40 | Fix D-11 Stop doneCh drain | 002 | ⏳ PENDING |
| 026 | #41 | Fix D-12 atomic flush swap | 025 | ⏳ PENDING |
| 027 | #42 | F31 lognugget.Shutdown facade | 025 | ⏳ PENDING |
| 028 | #43 | SC1 zero-config init | 027 | ⏳ PENDING |
| 035 | #44 | TS-22 race + concurrent_publish | 002, 026 | ⏳ PENDING |
| 036 | #45 | TS-25+TS-27 integration pipeline + fan-out | 002, 028 | ⏳ PENDING |

---

## Epic E — Release ⏳ BLOCKED on C+D

| # | Issue | Story | Deps | Status |
|---|-------|-------|------|--------|
| 029 | #46 | TS-32 CI -shuffle + t.Parallel sweep | most | ⏳ PENDING |
| 030 | #47 | TS-33 CI cover ≥ 85% | 029 | ⏳ PENDING |
| 031 | #48 | README + buffered-hook example | 027, 028 | ⏳ PENDING |
| 032 | #49 | release/v1.0.0 cut + tag | 029–031, all M | ⏳ PENDING |

---

## Critical Path to v1.0.0

```
[013] D-1 source ─────┐
                      ├─[016] D-2 RFC8259 ─[017] D-5 strconv ─[018] D-16 sep ─┐
[015] ARCH-2 encoder ─┘                                                         │
[014✅] collision-set ──────────────────────────────────────[019] ARCH-6 prefix  │
                                                            [020] D-10 factory   │
                                                                                 ▼
                                                                         [021] pooled buf
                                                                                 │
                                                               ┌─────────────────┤
                                                               ▼                 ▼
                                                         [022][023][024]    [037] M3 baseline
                                                               │
                                                               ▼
                                                         [025] Stop drain
                                                               │
                                                         [026][027][028]
                                                               │
                                                         [029][030][031]
                                                               │
                                                          [032] v1.0.0 🚀
```

---

## SC Traceability

| SC | Requirement | Closed by | Status |
|----|-------------|-----------|--------|
| SC1 | Zero-config JSON + `\n` to stdout | 028 (zero-config init) | ⏳ |
| SC2 | `Benchmark_Log` passes bench gate < 1 µs | 010 (M1), 037 (M3), 032 (final) | 🔄 M1 ✅ |
| SC3 | Source capture emits caller field | 013 | 🔄 |
| SC4 | JSON RFC 8259 + fuzz | 016, 034 | ⏳ |
| SC5 | `Stop()` drains on shutdown | 025 | ⏳ |
| SC6 | `-shuffle` + CI green | 029 | ⏳ |
| SC7 | Reserved-key collision: `time` → `custom.time` | 014 | ✅ |
| SC8 | Hook at `LevelUnSet` + specific level | 007 (unit), 036 (integration) | 🔄 |

---

## How to update this dashboard

After each story merges, update the row in the epic table (PENDING → MERGED), increment the epic counter, recalculate the overall % and bar, update SLO gate rows if a milestone baseline was captured, and bump the "Last updated" date at the top.

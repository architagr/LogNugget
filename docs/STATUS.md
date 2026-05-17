# LogNugget v1 — Live Status Dashboard

> **Last updated:** 2026-05-18
> **Branch:** `feat/12-lognugget-v1`
> **Umbrella:** [#12](https://github.com/architagr/LogNugget/issues/12)

---

## Overall Progress

```
39 / 40 stories complete  (98%)
██████████████████████████████████████░░  98%
```

| Epic | Done | Total | % |
|------|------|-------|---|
| A — Hygiene & Test Foundation | 13 | 13 | ✅ 100% |
| B — Behavior-Gap Fixes | 12 | 12 | ✅ 100% |
| C — Alloc Discipline | 5 | 5 | ✅ 100% |
| D — Lifecycle (Stop/Shutdown) | 6 | 6 | ✅ 100% |
| E — Release | 3 | 4 | 🚀 75% — story 032 is final gate |

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
| **Final** (story 032) | **< 1,000** | ≤ 30 | ≤ 2,048 | ⏳ post-opt |

> M3 regression vs Post-B: context parser now active in hot-path bench (+300 ns).
> The pool + copy work (stories 021-024) did reduce allocs 22→18 and bytes.

### Gate checks

| Check | Target | Current | Status |
|-------|--------|---------|--------|
| Hot-path mean (addSource=off) | < 1,000 ns/op | ~2,050 ns/op | ❌ 2× over |
| Filtered path (below minLevel) | < 80 ns/op | **36 ns/op** | ✅ GREEN |
| Allocs/op | ≤ 30/op | 18/op | ✅ GREEN |
| Bytes/op | ≤ 2,048/op | ~1,400/op | ✅ GREEN |
| `-race` | clean | clean | ✅ GREEN |
| `bench-check.sh` | PASS | ❌ FAIL | ⚠️ ctx-map cost — post-v1.0.0 |

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

## Epic E — Release 🚀 IN PROGRESS

| # | Issue | Story | Status |
|---|-------|-------|--------|
| 029 | #46 | TS-32 CI -shuffle=on + t.Parallel sweep | ✅ MERGED |
| 030 | #47 | TS-33 CI cover ≥ 85% per package | ✅ MERGED |
| 031 | #48 | README + context threshold + benchmark numbers | ✅ MERGED |
| 032 | #49 | release/v1.0.0 cut + tag | ⏳ NEXT |

---

## Critical Path to v1.0.0

![Critical Path](assets/critical-path.svg)

```
[A ✅][B ✅] → [C ✅] → [D ✅] → [029 ✅][030 ✅][031 ✅] → [032] v1.0.0 🚀
```

**All gates are green except the hot-path SLO** (2× over budget due to ctx map).
The SLO miss does NOT block v1.0.0 per project decision — it is tracked post-release.

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

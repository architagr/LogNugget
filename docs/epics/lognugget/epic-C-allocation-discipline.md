# Epic C — Allocation Discipline

Milestone: M3 (2026-06-11 → 2026-06-24).
Owner: Project Lead.
Goal: kill intermediate `[]string` slice; pooled `[]byte` buf on LogEntry; allocations under NF3 ceiling.

Conditional milestone: full body only if M2 closes with allocs/op ≥ 28 (90% of NF3) OR `addSource=on` ≥ 950 ns. Otherwise reduces to docs + tests only.

## In scope

| Group | Refs |
|---|---|
| LogEntry.buf pooled []byte | F27, D-6, D-19, ARCH-8 |
| AppendField / direct render | F27 |
| GenerateInitialPool sizing test | F4 |
| LogEvent.Data copy semantics | ARCH-7 |
| Pool reset hardening | F26 (already in A; this confirms with new fields) |

## Out of scope

- fieldSlicePool, ctxMapBufPool (only land if NF3 STILL trips after this epic; treat as v1 stretch).
- Lifecycle Stop (Epic D).

## Exit criteria

1. `LogEntry` has `buf []byte` cap 1 KB initial; reset shrinks to `buf[:0]`.
2. `entry.Log` renders directly into `buf` via `config.AppendField`; no per-call `make([]string,...)`.
3. `LogEvent.Data` is fresh copy via `append([]byte(nil), buf...)`; pooled buf returned in same `Log` call.
4. `Benchmark_Log/_addSourceOff_fields16` allocs/op ≤ 12; bytes/op ≤ 900.
5. `entry.GenerateInitialPool(n)` covered: `Test_GenerateInitialPool_PrePopulatesN`.
6. Bench gate green; `Benchmark_Log_Parallel` ≤ +20% over serial (R3 trigger).

## Stories

| Story | Refs |
|---|---|
| 021 | D-6 / F27 LogEntry pooled []byte buf |
| 022 | D-19 drop ctx-data index arithmetic |
| 023 | F4 GenerateInitialPool test |
| 024 | ARCH-7 LogEvent.Data copy semantics |

## Bench gate notes

- Targets: LLD §3.3 table — addSource off `~10 allocs / ~800 B`; on `~12 allocs / ~900 B`.
- R3 escalation: if parallel bench regresses > 10% vs serial, ARCH-9 per-P slab work moves to v1.x (not v1).
- Project Lead approves baseline update at epic close (one update per milestone max).

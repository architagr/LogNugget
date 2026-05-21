# P1 — Atomic minLevel + single config snapshot

Owner-role: engineer.
Issue: [#82](https://github.com/architagr/LogNugget/issues/82).
Branch: `feat/82-p1-atomic-min-level` (cut from `feat/81-v2-performance`, PRs back to `feat/81-v2-performance`). Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`) because git rejects nested refs when the parent ref exists. All story PRs target `feat/81-v2-performance`.
Depends: none. P1 is the foundation; P2, P3, P4 must not start until P1 is merged.
LOC est: 180.

## Summary

Replace the 6× `configMu.RLock` acquisitions per hot-path call with one atomic load for the level gate and a single `configMu.RLock` snapshot (`HotSnapshot`) for the full field-render path. The filtered path (level below minimum) drops to 0 locks and ~5 ns.

## Files touched

- `config/config.go` — add `atomicMinLevel atomic.Int32`, `GetAtomicMinLevel()`, `HotSnapshot` struct, `GetHotSnapshot()`, `ContextFieldsAppender` type + `SetContextFieldsAppender()`, dual-write in `SetMinLevel` and `resetConfig`, `contextAppender` field on `Config`
- `entry/entry.go` — replace level-gate call and multi-RLock reads with `GetAtomicMinLevel()` + `GetHotSnapshot()`; pass `snap` through `logWithSkip`
- NEW `config/atomic_snapshot_bench_test.go` — `BenchmarkLogEntry_FilteredPath`, `BenchmarkLogEntry_HotPath_NoCtx`

## Tests-first gate

1. `Test_AtomicMinLevel_ConsistentWithConfig` — after `SetMinLevel(LevelWarn)`, `GetAtomicMinLevel()` returns `LevelWarn` without holding `configMu`.
2. `Test_AtomicMinLevel_RaceWithSetMinLevel` — 8 goroutines calling `SetMinLevel` + `GetAtomicMinLevel` concurrently under `-race`; no data race detected.
3. `Test_HotSnapshot_Fields` — `GetHotSnapshot().TimeFormat` matches `GetConfig().TimeFormat`; `snap.EncoderOpen` matches `snap.Encoder.OpenBytes()`.
4. `Test_LogEntry_FilteredPath_ZeroAlloc` — a log call at a level below `minLevel` triggers 0 allocations (use `testing.AllocsPerRun`).
5. `Test_SetContextFieldsAppender_Stored` — after `SetContextFieldsAppender(fn)`, `GetHotSnapshot().ContextAppender != nil`.

## Acceptance criteria

1. Filtered path (`level < minLevel`): 0 allocs, 0 `configMu` acquisitions — verified by `Test_LogEntry_FilteredPath_ZeroAlloc` passing and `BenchmarkLogEntry_FilteredPath` reporting `0 allocs/op`.
2. Hot path: exactly 1 `configMu.RLock` acquisition per log call — `GetHotSnapshot()` is the single acquisition site; no other RLock call remains in `logWithSkip`.
3. `atomicMinLevel` is always consistent with `defaultConfig.minLevel`: both written atomically under `configMu.Lock()` in `SetMinLevel` and `resetConfig`.
4. `ContextFieldsAppender` type alias defined; `SetContextFieldsAppender` exported; `Config.contextAppender` field added.
5. `BenchmarkLogEntry_FilteredPath`: ≤ 35 ns/op, 0 allocs/op — bench-check gate green.
6. `go test -race ./...` clean.
7. All existing tests in `config/` and `entry/` pass without modification.

## Bench notes

- `BenchmarkLogEntry_FilteredPath` is the regression guard for the filtered path. It must not exceed 35 ns/op at any point in the story PR review cycle.
- `BenchmarkLogEntry_HotPath_NoCtx` (no context fields, no static fields) establishes the post-P1 baseline for P2–P4 to beat.
- `b.ReportAllocs()` required in both benchmarks.

## Assignment

Assigned to: agent-engineer
Branch: feat/82-p1-atomic-min-level
Target PR: feat/81-v2-performance
Date: 2026-05-21
PL instruction: Write tests FIRST on feat/82-p1-atomic-min-level. Present test suite before writing any implementation code. Tests must cover:

- Filtered path 0 allocs 0 locks (AllocsPerRun)
- GetAtomicMinLevel() returns correct level after SetMinLevel
- GetHotSnapshot() returns all hot-path fields under single RLock
- GetHotSnapshot().RestrictedFields includes default reserved keys
Tests must pass `go test -tags testing -run TestXxx ./config/ ./entry/` before any implementation.

# P1 — atomic.Bool Pre-Processor Gate

Owner-role: engineer.
Issue: [#112](https://github.com/architagr/LogNugget/issues/112).
Branch: `feat/112-p1-atomic-preprocessor-gate` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: none. P1 is independent; start immediately.
LOC est: 80.

## Summary

Replace `HasEventPreProcessors()` — which currently acquires `configMu.RLock()` on every hot-path call — with an `atomic.Bool` load. The three mutators (`InitPreProcessors`, `AddPreProcessors`, `RemovePreProcessor`) update the atomic under the existing `configMu.Lock()` so the invariant is always consistent.

**Saving:** ~80 ns per log event (one RLock eliminated from the hot path before any allocation runs).

## Files touched

- `config/config.go` — add `hasPreProcessorsAtomic atomic.Bool` var; rewrite `HasEventPreProcessors()`; update `InitPreProcessors`, `AddPreProcessors`, `RemovePreProcessor`
- NEW `config/atomic_preprocessor_bench_test.go` — `BenchmarkHasEventPreProcessors`, race test

## Tests-first gate

1. `Test_HasPreProcessors_TrueAfterInit` — after `InitPreProcessors(fakeProc)`, `HasEventPreProcessors()` returns `true`.
2. `Test_HasPreProcessors_FalseAfterRemove` — after `InitPreProcessors(fakeProc)` then `RemovePreProcessor(name)`, returns `false`.
3. `Test_HasPreProcessors_FalseOnEmpty` — after `InitPreProcessors()` with no args, returns `false`.
4. `Test_HasPreProcessors_Race` — 4 goroutines calling `AddPreProcessors`/`RemovePreProcessor` concurrently with 4 goroutines calling `HasEventPreProcessors()`, under `-race`; no race detected.
5. `BenchmarkHasEventPreProcessors` — ≤ 5 ns/op, 0 allocs/op (verify atomic load is fast).

## Acceptance criteria

1. `HasEventPreProcessors()` acquires `configMu` zero times — no `RLock` call present in the function body.
2. `hasPreProcessorsAtomic` is updated in all three mutator paths: `InitPreProcessors`, `AddPreProcessors`, `RemovePreProcessor`.
3. `Test_HasPreProcessors_Race` passes under `go test -race`.
4. `BenchmarkHasEventPreProcessors`: ≤ 5 ns/op, 0 allocs/op.
5. All existing `config/` and `entry/` tests pass without modification.

## Assignment

Assigned to: agent-engineer
Branch: feat/112-p1-atomic-preprocessor-gate
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: Write tests FIRST. Present test suite before any implementation. Confirm 0 allocs/op on `BenchmarkHasEventPreProcessors` before marking PR ready.

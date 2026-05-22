# P3 — Atomic Channel Pointer in PublishLog

Owner-role: engineer.
Issue: [#114](https://github.com/architagr/LogNugget/issues/114).
Branch: `feat/114-p3-atomic-channel-pointer` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: P2 merged to feat/111-v3-performance.
LOC est: 50.

## Summary

`PublishLog` currently acquires `configMu.RLock()` solely to snapshot the `ch` channel pointer before sending. Replace this with an `atomic.Value` that stores the current channel; `resetConfig` writes the new channel into both `ch` (existing) and `atomicCh` under `configMu.Lock()`; `PublishLog` loads from `atomicCh` without any lock.

**Saving:** ~50 ns per log event (third and final RLock eliminated from the hot path).

After P1 + P2 + P3 are merged, the hot path has **zero** `configMu.RLock` acquisitions. Every lock is write-only (Set* mutators, resetConfig).

## Files touched

- `config/config.go`:
  - Add `var atomicCh atomic.Value` (stores `chan LogEvent`)
  - Update `resetConfig` to call `atomicCh.Store(newCh)` after assigning `ch = newCh`
  - Rewrite `PublishLog` to use `atomicCh.Load().(chan LogEvent)` instead of `configMu.RLock()`
- NEW `config/atomic_channel_bench_test.go` — `BenchmarkPublishLog`

Note: `atomic.Value` requires that all `Store` calls use the same concrete type (`chan LogEvent`). This is guaranteed because `ch` is always `chan LogEvent`. The first `Store` call must happen before the first `Load` call; `init()` + `resetConfig` ensures this.

## Tests-first gate

1. `Test_PublishLog_UsesAtomicChannel` — after `resetConfig`, a call to `PublishLog(LevelInfo, []byte("x"))` does not panic and does not acquire `configMu` (verified by checking that a concurrent `configMu.Lock()` does not block `PublishLog`).
2. `Test_PublishLog_Race` — 8 goroutines calling `PublishLog` concurrently with 1 goroutine calling `resetConfig`, under `-race`; no race detected.
3. `BenchmarkPublishLog` — publish throughput measured; baseline comparison with V2.

## Acceptance criteria

1. `PublishLog` body contains no `configMu.RLock()` call.
2. `atomicCh.Store(newCh)` is called in `resetConfig` immediately after `ch = newCh`, still under `configMu.Lock()`.
3. `Test_PublishLog_Race` passes under `go test -race`.
4. All existing integration tests (drain-on-stop, pipeline fan-out) pass without modification.
5. `go test ./...` green.

## Assignment

Assigned to: agent-engineer
Branch: feat/114-p3-atomic-channel-pointer
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: Write tests FIRST. Note that P9 (ring buffer) will later replace the channel entirely — P3's atomic.Value will then store a reference to the ring buffer instead. Design P3 so that swapping the stored type in P9 requires only a change to `PublishLog` and `resetConfig`, not a new interface.

# P5 — Channel capacity 1000

Owner-role: engineer.
Issue: [#86](https://github.com/architagr/LogNugget/issues/86).
Branch: `feat/86-p5-channel-capacity` (cut from `feat/81-v2-performance`, PRs back to `feat/81-v2-performance`). Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`) because git rejects nested refs when the parent ref exists. All story PRs target `feat/81-v2-performance`.
Depends: none. P5 is independent; it can be merged before or after P1–P4.
LOC est: 120.

## Summary

Raise the dispatch channel buffer from 10 to 1000 and expose `SetChannelCapacity(n int)` so callers can tune the value before `init()` fires. Update `DafaultLogBuffer` constant to `1000`. Introduce a package-level `channelCapacity` variable consumed by `resetConfig`. Add argument clamping with `log.Printf` warnings for out-of-range values.

## Files touched

- `config/config.go` — add `channelCapacity int = 1000` package var; update `DafaultLogBuffer` from `20` to `1000`; add `SetChannelCapacity(n int)`; update `resetConfig` to `make(chan LogEvent, channelCapacity)`
- NEW `entry/parallel_bench_test.go` — `BenchmarkLognugget_Parallel_10CtxFields` (GOMAXPROCS=8)
- NEW `config/channel_capacity_test.go` — clamping, warning, and default-value tests

## Tests-first gate

1. `Test_SetChannelCapacity_DefaultIs1000` — without calling `SetChannelCapacity`, the channel created by `resetConfig` has capacity 1000 (inspect via `cap(ch)`).
2. `Test_SetChannelCapacity_ZeroClamped` — `SetChannelCapacity(0)` clamps to 1000; channel created by subsequent `resetConfig` has capacity 1000; `log.Printf` warning is emitted.
3. `Test_SetChannelCapacity_NegativeClamped` — `SetChannelCapacity(-5)` clamps to 1000.
4. `Test_SetChannelCapacity_OverMaxClamped` — `SetChannelCapacity(200_000)` clamps to 100_000; channel has capacity 100_000; warning emitted.
5. `Test_SetChannelCapacity_ValidValue` — `SetChannelCapacity(5000)` is stored; channel has capacity 5000; no warning.
6. `Test_DafaultLogBuffer_Value` — constant `DafaultLogBuffer` equals `1000` (compile-time constant check).

## Acceptance criteria

1. `DafaultLogBuffer = 1000` — package constant updated; existing callers that read this constant (e.g., documentation, tests) see the new value.
2. `make(chan LogEvent, channelCapacity)` in `resetConfig` — channel buffer matches `channelCapacity` at construction time.
3. `SetChannelCapacity(0)` and `SetChannelCapacity(-5)` clamp to 1000; `log.Printf` warning emitted (verified by capturing `log` output in test).
4. `SetChannelCapacity(200_000)` clamps to 100_000; warning emitted.
5. `BenchmarkLognugget_Parallel_10CtxFields` at GOMAXPROCS=8: ≤ 1,000 ns/op, ≥ 3 M ops/sec — no goroutine blocking due to channel backpressure at this concurrency level.
6. No `configMu` acquisition in `SetChannelCapacity`; the variable is written before `init()` fires (Go init-ordering guarantee) and read inside `resetConfig` at `init()` time.
7. `go test -race ./...` clean.
8. All existing `config/` tests pass without modification.

## Bench notes

- `b.ReportAllocs()` required in `BenchmarkLognugget_Parallel_10CtxFields`.
- Run with `GOMAXPROCS=8` (via `runtime.GOMAXPROCS(8)` at bench start or `-cpu 8` flag) to expose channel contention.
- The parallel bench is the final integration gate for the full V2 stack (all 5 stories combined); at ≥ 3 M ops/sec the < 1 µs SLO is met.
- This story contributes the parallel bench file; the bench gate target is only reachable once P1–P4 are also merged.

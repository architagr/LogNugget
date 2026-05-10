# 039 — Fix #54: config.ResetConfig data race

Owner-role: engineer.
Refs: #54, NF7, NF9, SC6.
Depends: 006 (ResetConfig moves to test-only helper). Wave 2.5 — AFTER 006 lands to avoid `config/` collision.
LOC est: ~200.
Status: PENDING.

## Why

`-race` fails module-wide. Pre-existing on `feat/12-lognugget-v1`. Surfaces every time tests use `support.ConfigBuilder`. Three globals unprotected: `defaultConfig`, `ch`, `EventPreProcessors`. `go ProcessLogEvent()` reads them while `Set*` writes them.

## Files touched

- `config/config.go` — guard globals.
- `config/config_race_test.go` — NEW. Race-only test.

## Tests-first gate

- `Test_Race_ResetConfig_UnderConcurrentSet` — N goroutines call `SetMinLevel`/`SetEncoderType` while M call `ResetConfig`. No race report. Final state coherent.
- Existing `config/*_test.go` continue to pass under `-race`.

## Acceptance

1. `go test -race ./...` GREEN module-wide.
2. New race test passes 100 iterations under `-race -count=100`.
3. Bench gate green: `Benchmark_Log` < 1,000 ns/op (NF1 unviolated by lock choice).
4. Engineer picks Option A (RWMutex) or Option B (atomic.Pointer); justify in PR body.

## Coordination

- Story 006 collapses `config.ResetConfig` to test-only helper. Run 039 AFTER 006 merges to feature branch. Otherwise both touch `config.ResetConfig` and conflict.

## Bench notes

Add lock on read path is hostile to NF1. Engineer must benchstat before/after. If RWMutex regresses, swap to atomic.Pointer.

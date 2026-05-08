# 013 — Fix D-1: F17 source capture wiring

Owner-role: engineer.
Refs: D-1, F17, SC3, ARCH-3, T-3, TS-06.
Depends: 012.
LOC est: 200.

## Files touched

- `entry/entry.go` (lazy `runtime.Caller(skip)` when `cfg.addSource`; populate `caller`)
- NEW `entry/source_capture_test.go` (TS-06)

## Tests-first gate

1. `Test_LogEntry_CallerCapture_WhenAddSource_True` — emitted JSON has `caller` field with non-empty function name.
2. `Test_LogEntry_NoCallerWhenAddSourceFalse` — emitted JSON has no `caller` field (T-3 fix: paired test).
3. `Test_LogEntry_CallerOnUnknown` — when `runtime.Caller` returns ok=false, field = `"unknown"` (LLD §10).
4. SC3 mapped: assertion via `json.Unmarshal`.

## Acceptance

1. `runtime.Caller(skip)` (single frame, ARCH-3); not `runtime.Callers + CallersFrames`.
2. `skip` value verified by test against known caller name.
3. TODO at `entry.go:29` removed.
4. SC3 green; T-3 closed.

## Bench notes

- Stage 5 budget: 250 ns when on / 0 ns when off.
- Add `Benchmark_Log_AddSourceTrue` sub-bench (lands with story 037; placeholder added here).
- Watch ARCH-8 trigger: if `addSource=on,fields=16` ≥ 950 ns at this point, escalate to Project Lead.

# 012 — Fix D-7 + D-13: addSource default off + level-gate ordering

Owner-role: engineer.
Refs: D-7, D-13, ARCH-4, F6, R4, TS-05.
Depends: 001, 010.
LOC est: 160.

## Files touched

- `config/config.go` (`DafaultAddSource = false`; rename typo to `DefaultAddSource`)
- `entry/entry.go` (level gate ordering; nil-check after gate)
- NEW `entry/level_gate_test.go` (TS-05 F6 table)

## Tests-first gate

1. `Test_LogEntry_MinLevelGate` — every (event-level × min-level) pair; hook called iff level ≥ min.
2. `Test_LogEntry_FilteredPath_ZeroAlloc` — `testing.AllocsPerRun(100, ...)` for filtered call: 0 allocs.
3. `Test_DefaultAddSource_False` — `config.AddSource() == false` at zero-config.

## Acceptance

1. Default `addSource = false`.
2. Const renamed (typo `Dafault` → `Default`); existing callers updated.
3. `Log` ordering: level gate FIRST, then `EventPreProcessors == nil` check, then everything else.
4. Filtered path < 100 ns / 0 allocs (NF1 / F6).
5. SC1 still green: zero-config emits valid JSON without `caller`.

## Bench notes

- Hot-path delta: addSource=on path no longer hit by default; baseline benches (which run defaults) drop ~250 ns immediately.
- New `Benchmark_LogFiltered` (in 037) gates the < 100 ns ceiling.

# 023 — F4: GenerateInitialPool test + sizing rec

Owner-role: engineer.
Refs: F4, TS-08, F3, F25.
Depends: 021.
LOC est: 200.

## Files touched

- `entry/pool.go` (extract `GenerateInitialPool`; doc with sizing rec `GOMAXPROCS * 64`)
- NEW `entry/pool_test.go` (TS-08)

## Tests-first gate

1. `Test_NewLogEntry_ReturnsResetEntry` — fresh Get yields zero-value (post-reset).
2. `Test_GenerateInitialPool_PrePopulatesN` — after `GenerateInitialPool(n)`, N consecutive Gets observe non-cold entries (proxy: cap of buf >= initial cap).
3. `Test_LogEntry_Put_ReturnsToPool` — Put then Get returns the same instance (best-effort; sync.Pool weak guarantee, accept flake-tolerant assertion via repeated Get).
4. `Test_GenerateInitialPool_ZeroSafeWithN_0` — `GenerateInitialPool(0)` no-op.

## Acceptance

1. `GenerateInitialPool(n int)` doc string includes sizing rec `GOMAXPROCS * 64`.
2. Tests cover F3, F4, F25.
3. README addition deferred to story 031 (Epic E).

## Bench notes

- `Benchmark_Pool_GetPut` micro-bench < 30 ns (informational, not gate).

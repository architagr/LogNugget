# 037 — TS-30: parallel + filtered + json-escape benches; M3 baseline

Owner-role: engineer (bench file edits) + Project Lead (baseline update).
Refs: TS-30, R3, NF1, NF3, ARCH-9.
Depends: 021, 022, 024.
LOC est: 260.

## Files touched

- NEW `test/benchmark/log_parallel_bench_test.go` (Benchmark_LogParallel; b.RunParallel)
- NEW `test/benchmark/log_filtered_bench_test.go` (Benchmark_LogFiltered; F6 < 100 ns gate)
- NEW `test/benchmark/json_escape_bench_test.go` (Benchmark_JSONEncoder_Escape; per-byte loop)
- `test/benchmark/entry_benchmark_test.go` (sub-bench matrix per LLD test-framework §9: addSource × fields × encoder)
- `bench-baseline.txt` (Project Lead updates)

## Tests-first gate

- N/A; bench files. Project Lead reviews benchstat output before baseline update.

## Acceptance

1. `Benchmark_Log` exposes 5 sub-benches:
   - `addSource=off,fields=1` < 700 ns / ≤ 8 allocs / ≤ 800 B
   - `addSource=off,fields=8` < 850 ns / ≤ 12 / ≤ 1200 B
   - `addSource=off,fields=16` < 1000 ns / ≤ 30 / ≤ 2048 B (NF3)
   - `addSource=on,fields=16` < 1000 ns / ≤ 32 / ≤ 2200 B
   - `encoder=text,fields=8` < 800 ns / ≤ 10 / ≤ 1000 B
2. `Benchmark_LogFiltered` < 100 ns / 0 allocs.
3. `Benchmark_LogParallel` ≤ +20% serial (R3 trigger).
4. `Benchmark_JSONEncoder_Escape` < 100 ns / 100 chars.
5. M3 baseline regenerated; commit references "M3 baseline post-allocation-discipline".

## Bench notes

- THIS is the second baseline mutation in v1 (after 010). Project Lead only.
- If R3 trips (parallel > +10% over serial), escalate ARCH-9 (per-P slab) — NOT part of v1; document as v1.x candidate.

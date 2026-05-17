# 021 — Fix D-6 + F27: LogEntry pooled []byte buf

Owner-role: engineer.
Refs: D-6, F27, ARCH-8, NF3, TS-09.
Depends: 017, 018.
LOC est: 280.

## Files touched

- `entry/entry.go` (add `buf []byte`; render directly into buf via AppendField; size 1 KB initial)
- `entry/init.go` (or inline) `initLogEntry()` returns entry with pre-grown buf
- NEW `config/append_field_test.go` (TS-09 alloc-free)

## Tests-first gate

1. `Test_AppendField_NoAllocOnGrownBuf` — `testing.AllocsPerRun(100, func(){ buf=AppendField(buf[:0], k, v) }) == 0`.
2. `Test_LogEntry_BufRetainedAcrossPoolCycle` — Get/Put/Get returns same backing array (cap preserved).
3. `Test_LogEntry_BufResetSizeZero` — `reset()` sets `buf=buf[:0]`; len=0; cap unchanged.
4. `Benchmark_Log/_addSourceOff_fields16` — allocs/op ≤ 12 (per LLD §3.3).

## Acceptance

1. `LogEntry.buf` is a `[]byte` cap 1 KB initial.
2. Hot path: `e.buf = e.buf[:0]; e.buf = config.AppendField(e.buf, k, v)`; no per-call slice make.
3. D-6 capacity bug eliminated: no `[]string` aggregator anywhere in `Log`.
4. NF3 ceiling met after this story (allocs/op ≤ 30 — should now be ≤ 12).

## Bench notes

- Largest single hot-path saving: ~150 ns + ≥ 5 allocs (LLD §3.1 step 2).
- Stage 6 budget achieved.
- ARCH-8 outcome read here: if NF3 still trips, Project Lead pulls fieldSlicePool / ctxMapBufPool stories.

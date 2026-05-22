# P8 — Exact-Size Buffer Alias Severance

Owner-role: engineer.
Issue: [#119](https://github.com/architagr/LogNugget/issues/119).
Branch: `feat/119-p8-exact-buffer-size` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: none. Independent; may be worked in parallel with P1, P4, P5, P6.
LOC est: 30.

## Summary

After `logWithSkip` appends the closing bytes to `e.buf` and calls `config.PublishLog(level, data)`, it severs the alias with:

```go
e.buf = make([]byte, 0, initBufCap)  // always 1024 B
```

This allocates a new 1024 B backing array regardless of the actual log line length (~200 B for a typical structured log entry). Over time the pool stabilises at 1024 B/slot — far more than needed.

Replace `initBufCap` with `len(data)` (floored at 64 to avoid pathologically small allocations):

```go
newCap := len(data)
if newCap < 64 {
    newCap = 64
}
e.buf = make([]byte, 0, newCap)
```

Over time the pool slots naturally grow to the actual typical line size. First reuse of a larger line causes one realloc; subsequent reuses are allocation-free. For typical 200 B lines, this saves ~800 B/call.

**Saving:** ~800 B/call (bytes/op reduction); small ns impact (~5 ns from reduced GC pressure).

## Files touched

- `entry/entry.go` — replace 1 line in `logWithSkip` (alias severance section, currently line ~240)
- `entry/pool.go` — verify `initBufCap` constant is no longer used post-change (may keep for `GenerateInitialPool`); leave the constant but note it applies only to initial pool population
- NEW `entry/buffer_size_bench_test.go` — `BenchmarkBufferSeverance_ExactSize` vs `BenchmarkBufferSeverance_ConstSize`

## Tests-first gate

1. `Test_LogEntry_BufferSizeAfterSeverance` — after a log call, the pool slot's internal buffer capacity equals `max(64, len_of_last_log_line)` (test via internal access or by measuring allocs on second call).
2. `Test_LogEntry_LargeMessage_NoExtraAlloc` — log a 512 B message; the next log call of similar size allocates 0 extra bytes (pool buf has grown to accommodate).
3. `BenchmarkBufferSeverance_ExactSize` — verify B/op ≤ 300 (down from ~1,411 in V2 baseline).

## Acceptance criteria

1. `e.buf = make([]byte, 0, initBufCap)` replaced with `newCap`-based make.
2. `newCap` is `max(64, len(data))`.
3. `BenchmarkBufferSeverance_ExactSize`: B/op significantly lower than V2 baseline.
4. Filtered path allocs: 0 (no regression).
5. `go test ./...` green.

## Assignment

Assigned to: agent-engineer
Branch: feat/119-p8-exact-buffer-size
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: 3-line change. Write the benchmark first to establish B/op baseline, then make the change and confirm B/op drops.

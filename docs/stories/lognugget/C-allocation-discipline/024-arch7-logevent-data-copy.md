# 024 — ARCH-7: LogEvent.Data copy semantics

Owner-role: engineer.
Refs: ARCH-7, F21, NF3.
Depends: 021.
LOC est: 180.

## Files touched

- `config/publish.go` (`PublishLog` copies pooled buf into fresh `[]byte` before `ch <- LogEvent{...}`)
- `entry/entry.go` (`LogEntry.Put()` returns pooled buf safely)
- NEW `config/publish_test.go` (race + aliasing test)

## Tests-first gate

1. `Test_LogEvent_DataNotAliasedToPool` — log entry; mutate pooled buf after Put; assert dispatcher saw original bytes.
2. `Test_PublishLog_AllocsOneCopy` — `testing.AllocsPerRun` for full Log call ≤ 12 (cooperates with 021).
3. `Test_Race_PublishLogConcurrent` — 100 goroutines × 100 publishes; `-race` clean; total seen == 10_000.

## Acceptance

1. `LogEvent.Data` is `append([]byte(nil), buf...)` — fresh allocation; pooled buf stays with LogEntry.
2. No aliasing across goroutine boundary; -race green.
3. NF3 alloc accounting documents the copy as 1 alloc/event (~600 B).

## Bench notes

- Trades 1 alloc + ~50 ns for safety; this is the design point per LLD §8.4.
- Net allocs: ~12 (per LLD §3.3 addSource off table).

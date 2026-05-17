# 011 — Fix D-4: RFC3339 default time format

Owner-role: engineer.
Refs: D-4, F28, TS-23.
Depends: 001, 010 (M1 baseline).
LOC est: 140.

## Files touched

- `config/config.go` (`DefaultTimeFormat = time.RFC3339`)
- `custom_time/time.go` (add `AppendFormat(dst, t, layout) []byte`)
- NEW `custom_time/time_test.go` (TS-23: UTC + AppendFormat alloc-free)

## Tests-first gate

1. `Test_TimeNow_UTC` — returned `time.Time` is UTC.
2. `Test_AppendFormat_NoAlloc` — `testing.AllocsPerRun(100, ...) == 0`.
3. `Test_DefaultTimeFormat_RFC3339` — string match.

## Acceptance

1. `DefaultTimeFormat = time.RFC3339`.
2. `customTime.AppendFormat` allocates 0 per call (uses `t.AppendFormat`).
3. Test green; F28 closed.

## Bench notes

- Stage 4 budget: ~80 ns. Today uses `Format` returning `string` (1 alloc + ~40 ns). New `AppendFormat` saves 1 alloc / ~30 ns.
- Bench delta: -30 ns / -1 alloc on hot path.

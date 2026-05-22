# P4 — AppendFormat Direct Timestamp (No String Roundtrip)

Owner-role: engineer.
Issue: [#115](https://github.com/architagr/LogNugget/issues/115).
Branch: `feat/115-p4-direct-timestamp` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: none. Independent; may be worked in parallel with P1, P5, P6, P8.
LOC est: 40.

## Summary

`logWithSkip` currently calls `customTime.Format(now, layout)` which allocates a `string`, then passes it to `config.AppendQuotedString` which calls `[]byte(s)` — a second allocation. Replace both calls with a direct `time.Time.AppendFormat` that writes bytes into `e.buf` in place. Net saving: 2 allocs/event.

**Current code** (`entry/entry.go:179`):
```go
e.buf = config.AppendQuotedString(e.buf, customTime.Format(customTime.TimeNow(), snap.TimeFormat))
```

**Replacement:**
```go
now := customTime.TimeNow()
e.buf = append(e.buf, '"')
e.buf = now.AppendFormat(e.buf, snap.TimeFormat)
e.buf = append(e.buf, '"')
```

RFC3339 output (the default `snap.TimeFormat`) contains only digits, `T`, `Z`, `-`, `:`, `+`, `.` — none require JSON escaping. A comment in the code documents this invariant.

**Saving:** ~40 ns, 2 allocs/event.

## Files touched

- `entry/entry.go` — replace 1 line (logWithSkip timestamp section, currently line 179) with 3 lines
- NEW `entry/timestamp_bench_test.go` — `BenchmarkTimestamp_Direct` vs `BenchmarkTimestamp_ViaFormat`

## Tests-first gate

1. `Test_LogEntry_TimestampFormat_RFC3339` — log a message, capture output, verify the `"time"` field value matches `time.RFC3339` format (existing golden test can cover this).
2. `Test_LogEntry_TimestampFormat_Custom` — set `config.SetTimeFormat("2006-01-02")`, log a message, verify `"time"` value is `"YYYY-MM-DD"` format.
3. `BenchmarkTimestamp_Direct` — using `now.AppendFormat` directly: verify 0 allocs/op.
4. `BenchmarkTimestamp_ViaFormat` — using `customTime.Format` + `AppendQuotedString`: verify 2 allocs/op (baseline for comparison).

## Acceptance criteria

1. `customTime.Format` is no longer called in `logWithSkip`.
2. The timestamp field in log output matches `snap.TimeFormat` exactly.
3. `BenchmarkTimestamp_Direct`: 0 allocs/op.
4. Existing golden tests and JSON output tests pass without modification.
5. `go test ./...` green, `go test -race ./...` green.

## Assignment

Assigned to: agent-engineer
Branch: feat/115-p4-direct-timestamp
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: The change is 3 lines in entry.go. Write tests first to confirm RFC3339 format is preserved and 0 allocs.

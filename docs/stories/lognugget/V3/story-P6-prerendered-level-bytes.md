# P6 — Pre-Rendered Quoted Level Bytes

Owner-role: engineer.
Issue: [#117](https://github.com/architagr/LogNugget/issues/117).
Branch: `feat/117-p6-prerendered-level-bytes` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: none. Independent; may be worked in parallel with P1, P4, P5, P8.
LOC est: 60.

## Summary

`logWithSkip` calls `config.AppendQuotedString(e.buf, level.String())`. The `level.String()` call uses `fmt.Sprintf` for non-named levels (allocating a string) and returns a string even for named levels. `AppendQuotedString` then calls `appendJSONString(dst, []byte(levelStr))` allocating `[]byte`.

Add `config.AppendQuotedLevel(dst []byte, l enum.LogLevel) []byte` that uses a switch over the 5 named levels to append pre-quoted bytes directly. Named levels produce 0 allocations; unknown levels fall back to the existing path.

**Saving:** ~15 ns, 1 alloc/event (for all 5 named levels, which covers ~100% of production use).

## Files touched

- `config/parse_field.go` — add `AppendQuotedLevel(dst []byte, l enum.LogLevel) []byte`
- `entry/entry.go` — replace `config.AppendQuotedString(e.buf, level.String())` with `config.AppendQuotedLevel(e.buf, level)` (logWithSkip line ~182)
- NEW `config/level_bytes_bench_test.go` — `BenchmarkAppendQuotedLevel`

## AppendQuotedLevel implementation

```go
// AppendQuotedLevel appends the JSON-quoted string representation of l to dst.
// For the 5 named levels it appends a constant pre-quoted literal (0 allocs);
// unknown levels fall back to AppendQuotedString(dst, l.String()).
func AppendQuotedLevel(dst []byte, l enum.LogLevel) []byte {
    switch l {
    case enum.LevelDebug:
        return append(dst, `"DEBUG"`...)
    case enum.LevelInfo:
        return append(dst, `"INFO"`...)
    case enum.LevelWarn:
        return append(dst, `"WARN"`...)
    case enum.LevelError:
        return append(dst, `"ERROR"`...)
    case enum.LevelFatal:
        return append(dst, `"FATAL"`...)
    default:
        return AppendQuotedString(dst, l.String())
    }
}
```

## Tests-first gate

1. `Test_AppendQuotedLevel_AllNamedLevels` — table test: for each of the 5 named levels, `AppendQuotedLevel(nil, l)` equals `[]byte(`"DEBUG"`)` etc.
2. `Test_AppendQuotedLevel_UnknownLevel` — a custom level value not in the switch falls back correctly (output is a quoted string, not panic).
3. `Test_LogEntry_LevelField_InOutput` — a log entry at `LevelInfo` produces JSON with `"level":"INFO"` in the output (integration).
4. `BenchmarkAppendQuotedLevel` — ≤ 5 ns/op, 0 allocs/op for named levels.

## Acceptance criteria

1. `config.AppendQuotedLevel` exported and present in `config/parse_field.go`.
2. `entry.logWithSkip` calls `config.AppendQuotedLevel(e.buf, level)` — no `level.String()` call on the hot path.
3. `BenchmarkAppendQuotedLevel`: ≤ 5 ns/op, 0 allocs/op for `LevelInfo`.
4. `Test_AppendQuotedLevel_AllNamedLevels` passes for all 5 levels.
5. All existing JSON output golden tests pass without modification.

## Assignment

Assigned to: agent-engineer
Branch: feat/117-p6-prerendered-level-bytes
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: 20-line change total. Write table test first covering all 5 named levels. Confirm 0 allocs on bench before PR.

# P3 — Append-to-buf context API

Owner-role: engineer.
Issue: [#84](https://github.com/architagr/LogNugget/issues/84).
Branch: `feat/84-p3-ctx-append-buf` (cut from `feat/81-v2-performance` after P1 is merged, PRs back to `feat/81-v2-performance`). Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`) because git rejects nested refs when the parent ref exists. All story PRs target `feat/81-v2-performance`.
Depends: P1 merged. Uses `HotSnapshot.ContextAppender`, `HotSnapshot.ContextParser`, and `HotSnapshot.RestrictedFields` introduced by P1.
LOC est: 170.

## Summary

Replace the `setLogContextFields` method (which allocates `map[string]any` + `[]string`) with `appendContextFields`, a package-level function that writes directly to `e.buf`. When a `ContextFieldsAppender` is registered (set via `SetContextFieldsAppender` from P1), the appender writes to the buffer with 0 intermediate allocations. The legacy `ContextFieldsParser` path retains its allocations but eliminates N extra `configMu.RLock` acquisitions by using the pre-snapshotted `RestrictedFields`.

## Files touched

- `config/parse_field.go` — add `validateAndAppendField(dst []byte, key string, value any, restrictedSet map[string]struct{}) []byte` (internal variant, no RLock)
- `entry/entry.go` — add `appendContextFields(ctx context.Context, buf []byte, snap config.HotSnapshot) []byte`; delete `setLogContextFields`; replace call site in `logWithSkip`
- NEW `entry/ctx_appender_bench_test.go` — `BenchmarkLogEntry_CtxAppender_10`, `BenchmarkLogEntry_CtxParser_10`
- NEW `config/ctx_appender_test.go` — appender priority + nil-ctx + legacy-parser-still-works tests

## Tests-first gate

1. `Test_AppendContextFields_AppenderPriority` — when both `SetContextFieldsAppender(fn)` and `SetContextFieldsParser(fn)` are registered, only the appender is called; parser is not invoked.
2. `Test_AppendContextFields_NilCtx` — `appendContextFields(nil, buf, snap)` returns `buf` unchanged; no panic.
3. `Test_AppendContextFields_AppenderOutput` — appender that writes `,"req_id":"abc"` produces the expected byte sequence in the log line.
4. `Test_AppendContextFields_LegacyParserStillWorks` — `SetContextFieldsParser(fn)` (no appender registered) produces same output as v1.0.0 for identical context data.
5. `Test_ValidateAndAppendField_RestrictedKey` — a key in `restrictedSet` is prefixed with `DefaultPrefix`; no RLock acquired.
6. `Test_SetContextFieldsParser_StillWorks` — `SetContextFieldsParser` registration is not disturbed by this story; existing tests in `entry/ctx_fields_test.go` pass without modification.

## Acceptance criteria

1. `BenchmarkLogEntry_CtxAppender_10` allocs/op ≤ `BenchmarkLog` (v1.0.0 baseline) allocs/op minus 20 — the appender path eliminates at least 20 allocations vs the legacy parser path.
2. `appendContextFields` is a package-level function (not a method on `LogEntry`), independently testable without a pool entry.
3. `setLogContextFields` method deleted from `entry/entry.go`; its only call site is replaced.
4. `validateAndAppendField` in `config/parse_field.go` accepts a pre-snapshotted `restrictedSet`; it does not acquire `configMu`.
5. `SetContextFieldsParser` still works: existing tests in `entry/ctx_fields_test.go` pass without modification.
6. Legacy parser path: N context fields, 0 extra `configMu` acquisitions beyond the one snapshot already held (uses `snap.RestrictedFields`).
7. `go test -race ./...` clean.

## Bench notes

- `b.ReportAllocs()` required in both benchmarks.
- `BenchmarkLogEntry_CtxParser_10` is a regression guard for the legacy path; its alloc count must not increase vs P1/P2 baseline.
- `BenchmarkLogEntry_CtxAppender_10` is the primary gate: it must show ≥ 20 fewer allocs/op than the parser path on the same 10-field workload.

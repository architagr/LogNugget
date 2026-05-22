# P2 — Typed field API (Str/Int/Bool/Float64)

Owner-role: engineer.
Issue: [#83](https://github.com/architagr/LogNugget/issues/83).
Branch: `feat/83-p2-typed-field-api` (cut from `feat/81-v2-performance` after P1 is merged, PRs back to `feat/81-v2-performance`). Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`) because git rejects nested refs when the parent ref exists. All story PRs target `feat/81-v2-performance`.
Depends: P1 merged. Consumes `HotSnapshot` from P1 in the updated field dispatch loop.
LOC est: 220.

## Summary

Introduce a typed union on `model.LogAttr` (`AttrKind` discriminant + unexported value fields) and package-level constructors (`model.Str`, `model.Int`, `model.Uint`, `model.Float64`, `model.Bool`). Add `AppendAttr` to `config/parse_field.go` for zero-alloc kind-based dispatch. Add matching methods on `LogEntry`. The zero-value `LogAttr{Key:"k", Value:v}` struct literal continues to work via `KindAny`.

## Files touched

- `model/attr.go` — add `AttrKind uint8`, constants `KindAny=0..KindBool=5`, unexported value fields (`strVal`, `intVal`, `uintVal`, `floatVal`, `boolVal`), kind accessor, typed value accessors, constructors `Str/Int/Uint/Float64/Bool`
- `config/parse_field.go` — add `AppendAttr(dst []byte, attr model.LogAttr) []byte`
- `entry/entry.go` — add `Str/Int/Uint/Bool/Float64` methods on `*LogEntry`; update field dispatch loop in `logWithSkip` to fast-path `Kind != KindAny`
- NEW `entry/typed_fields_bench_test.go` — `BenchmarkLogEntry_TypedFields_10`, `BenchmarkLogEntry_LegacyAttrs_10`
- NEW `model/attr_test.go` (if not already present) — constructor and zero-value tests

## Tests-first gate

1. `Test_LogAttr_KindAny_ZeroValue` — `model.LogAttr{Key:"k", Value:"v"}` has `Kind == KindAny`; `AppendAttr` delegates to `AppendField`; output identical to v1.0.0.
2. `Test_AppendAttr_Str` — `AppendAttr(dst, model.Str("msg","hello"))` produces `"msg":"hello"` with 0 allocs (`testing.AllocsPerRun`).
3. `Test_AppendAttr_Int` — `AppendAttr(dst, model.Int("n", 99))` produces `"n":99` with 0 allocs.
4. `Test_AppendAttr_Bool` — `AppendAttr(dst, model.Bool("ok", true))` produces `"ok":true` with 0 allocs.
5. `Test_AppendAttr_Float64_NaN` — `AppendAttr(dst, model.Float64("f", math.NaN()))` produces `"f":null`.
6. `Test_LogEntry_Str_ChainReturn` — `entry.Str("k","v")` returns `*LogEntry`; chaining `.Int("n",1)` compiles and appends both fields.
7. `Test_BackwardCompat_LogAttrStructLiteral` — existing call site `log.Info(ctx, "msg", model.LogAttr{Key:"k", Value:"v"})` produces identical JSON output before and after this story.

## Acceptance criteria

1. `model.Int("k", 99)` allocates 0 bytes (`testing.AllocsPerRun` returns 0).
2. `AppendAttr` for `KindStr`, `KindInt`, `KindBool`, `KindFloat64`, `KindUint` each allocate 0 bytes.
3. Backward compatibility: `LogAttr{Key:"k", Value:"v"}` (zero `Kind`) still works; output is byte-identical to v1.0.0 for the `KindAny` path.
4. `BenchmarkLogEntry_TypedFields_10` (10 typed fields via `e.Str/e.Int`): 0 allocs/op for the field-append work.
5. `BenchmarkLogEntry_LegacyAttrs_10` (10 `KindAny` attrs): alloc count unchanged vs P1 baseline.
6. `go test -race ./...` clean.
7. All existing tests in `model/`, `config/`, and `entry/` pass without modification.

## Bench notes

- `b.ReportAllocs()` required in both benchmarks.
- The `BenchmarkLogEntry_TypedFields_10` target is 0 allocs/op for the field-append segment. Full log-call alloc count may still be > 0 from mandatory fields (P4 addresses the remaining sources).
- `BenchmarkLogEntry_LegacyAttrs_10` is a regression guard: it must not exceed the P1-baseline alloc count.

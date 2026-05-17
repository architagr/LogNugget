# 017 — Fix D-5: strconv-based ParseLogField

Owner-role: engineer.
Refs: D-5, F10, NF1, TS-10.
Depends: 016.
LOC est: 260.

## Files touched

- `config/parse_field.go` (rewrite `ParseLogField` → `AppendField(dst, key, value) []byte`)
- `config/config.go` (legacy `ParseLogField` kept as thin wrapper for any remaining caller; deprecated comment)
- NEW `config/parse_log_field_test.go` (TS-10 every type)

## Tests-first gate

1. `Test_AppendField_Types` — table: string, int8/16/32/64, uint*, float32/64, bool, error, time.Time, slice (defensive: documented slow path), nested map (slow path).
2. `Test_AppendField_NoAlloc_Numerics` — `testing.AllocsPerRun(100, ...) == 0` for int/float values.
3. `Test_AppendField_NumericsUnquoted` — int `42` rendered as `"key":42` not `"key":"42"`.
4. `Test_AppendField_NaNInf` — `math.NaN()`, `math.Inf(1)` → `"key":null`.
5. `Test_AppendField_StringEscape` — uses 016 table; embedded quote → `\"`.

## Acceptance

1. No `fmt.Sprintf` on hot path for primitives.
2. `strconv.AppendInt/AppendFloat/AppendBool` for numerics/bool.
3. Numerics unquoted (cooperates with F10).
4. `AppendField` writes into caller's `dst` slice; returns extended slice.
5. Slow path for `any` types of unknown shape uses `fmt.Append("%+v", ...)` — documented and bench-callout.

## Bench notes

- Removes 200+ ns / multiple allocs per int field.
- Stage 6 (render fields) target: ~250 ns total for 16 fields. Today >2000 ns.
- Significant net saving here: -300+ ns and -10+ allocs.

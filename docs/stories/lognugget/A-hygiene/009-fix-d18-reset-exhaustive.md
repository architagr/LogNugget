# 009 — Fix D-18 + T-2 + T-4: reset() exhaustive + entry tests refactor

Owner-role: engineer.
Refs: D-18, T-2, T-4, F26, F2.
Depends: 001.
LOC est: 280.

## Files touched

- `entry/entry.go` (reset())
- DELETE `entry/entry_test.go` (legacy)
- NEW `entry/levels_test.go` (TS-04 table-driven, json-unmarshal asserts per T-2)
- NEW `entry/reset_test.go` (TS-07 reflect-exhaustive)

## Tests-first gate

1. `Test_LogEntry_ResetExhaustive` — uses `reflect.Value.IsZero()` over every field after reset.
2. `Test_LogEntry_LevelMethods` — table-driven (Debug/Info/Warn/Error/Fatal/Panic); assertions via `json.Unmarshal` not string-contains (T-2 fix).
3. `Test_LogEntry_ErrorAcceptsErr` — Error/Fatal/Panic accept `error`; Debug/Info/Warn do not (compile-time).
4. Tests `t.Parallel()` after singleton reset via support.

## Acceptance

1. `reset()` zeroes every per-call field; future fields (buf, level, err) auto-covered by reflect test.
2. Legacy `entry_test.go` removed; replaced by table-driven tests.
3. Assertions use `json.Unmarshal`; no coupling to current `ParseLogField` shape.
4. All level methods covered; `Fatal` deferred / `Panic` recovered in test harness so test process survives.
5. T-2 + T-4 closed.

## Bench notes

- `reset()` may grow slightly (more fields zeroed). Budget: < 5 ns over current. Hot-path stage 2 budget remains ~30 ns.

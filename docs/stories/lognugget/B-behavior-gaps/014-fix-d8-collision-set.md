# 014 — Fix D-8: collision-set map + SC7 integration

Owner-role: engineer.
Refs: D-8, F14, SC7, TS-12, TS-28.
Depends: 001.
LOC est: 280.

## Files touched

- `config/config.go` (`restrictedFields []string` → `restrictedFieldsSet map[string]struct{}`; populated in `SetDefaultFields`)
- `config/validate.go` (`ValidateandParseLogField` uses set lookup)
- NEW `config/collision_test.go` (TS-12 unit table)
- NEW `test/integration/collision_prefix_test.go` (TS-28 SC7 end-to-end)

## Tests-first gate

1. `Test_ValidateAndParse_PrefixesCollidingKey` — table: reserved `time/level/message/error/caller`; non-reserved; renamed-via-SetDefaultFields.
2. `Test_Integration_CollisionPrefix` — full pipeline: user field `time` lands as `custom.time` in emitted JSON.
3. `Benchmark_ValidateField` — < 20 ns / 0 alloc per call (set lookup).

## Acceptance

1. `restrictedFieldsSet` populated on `SetDefaultFields` and on `init` for built-in defaults.
2. O(1) lookup; benchmark proves.
3. SC7 unit + integration green.
4. Set repopulated correctly when `SetDefaultFields` renames default keys (collision tracks renamed name).

## Bench notes

- Hot-path saves ~30 ns per user field (5-key slice `Contains` → set lookup).
- 16 fields → ~400 ns saved. Significant.

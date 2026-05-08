# 019 — ARCH-6: pre-render default-key prefixes

Owner-role: engineer.
Refs: ARCH-6, F12, F13, TS-11.
Depends: 014, 017.
LOC est: 220.

## Files touched

- `config/config.go` (`defaultFieldsRendered` cache populated on `SetDefaultFields` + at init)
- `entry/entry.go` (use cached `[]byte("\"<key>\":")` instead of per-call assembly)
- NEW `config/default_fields_test.go` (TS-11)

## Tests-first gate

1. `Test_SetDefaultFields_RenamesAndPrerenders` — after rename, cached prefix bytes reflect new name.
2. `Test_DefaultFieldsRendered_NoAllocOnHotPath` — `testing.AllocsPerRun` for `Log` does not increment when emitting `time/level/message`.
3. `Test_SetDefaultFields_RebuildsCollisionSet` — interaction with 014 collision set (renamed key now becomes the protected one).

## Acceptance

1. `defaultFieldsRendered [N]string` (or `[][]byte`) populated; lookup is index by `enum.DefaultLogKey`.
2. `Log` does not assemble `"time":` per call; appends pre-rendered prefix + value.
3. `SetDefaultFields` rebuilds cache AND `restrictedFieldsSet`.

## Bench notes

- Saves ~80 ns/event (LLD §6.5).
- 0 alloc on hot path for default keys.

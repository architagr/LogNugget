# 007 — Fix D-3 + D-20: EventPreProcessor singleton + iter-order doc + test refactor

Owner-role: engineer.
Refs: D-3, D-20, T-5, T-6, NF8, F18, F19, ARCH-11.
Depends: 001, 002.
LOC est: 280.

## Files touched

- `pipeline_stage/pre_processing_stage.go` (drop fake `sync.Once`; package-var assignment; doc comment for ARCH-11 hook iter order)
- REWRITE `pipeline_stage/pre_processing_stage_test.go` (TS-19; fix T-5 typo, T-6 spurious DeRegister)

## Tests-first gate

1. `Test_PreProcess_UnsetGetsAll` — hook at LevelUnSet receives every level event.
2. `Test_PreProcess_LevelOnly` — hook at LevelDebug only receives Debug events.
3. `Test_PreProcess_DeRegister` — DeRegister removes hook; subsequent events not delivered.
4. `Test_PreProcess_NameUniqueness` — registering second hook with same Name overwrites first; documented.
5. Tests use `set-equality` not ordered (D-20 / ARCH-11).

## Acceptance

1. `EventPreProcessorObj` is a package-var built in `init`; no `(&sync.Once{}).Do` pattern.
2. Doc comment: "hook iteration order is undefined; do not rely on order".
3. T-5 typo fixed: each mock returns its own `Name()`.
4. T-6 spurious pre-register `DeRegisterHook` removed; fresh observer per test via constructor.
5. SC8 a/b/c green at unit level (integration still in 036).

## Bench notes

- Hot-path neutral; `PreProcess` is dispatcher-side, not caller-side.

# Epic A — Module Hygiene & Test Scaffolding

Milestone: M1 (2026-05-14 → 2026-05-27).
Owner: Project Lead.
Goal: clean module surface + test infra in place so M2 baseline is attributable to F10/F17/F28 work, not churn.

## In scope

| Group | Refs |
|---|---|
| Module bump | D-14, NF11 |
| Demo relocate | D-15, NF12 |
| Bad init / reset foot-gun | D-9, D-3, NF7, NF8 |
| Dead encoder field | D-17, F7 |
| Exhaustive reset | D-18, F26 |
| Test-support bootstrap | TS-01, TS-02, TS-03 |
| Baseline recapture | NF6 |

## Out of scope

- Any F10/F17/F28 behavior change (Epic B).
- Pool / alloc rewrite (Epic C).
- Lifecycle drain refactor (Epic D).

## Exit criteria

1. Library `go.mod` = `go 1.21`, no `gin`/`zerolog` runtime deps. `go mod tidy` clean.
2. `examples/` carries demo `logger.go` (own go.mod or build-ignore).
3. `config.init` uses `sync.Once`; `ResetConfig` test-only or removed.
4. `pipelineStage` `EventPreProcessorObj` initialized via package-var; no fake `sync.Once`.
5. `JSONEncoder` struct trimmed; no nil `json.NewEncoder`.
6. `LogEntry.reset()` reflect-exhaustive; `TestLogEntryResetExhaustive` green.
7. `test/support/` package present: builders, doubles, corpus, golden helpers (TS-01/02/03).
8. `bench-baseline.txt` re-captured on cleaned tree; committed via Project Lead.

## Stories

| Story | Refs |
|---|---|
| 001 | TS-01 support builders |
| 002 | TS-02 support doubles |
| 003 | TS-03 support corpus + golden |
| 004 | D-14 go 1.21 bump |
| 005 | D-15 demo relocate |
| 006 | D-9 init / ResetConfig fix |
| 007 | D-3 EventPreProcessor singleton fix |
| 008 | D-17 JSONEncoder dead field |
| 009 | D-18 reset() exhaustive + test |
| 010 | M1 baseline recapture (Project Lead only) |

## Bench gate notes

- 010 is the only story permitted to mutate `bench-baseline.txt`. Project Lead runs `--update-baseline`.
- Other stories must keep ns/op delta ≤ 0 vs prior baseline (no regression, neutral OK).
- No story in this epic touches the hot path render (config/encoder/entry render-loop). Bench delta should be ≈ 0.

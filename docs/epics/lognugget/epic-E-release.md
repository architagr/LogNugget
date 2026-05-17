# Epic E — Release Hardening

Milestone: M5 (2026-07-09 → 2026-07-22).
Owner: Project Lead.
Goal: cut `release/v1.0.0` from `develop`; merge to `main`; signed annotated tag.

## In scope

| Group | Refs |
|---|---|
| README sync | NF12, F15-29 surface |
| Examples polish | NF12 |
| Final baseline | NF6 |
| Release branch + tag | CLAUDE.md release flow |
| Coverage threshold + shuffle CI | TS-32, TS-33 |

## Out of scope

- v2 features (Sampling, dispatcher Stop, OTel, multi-instance).

## Exit criteria

1. README documents: zero-config flow, F31 Shutdown, F23 backpressure, F4 GenerateInitialPool sizing rec, F22 channel buffer = 10 vs F24 maxBufferSize, ARCH-11 hook order undefined, N-3 mean-as-p99 proxy.
2. `examples/gin-demo/` and `examples/buffered-hook/` build standalone.
3. CI runs `-shuffle=on`; per-pkg cover ≥ 85% threshold check.
4. SC1–SC8 green on `release/v1.0.0`.
5. `go test ./...`, `go test -race ./...`, `golangci-lint run`, `bench-check.sh` green.
6. `release/v1.0.0` cut, merged to `main` (merge-commit), tagged `v1.0.0` (signed annotated), back-merged to `develop`.

## Stories

| Story | Refs |
|---|---|
| 029 | TS-32 CI -shuffle=on |
| 030 | TS-33 CI cover ≥ 85% |
| 031 | README sync + buffered-hook example |
| 032 | release/v1.0.0 branch cut + tag (Project Lead only) |

## Bench gate notes

- 032 may bump baseline final time; Project Lead only.
- All other stories non-hot-path.

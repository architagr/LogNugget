# 029 — TS-32: CI -shuffle=on + per-test t.Parallel sweep

Owner-role: engineer.
Refs: TS-32, T-13, T-14.
Depends: every test refactored (most stories under A/B/C/D).
LOC est: 80.

## Files touched

- `.github/workflows/ci.yml` (NEW or update; `-shuffle=on`)
- Sweep across `*_test.go`: add `t.Parallel()` to every leaf subtest that doesn't touch singleton.

## Tests-first gate

1. CI passes with `-shuffle=on` over 5 consecutive runs (manually verified before merge).
2. Spot-check: 3 random tests, run with `-run` selector; pass independent of order.

## Acceptance

1. CI workflow runs `go test -shuffle=on ./...`.
2. Every leaf subtest in non-singleton files has `t.Parallel()`.
3. Singleton-touching tests use `support.NewConfigBuilder(t).Build()` cleanup; `t.Parallel()` only added if cleanup is fully idempotent.

## Bench notes

- N/A.

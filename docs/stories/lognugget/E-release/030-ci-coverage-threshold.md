# 030 — TS-33: CI coverage ≥ 85% per package

Owner-role: engineer.
Refs: TS-33.
Depends: 029.
LOC est: 100.

## Files touched

- `.github/workflows/ci.yml` (cover.out artifact upload + per-pkg threshold check)
- NEW `scripts/cover-check.sh` (per-pkg threshold script)

## Tests-first gate

1. `cover-check.sh` itself has unit-style smoke test (golden output for fixture).
2. Run `go test -cover ./...` locally; assert each pkg ≥ 85%.

## Acceptance

1. `cover.out` uploaded as CI artifact (HTML).
2. CI fails if any pkg `< 85%` line coverage.
3. `bench-check.sh` baseline NOT mutated by this story.

## Bench notes

- N/A.

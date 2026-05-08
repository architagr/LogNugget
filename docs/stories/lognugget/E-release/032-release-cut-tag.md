# 032 — release/v1.0.0 cut + tag (Project Lead only)

Owner-role: N/A (Project Lead).
Refs: PRD §10, M5 exit, SC1–SC8.
Depends: 029, 030, 031, all M1–M4 stories merged into develop.
LOC est: 30 (commit messages + tag annotation).

## Files touched

- `bench-baseline.txt` (final M5 update if needed; Project Lead only)
- annotated tag `v1.0.0`

## Tests-first gate

- Pre-cut checklist:
  1. `go test ./...` green.
  2. `go test -race ./...` green.
  3. `golangci-lint run` clean.
  4. `./.claude/scripts/bench-check.sh` green.
  5. SC1–SC8 each map to ≥ 1 named test that passes (verified manually against test-framework §16).

## Acceptance

1. Branch `release/v1.0.0` cut from `develop`.
2. Merged into `main` via merge-commit (NOT squash).
3. Signed annotated tag `v1.0.0` pushed: `git tag -a v1.0.0 -m "Release v1.0.0" && git push origin v1.0.0`.
4. `main` back-merged into `develop`.
5. Issue umbrella closed.

## Bench notes

- Final baseline locked to v1.0.0 commit.

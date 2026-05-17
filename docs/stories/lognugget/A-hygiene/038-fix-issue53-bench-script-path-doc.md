# 038 — Fix #53: bench-check.sh path doc drift

Owner-role: N/A (Project Lead).
Refs: #53, CLAUDE.md, NF6.
Depends: none.
LOC est: ≤ 10.
Status: DOC-FIXED.

## Why

QA Lead filed #53. Said `bench-check.sh` missing. Real cause: doc drift. Script lives at `./scripts/bench-check.sh`. CLAUDE.md said `./.claude/scripts/bench-check.sh` in three spots.

## Files touched

- `CLAUDE.md` lines 18, 88, 112 — corrected to `./scripts/bench-check.sh`.

## Acceptance

1. CLAUDE.md three refs all canonical. DONE.
2. Issue #53 closed with comment pointing engineers at `./scripts/bench-check.sh`. DONE.

## Bench notes

Doc-only. Bench delta = 0.

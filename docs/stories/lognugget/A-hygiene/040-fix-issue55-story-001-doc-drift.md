# 040 — Fix #55: story 001 doc drift

Owner-role: N/A (Project Lead, doc-only).
Refs: #55, story 001.
Depends: none.
LOC est: ≤ 10.
Status: PENDING.

## Why

QA Lead filed #55. Story 001 acceptance #5 wrong on two points:

1. Says `pipelineStage.PublishLogMessageHookContract`. Real type is `config.PublishLogMessageHookContract` (capital P, in `config/` package).
2. Says `SpyHook records (level, []byte)`. Actual signature is `PublishLogMessage(entry []byte)` — no level. Engineer in PR #51 correctly dropped level.

## Files touched

- `docs/stories/lognugget/A-hygiene/001-test-support-builders.md` — fix acceptance #5.

## Acceptance

1. Story 001 acceptance #5 references `config.PublishLogMessageHookContract`.
2. Story 001 acceptance #5 records `[]byte` only (no level field).
3. No code changes.

# 033 — TS-13 + TS-14: static + context parser tests

Owner-role: engineer.
Refs: F15, F16, TS-13, TS-14.
Depends: 002 (StubStaticParser, StubContextParser).
LOC est: 240.

## Files touched

- NEW `config/static_parser_test.go` (TS-13)
- NEW `config/context_parser_test.go` (TS-14)

## Tests-first gate

1. `Test_StaticEnvParser_EvaluatesOnce` — register parser; assert `Calls() == 1` after N log calls (F15).
2. `Test_StaticEnvParser_RenderedAppendedToEveryLine` — every emitted line contains the static fields.
3. `Test_ContextParser_InvokedPerCall` — `Calls() == N` for N log calls with non-nil ctx.
4. `Test_ContextParser_SkippedWhenCtxNil` — `Calls() == 0` when ctx nil.
5. `Test_ContextParser_F14Collision` — context-parser key `time` rendered as `custom.time` (interaction with 014).

## Acceptance

1. F15 contract verified: parser evaluated exactly once at registration.
2. F16 contract verified: parser invoked per-call when ctx non-nil.
3. F14 collision protection applies to ctx-parser keys.

## Bench notes

- Test only.
- Add `Benchmark_ContextParser_Overhead` follow-up if observed regression > 50 ns; not part of this story.

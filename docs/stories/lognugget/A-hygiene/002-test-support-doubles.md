# 002 — TS-02: test/support doubles

Owner-role: engineer.
Refs: TS-02, T-1.
Depends: 001.
LOC est: 200.

## Files touched

- NEW `test/support/doubles.go` (SlowHook, FakePostProcessor, StubContextParser, StubStaticParser, StubEncoder, FakeClock)

## Tests-first gate

- `test/support/doubles_test.go` — each double has minimal contract test.

## Acceptance

1. `SlowHook` implements hook contract; blocks `PublishLogMessage` until `release()`.
2. `FakePostProcessor` no-IO; `Got() [][]byte`.
3. `StubContextParser` returns fixed map; `Calls() int`.
4. `StubStaticParser` returns fixed map; counts evaluations (must be 1 for F15).
5. `StubEncoder` `Append(dst, body)` passthrough.
6. `FakeClock.Now() time.Time` deterministic; pinned `2026-01-02T03:04:05Z`.
7. T-1 mitigation: doubles enable `assert.Eventually` everywhere; no `time.Sleep` waits.

## Bench notes

- Test-only.

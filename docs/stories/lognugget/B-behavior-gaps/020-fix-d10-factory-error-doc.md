# 020 — Fix D-10: factory error doc + variant for tests

Owner-role: engineer.
Refs: D-10, F7.
Depends: 008, 015.
LOC est: 80.

## Files touched

- `encoder/factory.go` (doc comment; new `DefaultEncoderFactoryE(t) (Encoder, error)` variant)
- `encoder/factory_test.go` (extend with error-variant test)

## Tests-first gate

1. `Test_DefaultEncoderFactoryE_UnknownType_ReturnsError` — error variant returns `ErrUnknownEncoder`.
2. `Test_DefaultEncoderFactory_GracefulFallback_DocCheck` — graceful variant still returns JSON for unknown (preserves behavior).

## Acceptance

1. Existing `DefaultEncoderFactory(t) Encoder` keeps fallback semantics; doc comment notes "fallback to JSON for unknown; use `DefaultEncoderFactoryE` to surface error".
2. Error variant uses sentinel `ErrUnknownEncoder`.
3. `SetEncoderType` continues to call graceful variant; logs nothing (NF13 no telemetry).

## Bench notes

- Factory called once at startup; hot-path delta = 0.

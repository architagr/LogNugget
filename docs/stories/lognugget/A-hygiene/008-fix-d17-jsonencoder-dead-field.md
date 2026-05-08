# 008 — Fix D-17: drop dead json.NewEncoder field

Owner-role: engineer.
Refs: D-17, F7.
Depends: none (independent).
LOC est: 80 (incl. factory test for TS-15).

## Files touched

- `encoder/json_encoder.go` (struct trim)
- NEW `encoder/factory_test.go` (TS-15 + TS-18 partial; F7 fallback)

## Tests-first gate

1. `Test_JSONEncoder_NoEmbeddedNilEncoder` — type assertion / reflect: `JSONEncoder` has zero non-zero fields, no `*json.Encoder`.
2. `Test_DefaultEncoderFactory_KnownTypes` — JSON, Text resolve.
3. `Test_DefaultEncoderFactory_UnknownFallsBackToJSON` — D-10 doc ack.

## Acceptance

1. `JSONEncoder` is `struct{}` (or `struct{ table *[256]uint8 }` if 016 lands first).
2. No `json.NewEncoder(nil)` call survives.
3. Factory tests green.

## Bench notes

- Removes a never-used field allocation. Hot-path delta ≈ 0 because field never invoked.

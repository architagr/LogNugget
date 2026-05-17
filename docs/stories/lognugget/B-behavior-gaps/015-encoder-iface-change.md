# 015 — ARCH-2 + ARCH-14: encoder iface Append + newline framing

Owner-role: engineer.
Refs: ARCH-2, ARCH-14, F7, F8, F9, TS-18.
Depends: 008.
LOC est: 240.

## Files touched

- `encoder/encoder.go` (interface change: `Append(dst, body []byte) []byte` + `Name() string`)
- `encoder/text_encoder.go` (impl Append; `\n` appended)
- `encoder/json_encoder.go` (impl Append; placeholder for F10 escaping in 016)
- `encoder/factory.go` (no signature change; resolves to `Encoder`)
- `entry/entry.go` (call `encoderObj.Append(dst, body)` instead of `Write(string)`)
- NEW `encoder/text_encoder_test.go`

## Tests-first gate

1. `Test_Encoder_Iface_Compliance` — both encoders satisfy iface; pkg-private `_ Encoder = (*JSONEncoder)(nil)`.
2. `Test_TextEncoder_Passthrough` — `Append(nil, body)` returns `body` + `\n`.
3. `Test_JSONEncoder_AppendBracesAndNewline` — wraps body in `{...}\n` (escaping arrives in 016).

## Acceptance

1. Old `Write(string) ([]byte, error)` removed (no shim retained per epic close criterion).
2. Newline framing centralised in encoder (ARCH-14).
3. SC1 still green: zero-config output ends `\n`.
4. Caller passes pre-rendered body bytes; encoder appends to dst.

## Bench notes

- Stage 8 budget: ~150 ns / 1 alloc (the output []byte). Append signature lets caller reuse a pooled dst → saves the alloc when 021 lands. For now allocate inside encoder.
- Net hot-path saving here: ~50 ns + 0 alloc change at this story (alloc dominated in 021).

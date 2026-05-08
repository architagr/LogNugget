# 016 — Fix D-2: JSONEncoder RFC 8259 escaping

Owner-role: engineer.
Refs: D-2, F10, SC4, TS-16.
Depends: 015.
LOC est: 280.

## Files touched

- `encoder/json_encoder.go` (escape table init; `Append` does per-byte escaping for body; OR `escapeJSON(dst, src)` helper)
- NEW `encoder/json_encoder_test.go` (TS-16 escape table)
- UPDATE `testdata/golden/unicode_escape.json` (real content)

## Tests-first gate

1. `Test_JSONEncoder_Escape` — table: `"`, `\`, `\b\f\n\r\t`, U+0000–U+001F (every code point), valid UTF-8 multi-byte, invalid UTF-8 → `�`.
2. `Test_JSONEncoder_RFC8259_Roundtrip` — 1000-event corpus via `support.JSONCorpus`; `encoding/json.Unmarshal` round-trip succeeds for every event.
3. `Test_JSONEncoder_Numerics_Unquoted` — int/float emitted unquoted (cooperates with 017).
4. Golden master: `Test_JSONEncoder_UnicodeGolden` matches `testdata/golden/unicode_escape.json`.

## Acceptance

1. `var jsonEscapeTable [256]uint8` initialised in `package init`.
2. Escape semantics RFC 8259 conformant (LLD §7.3 table).
3. NaN / ±Inf → JSON `null`.
4. SC4 green; D-2 closed.
5. F10 escape lives in encoder, NOT in `ParseLogField` (which becomes value-render only — see 017).

## Bench notes

- Per-byte loop with table lookup: ~3 ns/byte. 16 fields × ~50 bytes ≈ 2400 ns total escape work — already INSIDE stage 6 (render fields, target 250 ns). Mitigation: pre-render at AppendField site so escape happens once per value, not per field.
- Add `Benchmark_JSONEncoder_Escape` (lands in 037).
- Watch alloc count: must stay ≤ 1 (the output []byte). Use pre-grown dst.

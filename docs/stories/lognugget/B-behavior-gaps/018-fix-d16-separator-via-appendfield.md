# 018 — Fix D-16: comma separator via AppendField sequencing

Owner-role: engineer.
Refs: D-16, F10, F12.
Depends: 017.
LOC est: 120.

## Files touched

- `entry/entry.go` (drop `strings.Join`; sequential `AppendField` calls with prepended comma)
- NEW unit asserting comma placement

## Tests-first gate

1. `Test_LogEntry_FieldSeparator_Comma` — emitted JSON has `,` between key:value pairs (verified via `json.Unmarshal` count, not literal substring per T-2 lesson).
2. `Test_LogEntry_TrailingNoComma` — last field followed by `}` not `,}`.

## Acceptance

1. `strings.Join(data, ", ")` removed from `entry.go:91`.
2. Render loop: first AppendField writes key:val; subsequent prepend `,` to dst.
3. Output is valid JSON post-encoder wrap (SC4 still green via 016 fuzz).

## Bench notes

- Saves the `strings.Join` allocation + the per-call `[]string` slice (latter killed in 021).
- ~100 ns + 1 alloc saved.

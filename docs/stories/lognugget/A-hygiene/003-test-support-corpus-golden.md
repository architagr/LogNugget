# 003 — TS-03 + TS-31: corpus + golden helpers + golden files

Owner-role: engineer.
Refs: TS-03, TS-31, SC4.
Depends: 001.
LOC est: 220.

## Files touched

- NEW `test/support/corpus.go`
- NEW `test/support/golden.go`
- NEW `testdata/golden/zero_config.json`
- NEW `testdata/golden/error_with_fields.json`
- NEW `testdata/golden/unicode_escape.json`
- NEW `testdata/golden/collision_prefixed.json`
- NEW `testdata/fuzz/FuzzJSONEncoder/seed-001..010` (10 short seeds; corpus generator covers rest)

## Tests-first gate

- `test/support/golden_test.go` — `Golden(t, name, got)` mismatch fails; `-update` flag rewrites.
- `test/support/corpus_test.go` — `JSONCorpus(42, 1000)` returns 1000 events; deterministic (same seed → same bytes).

## Acceptance

1. `support.JSONCorpus(seed, n)` deterministic; mix per LLD test-framework §11.
2. `support.Golden(tb, name, got []byte)` reads `testdata/golden/<name>.json`; `-update` rewrites.
3. Four golden files exist with placeholder content; real content populated by stories 016/028 etc.
4. `FakeClock` integrated so golden timestamps stable.

## Bench notes

- Test-only.

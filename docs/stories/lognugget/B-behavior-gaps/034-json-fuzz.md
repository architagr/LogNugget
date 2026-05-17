# 034 — TS-17: FuzzJSONEncoder + fuzz seed corpus

Owner-role: engineer.
Refs: TS-17, F10, SC4.
Depends: 003 (corpus + golden), 016.
LOC est: 160.

## Files touched

- NEW `encoder/json_encoder_fuzz_test.go`
- NEW `testdata/fuzz/FuzzJSONEncoder/seed-{001..050}` (50 hand-picked seeds)

## Tests-first gate

- Seed corpus committed first; `go test -run FuzzJSONEncoder` (no `-fuzz`) replays seeds and passes before fuzz body lands.

## Acceptance

1. `FuzzJSONEncoder` body: `support.JSONCorpus(42, 1000)` registered as `f.Add` seeds; fuzz harness calls `AppendField` + `JSONEncoder.Append`; result must `json.Unmarshal` round-trip.
2. NaN/Inf seeds emit `null`; fuzz must accept that as round-trip (assertion handles).
3. CI runs nightly: `go test -fuzz=FuzzJSONEncoder -fuzztime=30s ./encoder` (not on per-PR per test-framework §11).
4. Seed-replay-only run on every PR: `go test ./encoder` (passes seeds without fuzzing).

## Bench notes

- Fuzz off hot path.

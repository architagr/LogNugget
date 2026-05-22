# P4 — Inline encoder framing

Owner-role: engineer.
Issue: [#85](https://github.com/architagr/LogNugget/issues/85).
Branch: `feat/85-p4-inline-framing` (cut from `feat/81-v2-performance` after P1 is merged, PRs back to `feat/81-v2-performance`). Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`) because git rejects nested refs when the parent ref exists. All story PRs target `feat/81-v2-performance`.
Depends: P1 merged. Relies on `HotSnapshot.EncoderOpen` and `HotSnapshot.EncoderClose` cached in `GetHotSnapshot()`.
LOC est: 190.

## Summary

Extend the `encoder.Encoder` interface with `OpenBytes() []byte` and `CloseBytes() []byte`. Implement them on `JSONEncoder` and `TextEncoder`. Rewrite `logWithSkip` to prepend `snap.EncoderOpen` and append `snap.EncoderClose` directly to `e.buf`, then transfer ownership of `e.buf` to the channel and sever the pool alias with a fresh `make([]byte, 0, initBufCap)` — eliminating the `en.Append(nil, e.buf)` intermediate buffer and the `append([]byte(nil), Data...)` defensive copy in `PublishLog`.

## Files touched

- `encoder/encoder.go` — add `OpenBytes() []byte` and `CloseBytes() []byte` to `Encoder` interface; add `NoopFramer` embeddable struct
- `encoder/json_encoder.go` — implement `OpenBytes`/`CloseBytes` on `JSONEncoder`; add compile-time interface check
- `encoder/text_encoder.go` — implement `OpenBytes`/`CloseBytes` on `TextEncoder`; add compile-time interface check
- `entry/entry.go` — rewrite framing sequence in `logWithSkip` (open frame, append fields, close frame, channel send, sever alias, pool return); remove `en.Append(nil, e.buf)` call
- `config/config.go` — remove `append([]byte(nil), Data...)` defensive copy in `PublishLog`
- NEW `entry/inline_framing_bench_test.go` — `BenchmarkLogEntry_InlineFraming_10`
- NEW `encoder/framing_test.go` — `OpenBytes`/`CloseBytes` round-trip + `NoopFramer` defaults

## Tests-first gate

1. `Test_JSONEncoder_OpenCloseBytes` — `OpenBytes()` returns `[]byte{'{'}`, `CloseBytes()` returns `[]byte{'}', '\n'}`.
2. `Test_TextEncoder_OpenCloseBytes` — `OpenBytes()` returns `nil`, `CloseBytes()` returns `[]byte{'\n'}`.
3. `Test_NoopFramer_Defaults` — `NoopFramer{}.OpenBytes()` returns `nil`; `NoopFramer{}.CloseBytes()` returns `[]byte{'\n'}`.
4. `Test_InlineFraming_JSONOutput` — a full log call with `JSONEncoder` produces a valid JSON object starting with `{` and ending with `}\n`; byte-identical to v1.0.0 output.
5. `Test_InlineFraming_TextOutput` — a full log call with `TextEncoder` produces output ending with `\n`; no double newline.
6. `Test_OwnershipTransfer_RaceClean` — under `go test -race`: send `e.buf` to channel, execute `e.buf = make([]byte, 0, initBufCap)`, call `e.Put()`; race detector reports no data race.
7. `Test_Append_BackwardCompat` — `Encoder.Append(dst, body)` still compiles and returns the correct result on both `JSONEncoder` and `TextEncoder` (the method is retained).

## Acceptance criteria

1. `BenchmarkLogEntry_InlineFraming_10` allocs/op drops by ≥ 1 vs the P3 baseline (elimination of `en.Append` intermediate buffer counts as 1 allocation; `PublishLog` defensive copy removal counts as a second; combined ≥ 1 net).
2. `go test -race ./...` clean — ownership transfer sequence verified race-free.
3. `Encoder.Append(dst, body)` method is retained on the interface and on both implementations; callers that invoke it directly continue to compile and work correctly.
4. `NoopFramer` embeddable struct exported from `encoder` package; third-party encoder implementations can embed it to satisfy the extended interface without writing `OpenBytes`/`CloseBytes` from scratch.
5. `JSONEncoder` and `TextEncoder` each have a compile-time interface assertion (`var _ Encoder = (*JSONEncoder)(nil)` pattern).
6. `pool.go` (`reset()` method) is not modified; it retains `e.buf[:0]` which is already the fresh backing slice after the ownership transfer sequence.
7. All existing encoder and entry tests pass without modification.

## Bench notes

- `b.ReportAllocs()` required.
- The ownership transfer correctness (step 12: `e.buf = make(...)` before `e.Put()`) must be explicitly tested under `-race` per the race-detector protocol in CLAUDE.md.
- Bench comparison: run `go test -bench=BenchmarkLogEntry_InlineFraming_10 -benchmem -count=10 ./entry/` against the P3 baseline to confirm the ≥ 1 alloc drop.

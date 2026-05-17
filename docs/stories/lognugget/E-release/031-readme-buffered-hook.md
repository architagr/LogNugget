# 031 — README sync + buffered-hook example

Owner-role: engineer.
Refs: NF12, F4, F22, F23, F24, F31, ARCH-10, ARCH-11, ARCH-13, N-2, N-3.
Depends: 027, 028.
LOC est: 220.

## Files touched

- `README.md` (rewrite for v1)
- NEW `examples/buffered-hook/main.go` + `go.mod` (R2 mitigation example)

## Tests-first gate

- N/A docs; `examples/buffered-hook/` MUST `go build` standalone (CI step verifies).

## Acceptance

README documents:
1. Zero-config flow (SC1).
2. F31 `lognugget.Shutdown()` — graceful drain.
3. F23 backpressure: callers block when channel full; sizing rec.
4. F4 `GenerateInitialPool(GOMAXPROCS * 64)` rec.
5. F22 channel buffer (10) vs F24 maxBufferSize (post-proc bucket; 20 default).
6. ARCH-11: hook iter order undefined.
7. ARCH-10: no drop policy in v1; v2 candidate.
8. ARCH-13: writer errors silently dropped.
9. N-3: bench gate uses mean ns/op as p99 proxy for v1.
10. N-2: zero-config defaults stated (rate=1s, max=20, output=os.Stdout).

`examples/buffered-hook/`:
1. Demonstrates a hook that internally fans out to its own goroutine + buffered channel (R2 mitigation).
2. Standalone `go.mod`.

## Bench notes

- N/A docs.

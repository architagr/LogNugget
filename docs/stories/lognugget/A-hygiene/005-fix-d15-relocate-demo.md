# 005 — Fix D-15: relocate demo, strip lib deps

Owner-role: engineer.
Refs: D-15, NF12, T-9.
Depends: 004.
LOC est: 60 (mostly file moves + go.mod surgery).

## Files touched

- DELETE `logger.go` (repo root)
- NEW `examples/gin-demo/main.go` (moved demo)
- NEW `examples/gin-demo/go.mod`
- `go.mod` (remove `gin`, `zerolog`)
- `go.sum` (regenerate)
- DELETE `Benchmark_ZeroLog` from `test/benchmark/entry_benchmark_test.go` (T-9; full bench rewrite is story 010)

## Tests-first gate

- N/A pure relocation; verification = library `go list -m all` shows stdlib + testify only.

## Acceptance

1. Library `go.mod` runtime deps: stdlib only. Test deps: testify only.
2. `examples/gin-demo/` builds standalone (`cd examples/gin-demo && go build`).
3. Repo-root `logger.go` removed.
4. `go test ./...` green for library; example out-of-tree.
5. `Benchmark_ZeroLog` removed from library bench file (T-9).

## Bench notes

- Removing zerolog import shouldn't change library benches; it changes which side bench numbers compare against. Note in 010 baseline.

# 004 — Fix D-14: bump go.mod to 1.21

Owner-role: engineer.
Refs: D-14, NF11.
Depends: 001 (so test-support compiles on 1.21).
LOC est: 30.

## Files touched

- `go.mod` (`go 1.21`)
- `go.sum` (regenerate)

## Tests-first gate

- N/A pure tooling change; verification = `go build ./...` + `go test ./...` green on 1.21.

## Acceptance

1. `go.mod`: `go 1.21`.
2. `go mod tidy` clean.
3. `go build ./...` green.
4. `go test ./...` green (existing tests still compile).
5. CI Go version pinned to 1.21 (no CI yet — call out for Epic E story 029).

## Bench notes

- Bench delta should be neutral or slightly improved (compiler differences).

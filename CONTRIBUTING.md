# Contributing to LogNugget

Thank you for contributing! Please read this guide before opening issues or pull requests.

## Prerequisites

- Go 1.21+
- [`golangci-lint`](https://golangci-lint.run/usage/install/) installed and on your `$PATH`

## Workflow

1. **Fork** the repository and clone your fork locally.
2. **Branch** off `develop` using the naming convention below.
3. **Implement** with TDD — write tests first, then make them pass.
4. **Open a draft PR** as soon as local tests pass; this is the surface QA reviews against.
5. **Address QA** comments (up to 3 cycles); unresolved threads are arbitrated by the Project Lead.
6. **Mark ready** only after all gates below are green.

## Branch Naming

```
feat/<issue>-<short-slug>     # new feature
fix/<issue>-<short-slug>      # bug fix
docs/<issue>-<short-slug>     # documentation only
chore/<issue>-<short-slug>    # tooling, deps, config
refactor/<issue>-<short-slug> # code restructure without behaviour change
perf/<issue>-<short-slug>     # performance improvement
```

All branches are cut from `develop` and PR back to `develop` (or the parent feature branch for sub-tasks).

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(encoder): add JSON pretty-print option (refs #42)
fix(pipeline): prevent nil-deref on empty stage list (fixes #57)
docs: update README with Redis backend example
perf(lru): reduce allocations in hot path (refs #61)
```

Types: `feat`, `fix`, `docs`, `chore`, `perf`, `refactor`, `test`.

## Running Tests

```bash
go test ./... -tags testing -race -shuffle=on
```

## Running the Linter

```bash
golangci-lint run
```

## Performance Gate

Every change to a hot path must include a `*_bench_test.go` file. Run the gate before marking a PR ready:

```bash
./scripts/bench-check.sh
```

The gate fails if any benchmark mean exceeds **1 µs (1,000 ns/op)** or introduces a statistically significant regression against `bench-baseline.txt`. Bypassing the gate is not permitted.

## PR Checklist

Before marking a draft PR as ready for review, confirm:

- [ ] `go test ./... -tags testing -race -shuffle=on` passes
- [ ] `golangci-lint run` is clean
- [ ] `./scripts/bench-check.sh` is green
- [ ] The PR body references the related issue (`Fixes #<n>`)
- [ ] New public APIs are documented with Go doc comments

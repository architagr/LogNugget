# Epic V2 — Performance: Close the 31× Throughput Gap vs zerolog

Milestone: V2 (2026-05-21 → target gate-green on feat/81-v2-performance).
Owner: Project Lead.
Umbrella issue: [#81](https://github.com/architagr/LogNugget/issues/81).
Goal: reduce the parallel 10-field hot path from ~3,440 ns/op / 55 allocs to < 1,000 ns/op / ≤ 5 allocs, meeting the project-wide < 1 µs p99 SLO declared in CLAUDE.md.

## In scope

| Group | Issue | Root cause |
|---|---|---|
| P1 — Atomic minLevel + single config snapshot | #82 | RC-3: 6× `configMu.RLock` per hot-path call |
| P2 — Typed field API (`Str`/`Int`/`Bool`/`Float64`) | #83 | RC-2: `interface{}` boxing in `model.LogAttr.Value` |
| P3 — Append-to-buf context API | #84 | RC-1: `map[string]any` + `[]string` intermediate in `setLogContextFields` |
| P4 — Inline framing, eliminate double-buffer | #85 | RC-4: `en.Append(nil, e.buf)` + `append([]byte(nil), Data...)` double copy |
| P5 — Channel capacity 1000 + configurable | #86 | RC-5: channel cap=10 blocks 8-goroutine parallel callers |

## Out of scope

- New encoder formats (CBOR, Protobuf, plain-text variants).
- Changes to the async pipeline architecture (background goroutine, `LogEvent` channel shape, flush loop).
- Lock-free ring buffer as channel replacement (deferred to a future epic).
- New log levels or changes to `enum.LogLevel` taxonomy.
- OpenTelemetry SDK integration.
- Changes to `Fatal`/`Panic` semantics.
- Raising or bypassing the bench-check gate threshold.

## Exit criteria

1. `./scripts/bench-check.sh` green on `feat/81-v2-performance` (all benchmarks ≤ 1,000 ns/op).
2. `BenchmarkLognugget_Parallel_10CtxFields` (GOMAXPROCS=8): ≥ 3 M ops/sec, ≤ 5 allocs/op.
3. Filtered path (`BenchmarkLogEntry_FilteredPath`): 0 allocs, ≤ 35 ns/op — must not regress from v1.0.0.
4. `go test ./...` green, `go test -race ./...` green.
5. `golangci-lint run` clean.
6. Existing call sites using `model.LogAttr{Key: "k", Value: v}` compile and produce identical output (backward compat).
7. `SetContextFieldsParser` legacy path still works; `SetContextFieldsAppender` appender takes priority when both registered.
8. Project Lead runs `./scripts/bench-check.sh --update-baseline`; baseline commit references #81.

## Stories

| Story | Issue | Branch |
|---|---|---|
| P1 — Atomic minLevel + single config snapshot | #82 | `feat/82-p1-atomic-min-level` |
| P2 — Typed field API (Str/Int/Bool/Float64) | #83 | `feat/83-p2-typed-field-api` |
| P3 — Append-to-buf context API | #84 | `feat/84-p3-ctx-append-buf` |
| P4 — Inline encoder framing | #85 | `feat/85-p4-inline-framing` |
| P5 — Channel capacity 1000 | #86 | `feat/86-p5-channel-capacity` |

Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`) because git rejects nested refs when the parent ref exists. All story PRs target `feat/81-v2-performance`.

## Dependencies

- P2, P3, and P4 all depend on P1 being merged into `feat/81-v2-performance` first; they consume `HotSnapshot` and `GetAtomicMinLevel` introduced by P1.
- P5 is independent and can be merged in any order.
- Each story branch is cut from `feat/81-v2-performance` and PRs back to `feat/81-v2-performance`.

## Bench gate notes

- Every story ships a `*_bench_test.go` file covering its hot path (see LLD §10.2).
- No story may raise the bench-check threshold or mark a PR ready over a red gate.
- Baseline update (`--update-baseline`) is performed by the Project Lead only, after all five story PRs (#82–#86) are merged and the gate is green. The update commit lands on the feature branch PR to `develop`, not on individual story PRs.
- `DafaultLogBuffer` is updated from `20` to `1000` in P5; this constant change is non-regressing on the bench gate.

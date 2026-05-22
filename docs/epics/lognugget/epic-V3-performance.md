# Epic V3 — Sub-500 ns Hot Path + First-Class OTel Distributed Tracing

Milestone: V3 (2026-05-22 → gate-green on feat/111-v3-performance).
Owner: Project Lead.
Umbrella issue: [#111](https://github.com/architagr/LogNugget/issues/111).
Goal: cut the parallel hot path from ~1,090 ns/op / 5 allocs to ≤ 500 ns/op / ≤ 1 alloc by eliminating every remaining RLock on the hot path, shrinking per-call allocations to zero, and adding first-class OTel distributed tracing support.

## In scope

| Group | Issue | Bottleneck | Est. saving |
|---|---|---|---|
| P1 — atomic.Bool pre-processor gate | #112 | `HasEventPreProcessors()` acquires `configMu.RLock` every call | ~80 ns |
| P2 — atomic.Pointer[HotSnapshot] copy-on-write | #113 | `GetHotSnapshot()` acquires `configMu.RLock` + copies 13-field struct | ~120 ns |
| P3 — atomic channel pointer in PublishLog | #114 | `PublishLog()` acquires `configMu.RLock` for channel pointer | ~50 ns |
| P4 — AppendFormat direct timestamp | #115 | `customTime.Format()` returns string; `AppendQuotedString` converts back to `[]byte` | ~40 ns, 2 allocs |
| P5 — appendJSONStringStr string-native escape | #116 | `AppendQuotedString` calls `[]byte(s)` on every string field | ~20 ns, 2 allocs |
| P6 — pre-rendered quoted level bytes | #117 | `level.String()` + `AppendQuotedString` — 5 known-constant values | ~15 ns, 1 alloc |
| P7 — first-class OTel ContextFieldsAppender | #118 | No built-in OTel; benchmark uses legacy map parser (~100 ns, 4 allocs) | ~100 ns, 4 allocs |
| P8 — exact-size buffer alias severance | #119 | New pool buf always 1024 B regardless of actual log line size (~200 B) | ~800 B/call |
| P9 — lock-free MPSC ring buffer | #120 | Go channel contention under 8-goroutine load costs ~130 ns | ~130 ns |

## Out of scope

- New encoder formats (CBOR, Protobuf, plain-text variants).
- Changes to the `LogEntry` public API (method signatures, level methods).
- New log levels or changes to `enum.LogLevel` taxonomy.
- Changes to `Fatal`/`Panic` semantics.
- Raising or bypassing the bench-check gate threshold.
- Bundling OTel SDK as a core dependency (OTel integration lives in `examples/otel-appender/`).

## Exit criteria

1. `./scripts/bench-check.sh` green on `feat/111-v3-performance`.
2. `Benchmark_Log_Parallel_NoCtx` (GOMAXPROCS=8): ≤ 500 ns/op.
3. `Benchmark_Log_Parallel_10CtxFields` with `SetContextFieldsAppender`: ≤ 500 ns/op, ≤ 1 alloc/op.
4. Filtered path (`BenchmarkLogEntry_FilteredPath`): 0 allocs, ≤ 35 ns/op — must not regress from V2.
5. `go test ./...` green, `go test -race ./...` green.
6. `golangci-lint run` clean.
7. Project Lead runs `./scripts/bench-check.sh --update-baseline`; baseline commit references #111.

## Stories

| Story | Issue | Branch |
|---|---|---|
| P1 — atomic.Bool pre-processor gate | #112 | `feat/112-p1-atomic-preprocessor-gate` |
| P2 — atomic.Pointer[HotSnapshot] | #113 | `feat/113-p2-atomic-pointer-snapshot` |
| P3 — atomic channel pointer | #114 | `feat/114-p3-atomic-channel-pointer` |
| P4 — direct timestamp append | #115 | `feat/115-p4-direct-timestamp` |
| P5 — string-native JSON escape | #116 | `feat/116-p5-string-native-escape` |
| P6 — pre-rendered level bytes | #117 | `feat/117-p6-prerendered-level-bytes` |
| P7 — OTel ContextFieldsAppender | #118 | `feat/118-p7-otel-context-appender` |
| P8 — exact-size buffer severance | #119 | `feat/119-p8-exact-buffer-size` |
| P9 — lock-free MPSC ring buffer | #120 | `feat/120-p9-mpsc-ring-buffer` |

Note: Sub-branches use `feat/<issue>-<slug>` naming (not nested `/`). All story PRs target `feat/111-v3-performance`.

## Dependency order

```
feat/111-v3-performance
├── P1 (#112) — independent; unblocks nothing but removes a lock on EVERY hot call
├── P6 (#117) — independent; pure hot-path win
├── P5 (#116) — independent; hot-path win
├── P4 (#115) — independent; hot-path win
├── P8 (#119) — independent; small, safe
├── P2 (#113) — after P1 merged (touches same configMu region); biggest single win
├── P3 (#114) — after P2 merged (same PublishLog region)
├── P7 (#118) — after P2 merged (benchmark update uses zero-alloc appender path)
└── P9 (#120) — last; replaces channel entirely; validates on full P1-P8 stack
```

P1, P4, P5, P6, P8 may be worked in parallel. P2 should start after P1 merges to the feature branch to avoid a large conflict in `config.go`. P3 and P7 start after P2. P9 is last.

# PRD: Epic V4, beat zerolog under real IO and be correct while doing it

| Field | Value |
|---|---|
| Feature slug | `v4-beat-zerolog` |
| Status | DELIVERED. Released as v4.0.0, with the module-path fix in v4.0.1 |
| Author | Project Lead |
| Date | 2026-09-20 |
| Target branch | `feat/131-v4-beat-zerolog` to `develop` to `main` |

---

## 1. Summary

V4 started as a performance epic: eliminate the remaining per-call allocations
(dispatch buffer pool, single-slab `LogEntry`), finish the typed field API, and
build a harness that measures LogNugget against zerolog under a sink with real
latency instead of `io.Discard`.

Re-measuring that work changed the epic. Two findings:

1. **The V4 benchmark numbers were measuring leaked global state.** Package
   `config` is a process-global singleton, and `Benchmark_Log` installed the
   legacy `map[string]any` context parser without tearing it down. Every
   benchmark that ran after it in the same binary paid for that parser:
   `Benchmark_Log_Parallel_NoCtx` reported 392 B/op and 4 allocs/op in a full
   suite run, and 50 B/op with 2 allocs/op when run alone.
2. **The optimisation behind the headline allocation win had introduced data
   corruption.** The V4-P2 dispatch buffer pool recycles a record's buffer as
   soon as every pre-processor returns, but the built-in collector retained
   that borrowed slice until its bucket flushed. Under concurrent load the
   writer received truncated and duplicated records.

The epic therefore covers correctness of the async pipeline, honesty of the
measurements, and the publication readiness that an announced library needs.

---

## 2. Problem statement

LogNugget is about to be announced publicly. Three classes of problem block
that:

**Correctness.** The async pipeline could corrupt records (buffer reuse),
reorder them (one flush goroutine per batch), lose them (`Shutdown` did not
drain the dispatch ring), and emit unparseable JSON (`Any` with a composite
value). None of these are visible in a single-goroutine smoke test; all of them
are visible in production.

**A configuration surface that did not work.** `SetOutput`, `SetRate` and
`SetLogBufferMaxSize` wrote to struct fields nothing read. `RegisterHook`
filled a map the dispatch path never consults. A user following the README's
configuration table would change nothing and have no way to tell.

**Gates that could not run.** `go test ./...` panicked on Go 1.26,
`golangci-lint run` refused to start against a v1 config schema, and the
performance gate, a documented merge blocker, ran in no workflow at all. Its
regression half was keyed off `benchstat`'s exit code, which is 0 whether or
not anything regressed.

---

## 3. Scope

### In scope

| ID | Story | Rationale |
|---|---|---|
| P2 | Dispatch buffer pool | Removes a `make()` per call |
| P3 | Single-slab `LogEntry` | Cold pool miss: 3 allocations down to 1 |
| P5 | `Err` / `Any` chain methods | Completes the typed field API |
| (none) | `SetContextFields` typed context API | Replaces raw `[]byte` handling for most callers |
| BENCH | loki-bench harness | Measures the claim that actually matters: latency under a real sink |
| C1 | Copy records into a pooled arena in the collector | Fixes buffer-reuse corruption |
| C2 | `FlushDispatch` + drain-on-`Shutdown` | Fixes record loss at exit |
| C3 | Single writer goroutine per collector | Fixes out-of-order output |
| C4 | One `Write` per flush batch | Makes batching observable |
| C5 | Wire `SetOutput` / `SetRate` / `SetLogBufferMaxSize` to the collector | Makes the documented surface real |
| C6 | Valid JSON for composite `Any`; reserved keys on chain methods | Fixes unparseable and ambiguous records |
| T1 | Benchmark isolation harness | Makes every published number reproducible in isolation |
| T2 | Repair the three gates | Restores the merge criteria |
| T3 | Remove `-shuffle=on` flakiness | CI must be trustworthy before it can block |
| DOC | Cookbook + README rewrite | An announced library needs a first hour that works |

### Out of scope

- Log rotation, sampling, adaptive ring sizing, an OTel SDK integration
  package. Listed as future work in the README.
- Closing the raw ns/op gap to zerolog against a discard sink. That gap is
  inherent to an async design and the README now says so rather than working
  around it.

---

## 4. Non-functional requirements

| ID | Requirement |
|---|---|
| NF1 | Caller-side hot path stays under the 1 µs p99 SLO, enforced by `bench-check.sh` |
| NF2 | No record may be corrupted, duplicated or reordered under concurrent load |
| NF3 | `Shutdown` loses nothing that was published before it was called |
| NF4 | Every documented configuration setter has an observable effect, covered by a test |
| NF5 | `go test -race -shuffle=on ./...` is green repeatedly, not on average |
| NF6 | Every published benchmark number is reproducible from a clean process |

---

## 5. Acceptance criteria

1. A concurrency test writes N records from 8 goroutines and reads back exactly
   N parseable records with no duplicates, `test/integration/buffer_reuse_test.go`.
2. A record logged immediately before `Shutdown` reaches the writer:
   `test/integration/flush_dispatch_test.go`.
3. Records arrive at the writer in publication order:
   `test/integration/ordering_test.go`.
4. Each of `SetOutput`, `SetRate`, `SetLogBufferMaxSize` changes observable
   behaviour, `test/integration/runtime_config_test.go`.
5. `Any` produces parseable JSON for slices, maps, structs and durations;
   reserved keys appear exactly once, `entry/record_validity_test.go`.
6. `go test ./...`, `golangci-lint run` and `./scripts/bench-check.sh` all pass,
   and all three run in CI.
7. Every cookbook example compiles, runs, and its output matches the README.

---

## 6. Risks

| Risk | Mitigation |
|---|---|
| The batched-`Write` change breaks a sink that assumed one record per `Write` | Documented as BREAKING in the CHANGELOG; records stay newline-delimited, and `examples/loki-bench` shows the split |
| Correctness fixes cost latency | Measured: roughly +8% on the parallel hot path, against 1 allocation instead of 2. Still ~3× inside the SLO |
| V3's published numbers cannot be compared to V4's | Stated explicitly in the CHANGELOG, README and STATUS rather than quietly re-baselined |

---

## 7. Open questions

- #132 asked for a sync path fast enough to beat zerolog at eight goroutines.
  It is implemented, but the measurement says the premise was wrong: removing
  the queue makes the parallel case slower, not faster, because the callers
  then contend on the sink. Is a sync path that is only a serial win worth
  keeping in the public API? It ships as opt-in and documented; the
  alternative is to withdraw it.
- Should the collector expose a `Sync()` for callers who want a durability
  point without shutting down? `FlushDispatch` covers the dispatch stage only.
- #135 assumed a `[]byte` key that this codebase has never had. Worth auditing
  the remaining open issues for the same class of stale premise before they
  are picked up.

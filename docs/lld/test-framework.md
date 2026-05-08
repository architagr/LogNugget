# LogNugget — Test Framework Design (v1)

Status: Draft for EM review.
Owner: Chief Architect.
Source LLD: `docs/lld/lognugget.md` (mechanics). Source PRD: `docs/prds/lognugget.md` (SC / F / NF). CLAUDE.md (gate semantics).
Caveman style. Tables / bullets / fragments. PRD owns SCs. LLD owns mechanics. This doc owns test design only.

---

## 1. Scope

In:
- Test layer model + ratios.
- Directory + naming conventions.
- Fixtures, builders, doubles catalog.
- Table-driven, race, bench, property, golden-master patterns.
- CI gate sequence + tooling decisions.
- F-ID / SC-ID → test mapping.
- Defect register for current `*_test.go` files.
- Story-cuttable test-only work.

Out:
- Production code mechanics → LLD.
- Acceptance criteria text → PRD §9.
- Build / branch policy → CLAUDE.md.
- v2 fuzz beyond JSON encoder / sampling tests / multi-instance benches.

---

## 2. Layer model

| Layer | Tool | Count target | Latency budget | Gate? |
|---|---|---|---|---|
| Unit | `testing` + testify/assert | ≥ 60 | < 50 ms / test | green = required |
| Race overlay | `go test -race` | reuses unit + integration | × 5 wall-clock | green = required (NF9, SC6) |
| Integration | `testing` (full pipeline) | ~ 8 | < 200 ms / test | green = required |
| Property / Fuzz | `testing.F` (Go 1.21) | 1 corpus, 2 fuzz funcs | seed corpus 1k+ | green = required (SC4) |
| Bench | `testing.B` + benchstat | 5 benches × `-count=10` | reported, not bounded per-test | gate via `bench-check.sh` (NF6) |

Pyramid: see `diagrams/test-pyramid.svg`.

Ratio target: unit > integration > property > bench. Bench is narrow but blocking.

---

## 3. Directory layout

```
.
├── entry/
│   ├── entry.go
│   ├── entry_test.go              (kept; refactored — see §17)
│   ├── level_gate_test.go         (NEW; F6)
│   ├── source_capture_test.go     (NEW; F17/SC3)
│   ├── reset_test.go              (NEW; F26)
│   ├── pool_test.go               (NEW; F3/F4/F25)
│   └── levels_test.go             (NEW; F2 sugar methods)
├── config/
│   ├── append_field_test.go       (NEW; F10/F27 alloc-free)
│   ├── parse_log_field_test.go    (NEW; F10 every type)
│   ├── default_fields_test.go     (NEW; F12/F13)
│   ├── collision_test.go          (NEW; F14/SC7)
│   ├── static_parser_test.go      (NEW; F15)
│   ├── context_parser_test.go     (NEW; F16)
│   ├── encoder_factory_test.go    (NEW; F7)
│   └── reset_config_test.go       (NEW; D-9 regression)
├── encoder/
│   ├── json_encoder_test.go       (NEW; F10)
│   ├── json_encoder_fuzz_test.go  (NEW; SC4 fuzz)
│   ├── text_encoder_test.go       (NEW; F9)
│   └── factory_test.go            (NEW; F7 fallback)
├── pipeline_stage/
│   ├── pre_processing_stage_test.go      (kept; expanded — see §17)
│   ├── unset_log_post_processor_hook_test.go (kept; expanded)
│   ├── flush_test.go              (NEW; F24)
│   ├── stop_drain_test.go         (NEW; F30/SC5)
│   └── concurrent_publish_test.go (NEW; NF9)
├── custom_time/
│   └── time_test.go               (NEW; F28)
├── lognugget/
│   ├── zero_config_test.go        (NEW; SC1, F31)
│   └── shutdown_test.go           (NEW; F31)
├── test/
│   ├── support/                   (NEW; helpers — §5, §6)
│   │   ├── builders.go
│   │   ├── doubles.go
│   │   ├── corpus.go
│   │   └── golden.go
│   ├── integration/               (NEW)
│   │   ├── pipeline_test.go       (F21–F23)
│   │   ├── stop_drain_test.go     (SC5)
│   │   ├── hook_fanout_test.go    (SC8)
│   │   └── collision_prefix_test.go (SC7)
│   ├── race/                      (NEW)
│   │   └── concurrency_test.go    (NF7/NF9)
│   └── benchmark/
│       ├── entry_benchmark_test.go (kept; rewrite per §9, §17)
│       ├── log_parallel_bench_test.go (NEW)
│       ├── log_filtered_bench_test.go (NEW)
│       └── json_escape_bench_test.go  (NEW)
└── testdata/
    └── golden/
        ├── zero_config.json
        ├── error_with_fields.json
        ├── unicode_escape.json
        └── collision_prefixed.json
```

---

## 4. Naming conventions

| Kind | Pattern | Example |
|---|---|---|
| Unit | `Test_<Type>_<Behavior>` | `Test_LogEntry_MinLevelGate` |
| Subtest | `t.Run("<case>", ...)` inside table | `t.Run("filtered/level=Debug,min=Error", ...)` |
| Bench | `Benchmark_<HotPath>_<Variant>` | `Benchmark_Log_Parallel`, `Benchmark_JSONEncoder_Escape` |
| Fuzz | `Fuzz<API>` | `FuzzJSONEncoder` |
| Example | `Example_<API>` | `Example_NewLogEntry` |
| Race-only | suffix `_Race` | `Test_HookRegister_Race` |
| Integration | `Test_Integration_<Behavior>` | `Test_Integration_StopDrainsAllQueued` |

Rules:
- Underscores allowed (Go test convention permits in name suffix).
- Subtest case name = data describing input, not prose.
- One assertion-purpose per subtest. Multiple `assert` calls allowed if all describe one behavior.

---

## 5. Fixtures & builders

Pattern: Builder (fluent), Fixture (file-backed), Recording channel (drain assertions).

All helpers live in `test/support/`. Imported as `support "github.com/architagr/lognugget/test/support"`.

| Helper | Pattern | Sketch |
|---|---|---|
| `ConfigBuilder` | Builder | sets singleton via `config.Set*`; returns reset func |
| `EntryBuilder` | Builder | constructs `*LogEntry` with N fields, ctx, err |
| `EventBuilder` | Builder | constructs `config.LogEvent{Level, Data}` for unit tests on hooks |
| `jsonCorpus(n)` | Fixture | seeded RNG; n events covering string/int/float/error/unicode |
| `goldenLoad(t, name)` / `goldenSave(t, name, b, update bool)` | Golden Master | reads/writes `testdata/golden/<name>.json`; `-update` flag |
| `recordingChan(cap)` | Recording channel | `chan []byte`; backs SC5 drain assertions |

Code skeletons (≤ 10 lines each):

```go
// builders.go
type ConfigBuilder struct{ tb testing.TB }

func NewConfigBuilder(tb testing.TB) *ConfigBuilder { tb.Helper(); return &ConfigBuilder{tb: tb} }
func (b *ConfigBuilder) MinLevel(l enum.LogLevel) *ConfigBuilder { config.SetMinLevel(l); return b }
func (b *ConfigBuilder) Encoder(t enum.LogEncodeType) *ConfigBuilder { config.SetEncoderType(t); return b }
func (b *ConfigBuilder) Output(w io.Writer) *ConfigBuilder { config.SetOutput(w); return b }
func (b *ConfigBuilder) AddSource(v bool) *ConfigBuilder { config.SetAddSource(v); return b }
func (b *ConfigBuilder) Build() func() { return func() { config.ResetConfig() } } // test-only ResetConfig (D-9)
```

```go
// builders.go
type EntryBuilder struct{ fields []model.LogAttr; ctx context.Context; err error }

func (b *EntryBuilder) WithFields(n int) *EntryBuilder { /* gen N attrs */ return b }
func (b *EntryBuilder) WithCtx(ctx context.Context) *EntryBuilder { b.ctx = ctx; return b }
func (b *EntryBuilder) WithErr(e error) *EntryBuilder { b.err = e; return b }
func (b *EntryBuilder) Build() *entry.LogEntry { return entry.NewLogEntry() }
```

```go
// corpus.go
type CorpusEvent struct{ Key string; Value any }

func JSONCorpus(seed int64, n int) []CorpusEvent { /* deterministic mix */ }
```

```go
// golden.go
var update = flag.Bool("update", false, "rewrite golden files")

func Golden(t testing.TB, name string, got []byte) {
    t.Helper()
    p := filepath.Join("testdata", "golden", name+".json")
    if *update { _ = os.WriteFile(p, got, 0o644); return }
    want, err := os.ReadFile(p); if err != nil { t.Fatal(err) }
    if !bytes.Equal(want, got) { t.Fatalf("golden mismatch:\nwant=%s\n got=%s", want, got) }
}
```

---

## 6. Test doubles catalog

Pattern: Spy (records), Fake (working in-mem), Stub (canned).

| Collaborator | Double | Type | Purpose |
|---|---|---|---|
| `io.Writer` | `FakeWriter` | Fake | mu-guarded `[]byte` slice; `Bytes()`, `Count()` |
| `pipelineStage.PublishLogMessageHookContract` | `SpyHook` | Spy | records `(level, []byte)` to chan; `Name()` configurable |
| `pipelineStage.PublishLogMessageHookContract` | `SlowHook` | Spy + sleep | blocks `PublishLogMessage` until `release()` — drives R2 / NF5 backpressure tests |
| `config.preProcessingObserverContract` | `FakePreProc` | Fake | in-mem ring of `(level, body)`; lets unit tests on `config.PublishLog` skip pipeline |
| `pipelineStage.eventPostProcessor` | `FakePostProcessor` | Fake | satisfies hook contract, no IO, exposes `Got()` |
| `config.ContextFieldsParser` | `StubContextParser` | Stub | returns fixed map; counts invocations |
| `config.StaticEnvFieldsParser` | `StubStaticParser` | Stub | returns fixed map; asserts evaluated-once (F15) |
| `encoder.Encoder` | `StubEncoder` | Stub | `Append(dst, body) []byte` passthrough — isolates pipeline tests from F10 churn |
| `customTime.TimeNow` | `FakeClock` | Stub via build-tag swap or function var | deterministic `time.Time`; needed for golden-master |

All doubles in `test/support/doubles.go`. Each has a constructor + minimum surface.

---

## 7. Table-driven patterns

Pattern: Table-driven + Subtests (`t.Run`).

Canonical structure:

```go
func Test_<Type>_<Behavior>(t *testing.T) {
    cases := []struct{
        name string
        // inputs
        // expected outputs
    }{ /* … */ }
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            // arrange / act / assert
        })
    }
}
```

One canonical example per concern:

| Concern | Test | Cases |
|---|---|---|
| Level gate (F6) | `Test_LogEntry_MinLevelGate` | every (event-level × min-level) pair; assert hook called iff level ≥ min |
| JSON escape (F10) | `Test_JSONEncoder_Escape` | `"`, `\`, `\b\f\n\r\t`, U+0000–U+001F, valid UTF-8, invalid UTF-8 → `�` |
| Collision protection (F14/SC7) | `Test_ValidateAndParse_PrefixesCollidingKey` | reserved key (`time`, `level`, …), non-reserved key, custom-renamed reserved key |
| Hook fan-out (F19) | `Test_PreProcess_FanOut` | (`LevelUnSet` only, `LevelDebug` only, both, neither) × event-level matrix |
| Encoder factory (F7) | `Test_DefaultEncoderFactory` | known JSON, known Text, unknown → JSON fallback (D-10 doc) |

Rule: never assert on hook iteration order (D-20). Use set-equality.

---

## 8. Race & concurrency tests

Pattern: Concurrent producer / single consumer; `t.Parallel`; `-race` overlay.

Invocation: `go test -race ./...` mandatory in CI (SC6). Local: same command before flipping draft to Ready (CLAUDE.md).

Tests:

| Test | What | Assertion |
|---|---|---|
| `Test_Race_PostProcessorPublish` | 100 goroutines × 1k publishes each | `sum(out.Bytes() splits) == 100_000`; no `-race` hit |
| `Test_Race_HookRegisterUnderLog` | 100 loggers + 1 register/deregister loop | no `-race` hit; documents NF7 startup-only contract |
| `Test_Race_BenchLogParallel` | reuses `Benchmark_LogParallel` under `-race` | only sanity, perf assertion lives in bench gate |
| `Test_Race_StopWhileLogging` | callers logging while `Stop()` invoked | drain count == enqueued count; no panic |

Goroutine-leak check: **DEFERRED-V2**. Justification: stdlib has no leak detector; `goleak` adds runtime dep; v1 dispatcher is process-lifetime by design (no clean stop) so leak detector fires false-positives. Revisit when dispatcher gets a `Stop()` (PRD N5 / v2).

Pool-bounce stress test: subset of `Benchmark_LogParallel` with `-race`; if parallel bench regresses > 10% vs serial, escalate to per-P slab (R3).

---

## 9. Bench harness design

Pattern: Test Harness + Golden-baseline.

Reference: `diagrams/bench-gate-flow.svg`. Source: `./.claude/scripts/bench-check.sh`.

Required for every bench:
- `b.ReportAllocs()`.
- Setup outside the timed region (`b.StopTimer()` … `b.StartTimer()`).
- No `t.Log`/`fmt.Print` in inner loop.
- Fixed-seed RNG when randomness is needed; reproducibility > coverage.

Bench matrix (5 sub-benches under `Benchmark_Log` via `b.Run`):

| Sub-bench | Variant | Target ns/op | Allocs | Bytes |
|---|---|---|---|---|
| `addSource=off,fields=1` | minimal | < 700 | ≤ 8 | ≤ 800 |
| `addSource=off,fields=8` | typical | < 850 | ≤ 12 | ≤ 1,200 |
| `addSource=off,fields=16` | NF3 ceiling input | < 1,000 | ≤ 30 | ≤ 2,048 |
| `addSource=on,fields=16` | F17 worst case | < 1,000 | ≤ 32 | ≤ 2,200 |
| `encoder=text,fields=8` | encoder swap | < 800 | ≤ 10 | ≤ 1,000 |

Standalone benches:

| Bench | Purpose | Target |
|---|---|---|
| `Benchmark_LogParallel` | `b.RunParallel` cross-P; R3 detector | < 1,200 ns/op (relaxed +20%) |
| `Benchmark_LogFiltered` | level below gate; F6 short-circuit | < 100 ns/op, 0 allocs |
| `Benchmark_JSONEncoder_Escape` | per-byte escape table cost | < 100 ns / 100 chars |

Run: `go test -bench=. -benchmem -count=10 -run=^$ ./...`. `bench-baseline.txt` updated by Project Lead only via `--update-baseline` flag (CLAUDE.md).

NF1 ceiling enforced inside `bench-check.sh` (script parses `ns/op` line and compares to 1,000). Mean is the gate, not p99 — `-count=10` plus benchstat covers variance.

Warm-up: rely on `b.N` ramp-up (`testing` defaults). No manual warm-up loop.

p99 capture: out of scope for v1. NF1 says < 1 µs p99 but bench-check.sh enforces mean < 1,000 ns/op as the gate; p99 instrumentation is **DEFERRED-V2** because `testing.B` does not expose per-iteration timings without a custom harness.

---

## 10. Integration test rig

Pattern: end-to-end pipeline test, `FakeWriter` as sink, `recordingChan` as observation point.

Layout: `test/integration/`. Each test:
1. `support.NewConfigBuilder(t).MinLevel(Debug).Encoder(JSON).Output(fakeW).Build()` (returns reset func).
2. `pipelineStage.NewUnsetLogEventPostProcessor(rate, max, fakeW)`; `RegisterHook(LevelUnSet, postProc)`.
3. Loop: `entry.NewLogEntry().Info(...)`.
4. Drain via `time.Sleep` (replaceable with `assert.Eventually`).
5. Assert on `fakeW.Bytes()` parsed via `encoding/json.Unmarshal`.
6. `defer postProc.Stop()`.

Required tests:

| Test | Maps |
|---|---|
| `Test_Integration_ZeroConfigEmitsJSONLine` | SC1 |
| `Test_Integration_StopDrainsAllQueued` | SC5; enqueue `2 × maxBufferSize`, assert all written |
| `Test_Integration_HookFanout` | SC8 a/b/c |
| `Test_Integration_CollisionPrefix` | SC7; user field `time` rendered as `custom.time` |
| `Test_Integration_ChannelBuffersAndBlocks` | F22/F23; SlowHook + N+1 callers; assert N+1th blocks |

`≤ maxBufferSize × 2` queue test (SC5):
- `maxBucketSize = 100`.
- Enqueue 200 events with no flush ticker firing yet.
- Call `Stop()`.
- Assert `fakeW.Count() == 200`.
- Implementation depends on D-11 fix (`doneCh` instead of busy-wait).

---

## 11. Property tests

Pattern: Property-based via Go 1.21 stdlib `testing.F`.

In: encoder F10 round-trip.
Out: anything else (deferred-v2; no need to fuzz pipeline today).

```go
func FuzzJSONEncoder(f *testing.F) {
    seeds := support.JSONCorpus(42, 1000)
    for _, s := range seeds { f.Add(s.Key, fmt.Sprint(s.Value)) }
    f.Fuzz(func(t *testing.T, k, v string) {
        body := config.AppendField(nil, k, v)
        out := (&encoder.JSONEncoder{}).Append(nil, body)
        var m map[string]any
        if err := json.Unmarshal(bytes.TrimRight(out, "\n"), &m); err != nil {
            t.Fatalf("unmarshal: %v\nout=%q", err, out)
        }
        if got, _ := m[k].(string); got != v { t.Fatalf("round-trip: %q != %q", got, v) }
    })
}
```

1,000-event corpus generator (`support.JSONCorpus`):
- 200 ASCII strings.
- 200 unicode strings (BMP + supplementary planes).
- 100 strings with embedded `"` / `\` / `\n`.
- 100 strings with control bytes 0x00–0x1F.
- 200 ints (incl. min/max int64).
- 100 floats (incl. NaN, ±Inf — encoder must emit `null`, fuzz asserts that).
- 100 errors (`errors.New` of varying length).

Seed corpus committed under `testdata/fuzz/FuzzJSONEncoder/`. CI run with `go test -fuzz=FuzzJSONEncoder -fuzztime=30s ./encoder` on nightly only — not per-PR (avoids flaky red gate).

---

## 12. Golden master tests

Pattern: Golden Master + `-update` flag.

Layout: `testdata/golden/<name>.json`.

Files:
- `zero_config.json` — output of `entry.NewLogEntry().Info(ctx, "msg")` with default config + FakeClock pinned.
- `error_with_fields.json` — Error path with err + 8 fields.
- `unicode_escape.json` — embedded `"`, `\n`, supplementary plane char.
- `collision_prefixed.json` — user field `time` → `custom.time`.

Mechanism: `support.Golden(t, name, got)` (sketch in §5).
Update: `go test ./... -update`. Diff inspected by reviewer; never auto-merged.
FakeClock pinned to `2026-01-02T03:04:05Z` so timestamps stable.

---

## 13. Coverage targets

| Metric | Target | Tool |
|---|---|---|
| Line coverage / package | ≥ 85% | `go test -cover -coverprofile=cover.out ./...` |
| Branch coverage on hot path | ≥ 95% (entry, config render, encoder) | `go tool cover -func=cover.out` (line-cov proxy) |
| Hot-path mutation coverage | DEFERRED-V2 | `go-mutesting` candidate |

`cover.out` not committed. CI uploads HTML to artifacts.
Branch coverage on Go is approximated by line coverage with a higher bar (Go has no native branch tooling).

---

## 14. CI gate sequence

Order (fail-fast):

1. `golangci-lint run` — style / vet superset.
2. `go vet ./...` — redundant if golangci-lint includes vet, but kept explicit.
3. `go build ./...` — compile.
4. `go test ./...` — unit + integration, no race.
5. `go test -race ./...` — race overlay (NF9, SC6).
6. `./.claude/scripts/bench-check.sh` — bench gate (NF1, NF3, NF6).

Any step red → stop. No re-ordering. No skipping. CLAUDE.md Performance Gate: bypass not permitted.

Pre-commit (local, advisory): steps 1, 4, 5.
Pre-merge (CI, blocking): all 6.

---

## 15. Tooling stack

| Dep | Decision | Rationale |
|---|---|---|
| `github.com/stretchr/testify` (assert + require) | IN | already present; ergonomic; no runtime impact |
| `testing.F` (Go 1.21 fuzz) | IN | stdlib; SC4 driver |
| `github.com/google/go-cmp/cmp` | IN | structural diff for golden files; better failure output than `reflect.DeepEqual` |
| `go.uber.org/goleak` | DEFERRED-V2 | dispatcher is process-lifetime in v1 → false-positives; revisit when stop is added |
| `testing/synctest` (Go 1.24+) | OUT | repo floor is Go 1.21 (NF11); not available |
| `golang.org/x/perf/cmd/benchstat` | IN | already required by `bench-check.sh`; no new dep added by this doc |
| `github.com/maxbrunsfeld/counterfeiter` | OUT | hand-written doubles are smaller than generated; prefer §6 catalog |
| Mutation testing (`go-mutesting`) | DEFERRED-V2 | high-value but heavyweight; revisit at v1.1 |
| `gotestsum` | OUT | CI runs raw `go test`; pretty output not worth dep |
| `github.com/dvyukov/go-fuzz` | OUT | superseded by stdlib `testing.F` since 1.18 |

Library `go.mod` runtime stays stdlib-only (NF12). `testify`, `go-cmp` are test-only (no runtime surface).

---

## 16. Mapping matrix

See `diagrams/coverage-map.svg` for the visual grid. Textual matrix below for grep / story-cutting.

| F-ID / SC | Test file | Test function | Fixture / double |
|---|---|---|---|
| F2 | `entry/levels_test.go` | `Test_LogEntry_LevelMethods` | EntryBuilder + SpyHook |
| F3 | `entry/pool_test.go` | `Test_NewLogEntry_ReturnsResetEntry` | — |
| F4 | `entry/pool_test.go` | `Test_GenerateInitialPool_PrePopulatesN` | — |
| F6 | `entry/level_gate_test.go` | `Test_LogEntry_MinLevelGate` | ConfigBuilder + SpyHook |
| F7 | `encoder/factory_test.go` | `Test_DefaultEncoderFactory` | — |
| F9 | `encoder/text_encoder_test.go` | `Test_TextEncoder_Passthrough` | — |
| F10 / SC4 | `encoder/json_encoder_test.go` + `_fuzz_test.go` | `Test_JSONEncoder_RFC8259_Roundtrip`, `FuzzJSONEncoder` | jsonCorpus(1000) + golden |
| F12 / F13 | `config/default_fields_test.go` | `Test_SetDefaultFields_RenamesAndPrerenders` | ConfigBuilder |
| F14 / SC7 | `config/collision_test.go` | `Test_ValidateAndParse_PrefixesCollidingKey` | ConfigBuilder |
| F15 | `config/static_parser_test.go` | `Test_StaticEnvParser_EvaluatesOnce` | StubStaticParser |
| F16 | `config/context_parser_test.go` | `Test_ContextParser_InvokedPerCall` | StubContextParser + EntryBuilder |
| F17 / SC3 | `entry/source_capture_test.go` | `Test_LogEntry_CallerCapture_WhenAddSource` | ConfigBuilder.AddSource(true) |
| F18 / F19 / SC8 | `pipeline_stage/pre_processing_stage_test.go` | `Test_PreProcess_UnsetGetsAll`, `_LevelOnly`, `_DeRegister` | SpyHook ×2 |
| F20 / F24 | `pipeline_stage/flush_test.go` | `Test_PostProcessor_FlushOnSize`, `_OnTicker`, `_IdleNoFlush` | FakeWriter |
| F21 / F22 / F23 | `test/integration/pipeline_test.go` | `Test_Integration_ChannelBuffersAndBlocks` | SlowHook + recordingChan |
| F25 / F26 | `entry/reset_test.go` | `Test_LogEntry_ResetExhaustive` | reflect |
| F27 | `config/append_field_test.go` | `Test_AppendField_NoAlloc` | testing.AllocsPerRun |
| F28 | `custom_time/time_test.go` | `Test_TimeNow_UTC`, `Test_AppendFormat_NoAlloc` | — |
| F29 | `config/time_format_test.go` | `Test_SetTimeFormat_AppliesToOutput` | ConfigBuilder + FakeClock |
| F30 / SC5 | `test/integration/stop_drain_test.go` | `Test_Integration_StopDrainsAllQueued` | FakeWriter + recordingChan |
| F31 / SC1 | `lognugget/zero_config_test.go` | `Test_ZeroConfig_EmitsJSONLineOnStdout`, `Test_Shutdown_Idempotent` | FakeWriter |
| SC2 / NF1 / NF3 | `test/benchmark/*_test.go` | `Benchmark_Log/_Parallel/_Filtered/_AddSourceTrue` + `bench-check.sh` | bench-baseline.txt |
| SC6 | CI workflow | go test, -race, golangci-lint | CI gate |
| NF7 / NF8 / NF9 | `test/race/concurrency_test.go` | `Test_Race_HookRegisterUnderLog`, `_PostProcessorPublish` | SpyHook ×100 goroutines |

---

## 17. Current-test defect register

Each row = task-slicer story candidate.

| ID | File:line | Defect | Required fix | Maps |
|---|---|---|---|---|
| T-1 | `entry/entry_test.go:66, 114, 147` | `time.Sleep(10 * time.Millisecond)` to wait for async hook — flaky on loaded CI; sequence not deterministic | Replace with `assert.Eventually`; or use FakePreProc + sync drain. | NF8 |
| T-2 | `entry/entry_test.go:70-79` | Asserts via `assert.Contains(t, logMsg, config.ParseLogField(...))` — couples test to current `ParseLogField` implementation under test (D-5). When F10 fix lands, every assertion breaks. | Switch to `encoding/json.Unmarshal(out, &m)` + `assert.Equal(t, "lognugget", m["app_name"])`. Decouples from string-shape. | F10 |
| T-3 | `entry/entry_test.go:79, 161` | `assert.NotContains(t, logMsg, "caller")` — but `caller` is the legitimate field name when addSource=true; today addSource is on by default (D-7) so this assertion only passes because F17 is non-functional (D-1). False green. | After D-1 + D-7 fix, replace with addSource-off assertion AND a paired addSource-on test that asserts `m["caller"]` is non-empty. | F17, SC3 |
| T-4 | `entry/entry_test.go` (entire file) | Three near-duplicate test functions differ only in min-level + level-called; not table-driven. | Convert to `Test_Entry_LevelGate` table per §7 canonical pattern. | F6 |
| T-5 | `pipeline_stage/pre_processing_stage_test.go:30-31, 37-38` | Both `mockUnsetHook.Name()` and `mockDebugHook.Name()` return `"mockUnsetHook"` (typo). Register/dedup logic operates on Name → `mockDebugHook` register may overwrite `mockUnsetHook`. Test passes only because they live in different level buckets. | Rename `mockDebugHook.Name()` to `"mockDebugHook"`. | F18 |
| T-6 | `pipeline_stage/pre_processing_stage_test.go:38` | `obj.DeRegisterHook(enum.LevelUnSet, unsetHook.Name())` called BEFORE `RegisterHook` — DeRegister on a non-existent hook silently passes; suggests author was working around state leak between tests. | Remove pre-register DeRegister; use fresh `newEventPreProcessingObserver()` per test (already done correctly elsewhere). | NF8 |
| T-7 | `pipeline_stage/unset_log_post_processor_hook_test.go:53, 66` | `out.Count()` × 6? Each `Write` call increments `called` once. With 3 messages flushed in one bucket → `called` should be 3, not 6. The expected `6` suggests the test is asserting a side-effect of the buggy D-12 (`Unlock`+flush+`Lock`) that double-writes. | After D-12 fix, assert `Count() == 3` per bucket. Currently green = false green. | F24, NF9 |
| T-8 | `pipeline_stage/unset_log_post_processor_hook_test.go:53` | `assert.Eventually(..., 200ms, 50ms)` — at `rate=time.Minute`, only the bucket-cap path triggers flush; 200 ms is fine. But `TestPublishMessageWithIOAfterRate:60` does `time.Sleep(time.Second)` against `rate=500ms` — flaky. | Use `assert.Eventually` everywhere; remove `time.Sleep`-then-assert pairs. | NF9 |
| T-9 | `test/benchmark/entry_benchmark_test.go:1-15` | Imports `github.com/rs/zerolog`; runs `Benchmark_ZeroLog` — comparison code in the library bench harness adds runtime dep on zerolog (NF12 violation in test-only path) and pollutes baseline numbers. | Move `Benchmark_ZeroLog` to `examples/` or `test/benchmark/comparison/` with its own go.mod. Library bench file imports stdlib + lognugget only. | NF12 |
| T-10 | `test/benchmark/entry_benchmark_test.go:80` | `unsetPostProcessor` is created but `config.SetOutput(&MockWriter{})` is also called — two parallel sinks; bench measures the wrong path. | Single sink: post-processor with FakeWriter; remove the `SetOutput` line or the post-processor, depending on which path SC2 measures. Per LLD §11.3 bench measures post-processor path. | SC2 |
| T-11 | `test/benchmark/entry_benchmark_test.go` | Bench runs `Benchmark_Log` only, no `b.RunParallel` variant; R3 (sync.Pool false sharing) cannot be detected. | Add `Benchmark_Log_Parallel` per §9. | R3 |
| T-12 | `test/benchmark/entry_benchmark_test.go:88` | Per-iteration `context.WithValue(context.WithValue(...))` inside the timed loop — measures context allocation, not log path. | Hoist ctx outside loop OR build N contexts upfront and index. | NF1 |
| T-13 | All 3 `entry/entry_test.go` tests | Each test mutates the global singleton `config` — no per-test reset, so order-of-execution affects results. `go test -shuffle=on` would expose ordering bugs. | Use `support.NewConfigBuilder(t).Build()` returning a `t.Cleanup` to reset; enable `-shuffle=on` in CI. | NF7 |
| T-14 | All `*_test.go` | None use `t.Parallel()`. | Add `t.Parallel()` to every leaf subtest unless it touches the singleton (most do — fix per T-13 first). | speed |
| T-15 | No coverage of: F4, F14, F15, F16, F17, F27, F28, F29, F30, F31 | Documented in §3 — every NEW test file in §3 corresponds to a missing-coverage gap today. | Cut stories per §18. | F-IDs above |

---

## 18. Story-cuttable test work

Each row ≤ 300 LOC. Independent. Story-id matches §17 / §16.

| Story | Scope | LOC est | Depends on |
|---|---|---|---|
| TS-01 | Create `test/support/` package: `ConfigBuilder`, `EntryBuilder`, `EventBuilder`, `FakeWriter`, `SpyHook`, `FakePreProc`. | 260 | none |
| TS-02 | `test/support/doubles.go`: `SlowHook`, `FakePostProcessor`, `StubContextParser`, `StubStaticParser`, `StubEncoder`, `FakeClock`. | 200 | TS-01 |
| TS-03 | `test/support/corpus.go` + `golden.go` + `testdata/fuzz/FuzzJSONEncoder/` seed files. | 220 | TS-01 |
| TS-04 | Refactor `entry/entry_test.go` → `entry/levels_test.go` table-driven (T-2, T-4 fix). | 240 | TS-01 |
| TS-05 | NEW `entry/level_gate_test.go` (F6 table). | 140 | TS-01 |
| TS-06 | NEW `entry/source_capture_test.go` (F17 / SC3, addSource off+on). | 160 | TS-01, D-1 fix in production |
| TS-07 | NEW `entry/reset_test.go` (F26 reflect-exhaustive). | 100 | none |
| TS-08 | NEW `entry/pool_test.go` (F3, F4, F25). | 180 | TS-01 |
| TS-09 | NEW `config/append_field_test.go` (F27 alloc-free; testing.AllocsPerRun ≥ 100). | 130 | F27 production code |
| TS-10 | NEW `config/parse_log_field_test.go` (F10 every type). | 220 | none |
| TS-11 | NEW `config/default_fields_test.go` (F12, F13). | 180 | TS-01 |
| TS-12 | NEW `config/collision_test.go` (F14, SC7 table). | 160 | TS-01 |
| TS-13 | NEW `config/static_parser_test.go` (F15 evaluates-once). | 110 | TS-02 |
| TS-14 | NEW `config/context_parser_test.go` (F16 per-call). | 130 | TS-02 |
| TS-15 | NEW `config/encoder_factory_test.go` + `reset_config_test.go` (F7, D-9 regression). | 160 | none |
| TS-16 | NEW `encoder/json_encoder_test.go` (F10 escape table). | 240 | F10 production code |
| TS-17 | NEW `encoder/json_encoder_fuzz_test.go` (FuzzJSONEncoder + corpus seed). | 130 | TS-03, TS-16 |
| TS-18 | NEW `encoder/text_encoder_test.go` + `factory_test.go`. | 120 | none |
| TS-19 | Expand `pipeline_stage/pre_processing_stage_test.go` (T-5, T-6 fix; SC8 a/b/c). | 200 | TS-01 |
| TS-20 | Expand `pipeline_stage/unset_log_post_processor_hook_test.go` + NEW `flush_test.go` (T-7, T-8 fix; F24 table). | 280 | D-11, D-12 fixes |
| TS-21 | NEW `pipeline_stage/stop_drain_test.go` (F30, prerequisite for SC5). | 180 | D-11 fix |
| TS-22 | NEW `pipeline_stage/concurrent_publish_test.go` + `test/race/concurrency_test.go` (NF9, NF7). | 220 | TS-01, TS-02 |
| TS-23 | NEW `custom_time/time_test.go` (F28 UTC, AppendFormat alloc-free). | 110 | F28 fix (D-4) |
| TS-24 | NEW `lognugget/zero_config_test.go` + `shutdown_test.go` (SC1, F31 idempotent). | 180 | TS-02 (FakeWriter swap of os.Stdout) |
| TS-25 | NEW `test/integration/pipeline_test.go` (F21–F23 SlowHook backpressure). | 200 | TS-02 |
| TS-26 | NEW `test/integration/stop_drain_test.go` (SC5 2× maxBufferSize). | 180 | TS-21 |
| TS-27 | NEW `test/integration/hook_fanout_test.go` (SC8). | 180 | TS-19 |
| TS-28 | NEW `test/integration/collision_prefix_test.go` (SC7 end-to-end). | 140 | TS-12 |
| TS-29 | Rewrite `test/benchmark/entry_benchmark_test.go` (T-9, T-10, T-11, T-12 fix; sub-benches per §9 table). | 280 | F10 done; D-9 fix |
| TS-30 | NEW `test/benchmark/log_parallel_bench_test.go`, `log_filtered_bench_test.go`, `json_escape_bench_test.go`. | 240 | TS-29 |
| TS-31 | NEW `testdata/golden/*.json` + `support.Golden` wiring; one golden per: zero-config, error-with-fields, unicode, collision. | 100 | TS-03 |
| TS-32 | Add `-shuffle=on` to CI script + per-test `t.Parallel()` enable sweep (T-13, T-14). | 80 | every test refactored |
| TS-33 | Add `cover.out` artifact upload + `≥ 85%` per-package threshold check to CI. | 80 | none |

Total: 33 stories. Every one ≤ 300 LOC. Each maps to one or more F/SC/NF/T-id.

---

## 19. Diagram index

| File | Caption |
|---|---|
| `diagrams/test-pyramid.svg` | Test pyramid: unit / integration / property / bench layers; race-overlay band; counts and gate. |
| `diagrams/bench-gate-flow.svg` | `bench-check.sh` flow: collect → benchstat → threshold → baseline diff → exit; closing-the-gap branch; baseline-maintenance side panel. |
| `diagrams/test-fixture-tree.svg` | `test/support` composition: builders, doubles (Spy/Fake/Stub), fixtures; pattern legend. |
| `diagrams/coverage-map.svg` | F-ID / SC-ID grid → test file → test function → fixture; SC rows highlighted. |

---

## Pattern catalog (one-liner each)

| Pattern | Site | Why |
|---|---|---|
| Table-driven | every level/encoder/collision test | one test, N cases, one report |
| Subtests (`t.Run`) | inside every table | per-case isolation + `-run` selectors |
| Parallel matrix (`t.Parallel`) | leaf subtests not touching singleton | wall-clock floor |
| Builder | `ConfigBuilder`, `EntryBuilder`, `EventBuilder` | readable, chainable arrange step |
| Test Double — Spy | `SpyHook` | record calls, assert side-effects |
| Test Double — Fake | `FakeWriter`, `FakePostProcessor`, `FakePreProc` | working in-mem impl, no IO |
| Test Double — Stub | `StubContextParser`, `StubStaticParser`, `StubEncoder`, `FakeClock` | canned input/output, isolation |
| Golden Master | `testdata/golden/*.json` + `-update` | snapshot regression catch |
| Property / Fuzz | `FuzzJSONEncoder` | unknown-input safety (SC4) |
| Fixture | `jsonCorpus(1000)` + golden files | reproducible inputs |
| Test Harness | `test/benchmark/*` + `bench-check.sh` | gate, baseline, regression detect |
| Recording channel | `recordingChan(cap)` | drain-completeness assertions |
| Setup/teardown via `t.Cleanup` | `ConfigBuilder.Build()` returns reset | per-test isolation around singleton |
| Reflect-exhaustive | `Test_LogEntry_ResetExhaustive` | future-proof reset coverage |
| Eventually | `assert.Eventually` instead of `time.Sleep` | flake-free async assertion |

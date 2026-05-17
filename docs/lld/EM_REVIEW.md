# LogNugget v1 — EM Gate Review

Reviewer: Engineering Manager. Date: 2026-05-08.
Inputs: PRD (Final), LLD (Draft), Test Framework (Draft), 8 SVG diagrams, current code under `entry/`, `config/`, `encoder/`, `pipeline_stage/`, `go.mod`.
HLD waiver: confirmed (PRD §11). LLD consumes PRD §6 as HLD substitute. Acceptable.

---

## Verdict

**APPROVED — conditional, non-blocking notes inline. Phase 2 may start.**

PRD stamped `EM-APPROVED: 2026-05-08`.

---

## 1. Coverage matrix (PRD → LLD section)

| Group | F-IDs | LLD section | Status |
|---|---|---|---|
| API surface | F1–F4 | §2, §4.1, §8 | OK |
| Levels | F5, F6 | §3.2, §4.5, §4.1 | OK |
| Encoders | F7–F10 | §4.4, §7.1–§7.5 | OK; F11 deferred per PRD COULD |
| Field shape | F12–F14 | §3, §4.2, §6.5 | OK |
| Context/static | F15, F16 | §4.2, §3 step 7 | OK |
| Source capture | F17 | §3 step 5, §4.1, §12 D-1 | OK |
| Hooks | F18–F20 | §4.3, §6.1–§6.3 | OK |
| Pipeline | F21–F24 | §4.2, §5.2, §6.4 | OK |
| Memory reuse | F25–F27 | §4.1, §8 | OK |
| Time | F28, F29 | §4.5, §12 D-4 | OK |
| Lifecycle | F30, F31 | §4.5, §9.1, §9.2 | OK |
| NF1/NF3 | bench gate | §3, §11 | OK |
| NF5/NF7/NF8/NF9 | concurrency | §5 | OK |
| NF10 | availability | §10 | OK |
| NF11/NF12 | compatibility | §12 D-14, D-15 | OK |
| NF13/NF14 | compliance | n/a (no telemetry) | OK by absence |
| NF15 | observability | deferred per PRD | OK |
| SC1–SC8 | acceptance | §11.1 SC traceability + test-framework §16 | OK — all mapped to named tests |

No PRD requirement orphaned.

---

## 2. Defect register spot-check (3 of 13+)

| ID | Claim | Verified at | Real? |
|---|---|---|---|
| D-1 | `caller` field has TODO; F17 non-functional | `entry/entry.go:29` literal `// TODO: add a function to set caller from runtime.Caller` | YES |
| D-7 | `DafaultAddSource = true` blows hot-path budget | `config/config.go:19` `DafaultAddSource bool = true` | YES |
| D-9 | `init` calls `ResetConfig`; `ResetConfig` exported + re-creates channel | `config/config.go:33-35` `init() { ResetConfig() }`; `:250-252` re-creates `ch` and re-spawns dispatcher | YES — exported foot-gun confirmed |

Bonus: D-2 (`encoder/json_encoder.go:21` literal `[]byte("{" + entryData + "}")` no escaping) and D-17 (`json.NewEncoder(nil)` dead field at `:10`) confirmed in same pass. D-14 (`go.mod:3` go 1.18) and D-15 (gin@1.10.1, zerolog@1.34.0 in library `go.mod`) confirmed. Defect register is real, not hallucinated.

---

## 3. SLO realism (sum the budget)

LLD §3 table addSource=off: 10+30+5+80+0+250+120+150+120+25 = **890 ns**. Matches doc.
addSource=on: +250 = **1,140 ns** — RED. LLD admits this in §3.1.

§3.1 mitigation plan (after rewrites 1–6):
- ~720 ns off / ~970 ns on. Headroom **30 ns** on the on-path.

Concerns:
- 30 ns headroom on `addSource=on` is paper-thin. One missed inline, one extra map lookup, gate trips.
- Bench matrix in test-framework §9 sets `addSource=on,fields=16` target at < 1,000 ns/op — same razor margin. Documented but no fallback if it slips.
- `addSource=off` projection of ~720 ns is plausible given current 1246 → optimizations (1)+(2)+(3)+(4)+(5)+(6) sum to ~500 ns claimed savings. Math holds.

Verdict: arithmetic is honest. Risk acknowledged in PRD R4. Acceptable but **flag for Project Lead**: if M2 closes with `addSource=on` ≥ 950 ns, escalate to ARCH-8 (pull F27 forward) immediately, not at M3.

---

## 4. Test framework alignment (SC → test)

| SC | Named test | Mapped? |
|---|---|---|
| SC1 | `Test_ZeroConfig_EmitsJSONLineOnStdout` + integration | YES |
| SC2 | `Benchmark_Log{,_Parallel,_Filtered,_AddSourceTrue}` + bench-check.sh | YES |
| SC3 | `Test_LogEntry_CallerCapture_WhenAddSource` | YES |
| SC4 | `Test_JSONEncoder_RFC8259_Roundtrip` + `FuzzJSONEncoder` | YES |
| SC5 | `Test_Integration_StopDrainsAllQueued` (2× maxBufferSize) | YES |
| SC6 | CI green (lint+test+race) | YES |
| SC7 | `Test_ValidateAndParse_PrefixesCollidingKey` + integration | YES |
| SC8 | `Test_PreProcess_UnsetGetsAll/_LevelOnly/_DeRegister` + integration | YES |

Test-framework §17 itemizes 15 current-test defects (T-1…T-15) — spot-checked T-3 (false-green caller assertion) and T-5 (Name() typo `mockUnsetHook` returned by `mockDebugHook`) — **both real** at `pipeline_stage/pre_processing_stage_test.go` (Name collision pattern present in package).

---

## 5. Story-cuttable check

- LLD §12 has 20 D-* defect rows + LLD §2 LOC budgets per package (largest 360 LOC).
- Test framework §18 has 33 stories TS-01…TS-33, each ≤ 300 LOC, with declared dependencies.
- Production-side stories not yet sliced (Project Lead next step), but defect register + LOC budgets give clean slicing input.

Granular enough. Project Lead can proceed.

---

## 6. Open trade-offs (LLD §13) — review

| ID | Decision | Sound? |
|---|---|---|
| ARCH-1 | singleton retained | OK (PRD N6) |
| ARCH-2 | Encoder iface signature change | OK; required for NF1 |
| ARCH-3 | `runtime.Caller` over `Callers+CallersFrames` | OK (PRD R4) |
| ARCH-4 | addSource default false | OK (fixes D-7) |
| ARCH-5 | new top-level `lognugget` package | OK (clean F31 surface) |
| ARCH-6 | pre-render default-key prefixes | OK; ~80 ns saving plausible |
| ARCH-7 | LogEvent.Data is copy not reslice | OK; counted in NF3 (1 alloc) |
| ARCH-8 | F27 conditional on M2 outcome | OK; matches PRD §7.2 |
| ARCH-9 | sync.Pool over per-P slab | OK; escalation criterion explicit (R3) |
| ARCH-10 | block-on-full, no drop policy | OK (PRD NF5) |
| ARCH-11 | hook iteration order undefined | OK; documented |
| ARCH-12 | ctx parser map allocation = consumer cost | OK; v2 escape hatch noted |
| ARCH-13 | silent drop on writer error | OK (PRD NF10) |
| ARCH-14 | newline inside encoder Append | OK (SC1 framing) |
| ARCH-15 | ResetConfig becomes test-only | OK (fixes D-9) |

No decision needs user input. All trade-offs reasoned and bounded.

---

## 7. Gaps & non-blocking notes

| # | Issue | Severity | Owner | Action |
|---|---|---|---|---|
| N-1 | `addSource=on` 30 ns headroom in §3.1 projection. One regression trips gate. | non-blocking | Project Lead | Set ARCH-8 escalation trigger at ≥ 950 ns, not the current 90% NF3 alloc trigger. |
| N-2 | LLD §9.1 step 6 wires `defaultPostProc` with `rate=1*time.Second, maxBucketSize=20`. Not stated in PRD. | non-blocking | Architect | Add one-line rationale comment near the `init` it cites; document zero-config defaults in README at M5. |
| N-3 | LLD §11 references `Benchmark_LogParallel` target < 1,200 ns/op (relaxed +20%). PRD §4.1 NF1 says `< 1 µs p99` — strict. Test-framework §9 admits p99 instrumentation deferred-v2. | non-blocking | EM | Acceptable: bench gate enforces mean, and `-count=10` + benchstat covers variance. Document explicitly in README that bench gate uses mean as p99 proxy for v1. |
| N-4 | `lognugget.Shutdown()` idempotency in §9.2 mentions sync.Once but no test asserts double-Stop is no-op. | non-blocking | Architect | TS-24 (`Test_Shutdown_Idempotent`) already covers; verify story actually calls Shutdown twice. |
| N-5 | LLD does not address `Fatal` vs `Panic` interaction with async pipeline: `runtime.Goexit()` returns *before* the dispatcher drains the event. Event may be lost. | non-blocking | Architect | Add note in §10 or §9.2; v1 acceptable (Fatal is best-effort), but document loss window. |
| N-6 | F11 (`logfmt`) is COULD; LLD correctly excludes. No action. | n/a | — | — |
| N-7 | SC5 test (`Test_Integration_StopDrainsAllQueued`) spec is `2 × maxBufferSize`. Channel buffer (10) is upstream of bucket (200 in test). Drain must also account for in-flight events on `config.ch`. LLD §9.2 says "ch is NOT closed in v1". | non-blocking | Architect | Document that SC5 test enqueues *post-dispatch* (registers post-proc directly, drives `PublishLogMessage` not `entry.Info`). Otherwise channel-side drain is also required and is harder. |

None block approval. All recorded for Project Lead handoff.

---

## 8. Approval

PRD stamped `EM-APPROVED: 2026-05-08`. Phase 2 may start.

Project Lead next steps:
1. Slice production-side stories from LLD §12 defect register (20 rows) + LLD §2 LOC budgets. Use `task-slicer`.
2. Pull test stories TS-01…TS-33 from test-framework §18 verbatim.
3. Open feature branch per CLAUDE.md GitFlow; epic A first (M1 hygiene), unblocks F10/F17/F28 baseline.
4. Set N-1 escalation trigger before M2 ends.

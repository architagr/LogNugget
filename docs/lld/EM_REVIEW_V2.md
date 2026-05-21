# EM Review — Epic V2 Performance Design Package

| Field | Value |
|---|---|
| Reviewer | Engineering Manager |
| Date | 2026-05-21 |
| Decision | APPROVED with minor gaps for Project Lead attention |
| Feature slug | `v2-performance` |
| Umbrella issue | #81 |

---

## Decision

**APPROVED.**

The design package is coherent, traceable end-to-end from PRD requirements to LLD signatures, and the live code cross-check confirms the proposed changes are compatible with the existing implementation. All five PRD open questions are resolved in the PRD itself. All four HLD architectural sub-questions (AQ-1 through AQ-4) are resolved and assigned to specific LLD sections. No blocking gaps exist.

The gaps listed below are minor. None individually would block implementation, but the two marked MUST-FIX must be resolved in the story PR before the PR is marked ready for review.

---

## Approval Note Written To PRD

`EM-APPROVED: 2026-05-21` — Full traceability from five identified root causes to LLD-level signatures; race-safety analysis is complete; backward compatibility hard constraints are enforced by design; bench-gate contract is clear and immutable.

---

## Summary of Review Coverage

| Artifact | Verdict |
|---|---|
| `docs/wiki/v2-performance.md` | Sound problem statement; root-cause table matches live code line numbers |
| `docs/prds/v2-performance.md` | All functional requirements traceable to stories; open questions fully resolved |
| `docs/hld/v2-performance.md` | All five HLD components map to LLD packages; performance budget breakdown present |
| `docs/lld/v2-performance.md` | Exact Go signatures provided; concurrency invariants stated; test strategy complete |
| `docs/epics/lognugget/epic-V2-performance.md` | Exit criteria match PRD Definition of Done |
| Stories P1–P5 | Independently scoped; all ≤ 300 LOC est.; acceptance criteria present |
| Live code (`entry/entry.go`, `config/config.go`, `model/attr.go`, `encoder/*.go`) | Cross-checked; design proposals are compatible with existing implementation |

---

## Gap Analysis

### GAP-1 — MUST-FIX: `enum.LogLevel` is `int` (64-bit); `atomic.Int32` cast is undocumented

**Severity:** Must-fix before P1 PR is marked ready.

**Finding:** `enum.LogLevel` is defined as `type LogLevel int` (verified in `enum/level.go`). On 64-bit platforms `int` is 64 bits. The LLD proposes `atomic.Int32` storing `int32(enum.LogLevel)`. The current maximum level value is `LevelFatal = 1 << 5 = 32`, which fits in `int32`. However:

1. The cast `int32(level)` is a narrowing conversion. If a future story adds a level with value >= 2^31, the cast silently truncates, producing a wrong level gate with no compile-time or runtime warning.
2. The LLD does not call out this invariant. An engineer implementing P1 could reasonably choose `atomic.Int64` (which matches the underlying type width) without violating the LLD, and a different engineer could choose `atomic.Int32` — producing two incompatible implementations across stories.

**Required action (addressed to `agent-architect`):** Add an explicit note to LLD §2.1 stating: "The cast `int32(enum.LogLevel)` is safe because all current and planned level values fit within `[1, 32]`. A compile-time assertion `var _ = [1]struct{}{}[int64(LevelFatal) - math.MaxInt32 + 1]` or a `const _ int32 = int32(LevelFatal)` in `config.go` must be added to make the constraint machine-verified." Alternatively, adopt `atomic.Int64` throughout to match the enum's underlying type width. The choice must be made explicit in the LLD and must be consistent across P1 and any future story that adds log levels.

---

### GAP-2 — MUST-FIX: `PublishLog` RLock is not counted in the "1 RLock per call" invariant

**Severity:** Must-fix — the invariant as stated will mislead engineers implementing P1.

**Finding:** The HLD Component A invariant states: "Hot path: exactly 1 RLock per call." The LLD §9 concurrency table lists `PublishLog` as taking its own separate `configMu.RLock` to snapshot `ch`. This means the hot path (when a message is actually logged) takes 2 RLocks total: one in `GetHotSnapshot()` and one in `PublishLog()`. The LLD table is accurate; the HLD invariant is not.

**Required action (addressed to `agent-architect`):** Correct the HLD Component A invariant to read: "The hot path acquires exactly 1 RLock in `logWithSkip` (via `GetHotSnapshot`). `PublishLog` acquires a second RLock for the channel pointer snapshot. Total across the call: 2 RLocks, down from 6. The filtered path acquires 0 RLocks." This is still a significant improvement and meets the SLO; the inaccurate invariant just risks a P1 implementation that incorrectly moves `ch` into `HotSnapshot` to reduce to 1 total — which would be an over-optimization with its own race risk (channel pointer replacement in `resetConfig`).

---

### GAP-3 — Minor: LLD introduces `KindUint` (scope not in PRD)

**Severity:** Minor — does not block approval; Project Lead must be aware.

**Finding:** The LLD §3.1 defines `KindUint AttrKind = 3` and adds `Uint(key string, val uint64) LogAttr` constructor and a `Uint` method on `*LogEntry`. The PRD F3 functional requirement lists only `Str`, `Int`, `Bool`, `Float64`. The wiki and HLD do not mention `KindUint`. The LLD also includes `KindUint = 3`, which shifts `KindFloat64` to `4` and `KindBool` to `5` — whereas the HLD lists `KindBool = 3` and `KindFloat64 = 4`.

This is a scope creep that is undocumented in the PRD, and the constant ordering diverges between HLD and LLD. The divergence does not cause a correctness problem (constants are internal), but it does create an ambiguity: does P2 deliver `Uint` or not?

**Required action (addressed to `agent-architect`):** Either (a) add `F3a: LogEntry.Uint(key string, val uint64) *LogEntry` to the PRD SHOULD list and align the HLD constant table, or (b) remove `KindUint` from the LLD and defer it to F17 (the COULD list already acknowledges `Uint64`). Constant ordinal values in the LLD must match the HLD exactly to avoid ambiguity during implementation.

---

### GAP-4 — Minor: P4 acceptance criterion understates the expected alloc reduction

**Severity:** Minor cosmetic; correcting it prevents a false-pass scenario.

**Finding:** Story P4 acceptance criterion 1 reads: "allocs/op drops by >= 1 vs the P3 baseline." The HLD and PRD both state that P4 eliminates 2 allocs/event (the `en.Append(nil, e.buf)` allocation and the `append([]byte(nil), Data...)` defensive copy in `PublishLog`). An implementation that removes only one of the two (e.g., eliminates `en.Append` but leaves the defensive copy in `PublishLog`) would pass the story criterion while leaving 1 alloc on the table.

**Required action (addressed to `agent-architect`):** Change story P4 acceptance criterion 1 to: "allocs/op drops by >= 2 vs the P3 baseline (one for the eliminated `en.Append(nil, e.buf)` intermediate buffer, one for the eliminated `append([]byte(nil), Data...)` defensive copy in `PublishLog`). If benchstat shows only 1 alloc dropped, the `PublishLog` defensive copy has not been removed — the PR must not be marked ready."

---

### GAP-5 — Minor: Branch naming inconsistency between Epic and HLD/PRD

**Severity:** Minor operational risk.

**Finding:** The Epic document and the five story files list story branches as:
- `feat/82-p1-atomic-min-level`
- `feat/83-p2-typed-field-api`
- `feat/84-p3-ctx-append-buf`
- `feat/85-p4-inline-framing`
- `feat/86-p5-channel-capacity`

The PRD delivery milestones section and HLD rollout plan show branches as:
- `feat/81-v2-performance/p1-atomic-minlevel`
- `feat/81-v2-performance/p2-typed-field-api`
- etc.

The CLAUDE.md branching convention states sub-task branches must follow the pattern `feat/<n>-<slug>/<task-slug>` (engineer sub-branches off the feature branch). The HLD/PRD convention is consistent with CLAUDE.md. The Epic/story convention is not.

**Required action (addressed to `agent-delivery-manager`):** Align the Epic and story branch names to the `feat/81-v2-performance/<task-slug>` pattern defined in the HLD, the PRD, and CLAUDE.md. The inconsistency, if not resolved, will cause engineers to cut branches from `develop` instead of from `feat/81-v2-performance`, breaking the P1 → P2/P3/P4 dependency chain.

---

### GAP-6 — Minor: `Put()` comment in `entry.go` will become misleading after P4

**Severity:** Minor, but a lying comment is a future bug magnet.

**Finding:** The existing `Put()` doc-comment at `entry/entry.go:68` states: "the encoded bytes have already been passed to `config.PublishLog`, which copies them into a fresh `[]byte` before sending the `LogEvent` onto the dispatch channel (ARCH-7)." After P4 lands, `PublishLog` no longer makes this defensive copy; instead `logWithSkip` severs the alias with `e.buf = make(...)`. The comment will be factually wrong.

**Required action (for P4 story):** Update the `Put()` doc-comment in `entry/entry.go` to reflect the P4 ownership-transfer model. This update should be included in the P4 story's file list and its acceptance criteria.

---

### GAP-7 — Minor: `SetChannelCapacity` thread safety in test environments

**Severity:** Minor, low probability in production; real in test environments.

**Finding:** The LLD §6.2 documents that `SetChannelCapacity` requires no mutex because "it is written before `init()` fires and read inside `resetConfig` at `init()` time." This is correct for production. However, `resetConfig` is also called by the test-only helper `TestResetConfig` in `test_only_helpers.go`. In test code where `TestResetConfig` is called in parallel with `SetChannelCapacity`, a data race on `channelCapacity` exists. The race detector will flag this.

**Required action (for P5 story):** Add a note to the P5 story: "`SetChannelCapacity` must acquire `configMu.Lock()` before writing `channelCapacity` (matching the pattern of all other `Set*` functions in `config.go`), OR the `channelCapacity` variable must use `atomic.Int64`. The LLD rationale about init-ordering is correct for production but not for test code. Recommend the `configMu.Lock()` approach for consistency." This is a testability issue that will surface immediately when the P5 test `Test_SetChannelCapacity_ZeroClamped` calls `resetConfig` after `SetChannelCapacity`.

---

### GAP-8 — Observation: Filtered-path still has one extra nil check vs the HLD data-flow diagram

**Severity:** Observation only; no required action before approval.

**Finding:** In the HLD data-flow diagram (Section 6), step 2 is "config.EventPreProcessors == nil? → e.Put(); return." In the current live code (`entry.go:121-124`), this nil check reads `config.EventPreProcessors` without holding `configMu`. This is an existing behaviour that predates V2 and is not within V2 scope (the pipeline architecture is N1). The LLD does not address it. This is not a V2 regression, but the HLD data-flow diagram implicitly preserves the unsafe read; the P1 story does not audit it. This is tracked for a future story.

---

## PRD Requirements Coverage Check

| Requirement | HLD Component | LLD Section | Story | Status |
|---|---|---|---|---|
| F1 — atomic.Int32 level gate | Component A | §2.1, §2.2 | P1 | Covered (see GAP-1) |
| F2 — single RLock per call | Component A | §2.4, §9 | P1 | Covered (see GAP-2) |
| F3 — Str/Int/Bool/Float64 on LogEntry | Component B | §3.3 | P2 | Covered (see GAP-3 re KindUint) |
| F4 — KindAny backward compat | Component B | §3.1 | P2 | Covered |
| F5 — ContextFieldsAppender zero-alloc path | Component C | §4.2 | P3 | Covered |
| F6 — SetContextFieldsParser legacy path | Component C | §4.2 | P3 | Covered |
| F7 — OpenBytes/CloseBytes on Encoder | Component D | §5.1 | P4 | Covered |
| F8 — logWithSkip inline framing | Component D | §5.4 | P4 | Covered |
| F9 — PublishLog no defensive copy | Component D | §5.4 | P4 | Covered (see GAP-4) |
| F10 — DafaultLogBuffer raised to 1000 | Component E | §6.1 | P5 | Covered |
| F11 — SetChannelCapacity | Component E | §6.2 | P5 | Covered (see GAP-7) |
| F12 — bench files per story | All | §10.2 | All | Covered |
| F13 — bench-check.sh exits 0 | Epic exit criteria | §10.2 | All | Covered |

All 13 MUST requirements are traceable from PRD to HLD component to LLD section to story. No requirement is orphaned.

---

## Story Size and Reviewability Check

| Story | LOC estimate | Independently reviewable? |
|---|---|---|
| P1 | 180 | Yes — no dependency on other V2 stories |
| P2 | 220 | Yes — depends only on P1 (HotSnapshot interface, which is stable after P1 merges) |
| P3 | 170 | Yes — depends only on P1; orthogonal to P2 |
| P4 | 190 | Yes — depends only on P1 (HotSnapshot.EncoderOpen/Close); orthogonal to P2/P3 |
| P5 | 120 | Yes — fully independent |

All stories are within the 300 LOC limit. P1 is gated as the required first merge; P2/P3/P4 may proceed in parallel off the feature branch once P1 merges. P5 is fully independent.

---

## Race-Safety Review

**P4 ownership-transfer correctness:** The LLD §5.5 analysis is correct. The sequence — channel send (copies slice header), `e.buf = make(...)` (severs alias), `e.Put()` (returns fresh slice to pool) — is race-free under the Go memory model. The channel send happens-before the receive; the backing array has no writer after the send; `reset()` operates on the fresh slice only. Verified against the live `reset()` implementation at `entry/entry.go:56-60`. Mandatory `go test -race` on the P4 bench is correctly specified in the story.

**P1 atomic level:** The dual-write under `configMu.Lock()` in `SetMinLevel` and `resetConfig` is sufficient. The TOCTOU window (one log call may observe a stale level during a concurrent `SetMinLevel`) is documented and acceptable. Consistent with zerolog.

**P3 dual-registration:** The `if/else if` structure in `appendContextFields` is deterministic. No race condition on the priority rule.

---

## Security and Compliance Check

- No new external dependencies. Confirmed by design.
- `SetChannelCapacity` max=100,000 OOM guard is present and tested.
- `AppendAttr` KindFloat64 NaN/Inf → `null` is handled (LLD §3.2).
- No new network surfaces, goroutines beyond the existing background dispatcher, or file I/O.

---

## Open Questions for Routing

| ID | Question | Route to | Blocking? |
|---|---|---|---|
| OQ-1 | Resolve GAP-1: adopt `atomic.Int64` OR add compile-time assertion for `int32(LogLevel)` cast safety. | `agent-architect` | Yes — must resolve before P1 story PR is marked ready |
| OQ-2 | Correct HLD Component A "exactly 1 RLock" invariant to reflect the `PublishLog` second RLock (GAP-2). | `agent-architect` | Yes — must resolve before P1 story PR is marked ready |
| OQ-3 | Align Epic + story branch names to `feat/81-v2-performance/<task-slug>` pattern (GAP-5). | `agent-delivery-manager` | Yes — must resolve before feature branch is cut |
| OQ-4 | Decide whether `KindUint` is in or out of P2 scope; align LLD constant ordinals with HLD (GAP-3). | `agent-architect` | No — can resolve in P2 PR review cycle |
| OQ-5 | Fix P4 acceptance criterion 1 to require >= 2 alloc reduction (GAP-4). | `agent-architect` | No — can resolve before P4 PR is opened |
| OQ-6 | Add `configMu.Lock()` (or `atomic.Int64`) to `SetChannelCapacity` to prevent race in test environments (GAP-7). | `agent-architect` | No — can resolve in P5 PR review cycle |

---

## Routing Instructions

The two blocking open questions (OQ-1 and OQ-2) require a targeted amendment to the LLD and HLD respectively. No full re-design is needed. The architect should be able to resolve both with < 20 lines of LLD amendment. Once the architect confirms resolution in writing on this document or the LLD, and `agent-delivery-manager` aligns the branch naming, work may proceed to `agent-project-lead` for story assignment.

Work MUST NOT flow to `agent-project-lead` until OQ-1, OQ-2, and OQ-3 are resolved.

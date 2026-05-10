# QA Wave 1 — Test Plan

Caveman. Tables. Per-PR.

Refs: PRD `docs/prds/lognugget.md`, LLD `docs/lld/lognugget.md`, test framework `docs/lld/test-framework.md`, CLAUDE.md gates.

Cycle cap: 3. Project Lead arbitrates after.

---

## PR #50 — `[008] fix(D-17): drop dead JSONEncoder field`

Branch: `task/12-008-jsonencoder-dead-field`. Story: `001/A-hygiene/008-fix-d17-jsonencoder-dead-field.md`.

Scope: remove `*json.Encoder` field from `JSONEncoder`; keep F7 contract; add factory + regression-guard tests.

### Specialists dispatched

| Specialist | Why |
|---|---|
| qa-blackbox | F7 Write contract preserved; factory error path pinned |
| qa-performance | bench-check.sh gate (LLD §9, NF1) |
| qa-security | reflect-based regression test = no panic vector; no input handling change |

### Test surface

| Area | Cases | Source |
|---|---|---|
| F7 contract | JSON + Text resolve, Write returns wrapped body, error nil | LLD §16 |
| TS-15 | unknown encoder → `ErrUnsupportedEncoderType` (pin current; D-10 deferred) | story 008 |
| Regression guard | `reflect.NumField()==0` after D-17 | story 008 |
| Bench gate | `bench-check.sh` <1µs | CLAUDE.md |

### Severity gates

| Finding | Severity | Block merge? |
|---|---|---|
| F7 contract regression | P0 | yes |
| Factory factory_test gap | P1 | yes |
| Regression-guard red-pre/green-post | P3 | NO — accept (intent of guard) |
| LOC overrun 104 vs 80 (parent tests) | P3 | NO — accept |
| `bench-check.sh` missing | P2 | NO — file new story |

### Pre-arbitration (QA Lead)

- LOC 104 vs 80: ACCEPT. Engineer was instructed not to rewrite parent-supplied tests; impl delta is +17/-7. No Lead arbitration needed.
- Regression-guard test red-pre / green-post: ACCEPT. That is the point of a regression guard; brief wording was loose. Document in PR thread.
- Bench-check.sh absent: NEW STORY (Epic A). Not a PR #50 blocker. Bench gate cannot run; field was never read so allocation delta on `Write` is provably zero.

---

## PR #51 — `[001] TS-01 test/support builders`

Branch: `task/12-001-test-support-builders`. Story: `001/A-hygiene/001-test-support-builders.md`.

Scope: `test/support/{builders.go,fakes.go}` + accompanying `_test.go` files. Test-only. No runtime path.

### Specialists dispatched

| Specialist | Why |
|---|---|
| qa-blackbox | acceptance #1–#8 vs story; LLD §5/§6 conformance |
| qa-security | `defensive copy in FakeWriter.Bytes`, `SpyHook.PublishLogMessage` payload aliasing, `FakePreProc` data-copy |

NOT dispatched: qa-performance (test-only, no hot path); qa-user (no UX surface); db-engineer (no DB).

### Test surface

| Area | Cases | Source |
|---|---|---|
| ConfigBuilder fluent chain | MinLevel/Encoder/Output/AddSource resolve via getters; Build returns cleanup; tb.Cleanup registered | story 001 #1, #7, #8 |
| EntryBuilder | WithFields(n) distinct keys; WithCtx/WithErr round-trip; Build returns *LogEntry | story 001 #2 |
| EventBuilder | Level + Body round-trip into config.LogEvent | story 001 #3 |
| FakeWriter | thread-safe writes; Bytes returns copy; Count = call count | story 001 #4 |
| SpyHook | satisfies config.PublishLogMessageHookContract; FIFO; drop-on-overflow | story 001 #5 |
| FakePreProc | satisfies preProcessingObserverContract structurally; mu-guarded record | story 001 #6 |

### Severity gates

| Finding | Severity | Block merge? |
|---|---|---|
| Acceptance criterion missed | P1 | yes |
| Race in test/support code itself | P0 | yes |
| Race in `config` package (pre-existing) | P2 | NO — file new story |
| LOC overrun 556 vs 300 | P1 | escalate Project Lead |
| `bench-check.sh` absent | P2 | NO — same new story as PR #50 |
| `entry/` pre-existing failures | P3 | NO — pre-existing |

### Pre-arbitration (QA Lead)

- LOC 556 vs 300: ESCALATE to Project Lead. 1.85× cap; pre-approved tests alone are 242 LOC; impl is 130; helpers_test is 50. Engineer cannot trim impl without losing godoc on exported identifiers required by `revive`. Recommendation: accept this once; tighten task-slicer for future stories.
- Config-package data race in `ResetConfig`/`ProcessLogEvent`: NEW STORY (defect register addition, candidate D-19 / behind sync.RWMutex or atomic.Pointer). Not a PR #51 blocker — race lives in production `config`, not in test/support. Pre-existing.
- `bench-check.sh` missing: same NEW STORY as PR #50 covers it.
- `entry/` package pre-existing failures: NOT a PR #51 problem; documented in story TS-04.

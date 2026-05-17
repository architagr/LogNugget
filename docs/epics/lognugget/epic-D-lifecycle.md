# Epic D — Lifecycle & Convenience

Milestone: M4 (2026-06-25 → 2026-07-08).
Owner: Project Lead.
Goal: Stop drain correctness (SC5) + zero-config Shutdown (F31). N-7 noted: SC5 drives `PublishLogMessage` directly to bound test surface to post-proc, not whole channel pipeline.

## In scope

| Group | Refs |
|---|---|
| Stop drain refactor | F30, D-11, D-12, ARCH-7 (no aliasing) |
| Double-buffer atomicity | D-12, F24, NF9 |
| Top-level Shutdown facade | F31, ARCH-5 |
| Zero-config init wiring | F20, SC1 |
| Fatal/Panic doc | N-5 |

## Out of scope

- ch close on Stop (deferred per LLD §9.2; PRD N5).
- v2 `SetOverflowPolicy`, dispatcher `Stop()`.

## Exit criteria

1. `unsetLogEventPostProcessor.Stop` uses `doneCh`; busy-wait removed; SC5 green for `2 × maxBufferSize`.
2. `PublishLogMessage` swap-bucket-inline under lock; D-12 double-write false-green killed.
3. `lognugget` package exports `Shutdown()`; idempotent; `Test_Shutdown_Idempotent` calls Shutdown twice.
4. `lognugget.init` constructs default post-proc, registers on `LevelUnSet`, calls `config.InitPreProcessors`.
5. SC1 integration test green: zero-config Info emits valid JSON line ending `\n`.
6. Doc note added to `Fatal`/`Panic`: `runtime.Goexit()` may exit before dispatcher drains; loss window documented (N-5).

## Stories

| Story | Refs |
|---|---|
| 025 | D-11 / F30 Stop doneCh drain |
| 026 | D-12 PublishLogMessage atomic swap |
| 027 | F31 / ARCH-5 lognugget.Shutdown |
| 028 | SC1 / F20 zero-config init |

## Bench gate notes

- Stop / drain code path is OFF the hot path; bench delta ≈ 0.
- Story 028 must not regress hot-path benches; the init wiring runs once.

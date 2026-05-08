# 027 — F31 + ARCH-5: lognugget.Shutdown facade

Owner-role: engineer.
Refs: F31, ARCH-5, TS-24 (shutdown half), N-4.
Depends: 025.
LOC est: 180.

## Files touched

- NEW `lognugget/lognugget.go` (top-level package; `defaultPostProc` var; `Shutdown()` func)
- NEW `lognugget/shutdown_test.go` (idempotency)

## Tests-first gate

1. `Test_Shutdown_Idempotent` — call `Shutdown()` twice; second call no-op (no panic on closed channels).
2. `Test_Shutdown_DrainsDefaultPostProcessor` — enqueue events; Shutdown; FakeWriter (substituted via support helper) sees all events.
3. `Test_Shutdown_BlocksUntilDrain` — assert `Shutdown()` returns AFTER all writes flushed (synchronous semantic from 025).

## Acceptance

1. `lognugget.Shutdown()` calls `defaultPostProc.Stop()`.
2. Idempotent via `sync.Once`.
3. Doc note re: Fatal/Panic loss window (N-5) on `Shutdown` doc comment.
4. ARCH-5: `defaultPostProc` not exported from `pipeline_stage`; lives here.

## Bench notes

- Off hot path.

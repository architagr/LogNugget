# 006 — Fix D-9: init via sync.Once; ResetConfig test-only

Owner-role: engineer.
Refs: D-9, ARCH-15, NF7, NF8, F7 (TS-15 reset_config_test).
Depends: 004.
LOC est: 220 (includes regression test).

## Files touched

- `config/config.go` (`init`, `ResetConfig`)
- NEW `config/reset_config_test.go`
- `test/support/builders.go` (point `Build()` cleanup at test-only helper)

## Tests-first gate

1. `Test_Init_OneShotChannel` — invoking `init` twice (via `sync.Once.Do` proxy) does not re-create `ch`; goroutine count stable.
2. `Test_ResetConfig_TestOnly` — `ResetConfig` is unexported OR build-tagged for tests; production callers cannot invoke.
3. `Test_Init_DispatcherStartedOnce` — `runtime.NumGoroutine` delta after init = 1.

## Acceptance

1. `config.init` body wrapped in `sync.Once`; channel + dispatcher created exactly once.
2. `config.ResetConfig` either unexported (`resetConfigForTests`) OR file-build-tagged with `//go:build test_only`. Decision: unexported helper available to test/support via internal package access.
3. No exported runtime config-reset surface remains.
4. ARCH-15 documented in code comment.
5. `support.ConfigBuilder.Build()` calls the test-only reset.

## Bench notes

- Hot-path neutral; init code runs once.

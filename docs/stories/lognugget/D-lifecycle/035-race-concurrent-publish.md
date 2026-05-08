# 035 — TS-22: race + concurrent_publish tests

Owner-role: engineer.
Refs: TS-22, NF7, NF9.
Depends: 002, 026.
LOC est: 220.

## Files touched

- NEW `pipeline_stage/concurrent_publish_test.go`
- NEW `test/race/concurrency_test.go`

## Tests-first gate

1. `Test_Race_PostProcessorPublish` — 100 goroutines × 1k publishes; sum(out splits) == 100_000; `-race` clean.
2. `Test_Race_HookRegisterUnderLog` — 100 logger goroutines + 1 register/deregister loop; documents NF7 startup-only contract; `-race` clean.
3. `Test_Race_StopWhileLogging` — callers logging while `Stop()` invoked; drain count == enqueued count; no panic.

## Acceptance

1. CI runs `go test -race ./...`; tests green.
2. NF9 race-freedom verified.
3. NF7 startup-only doc reinforced via test comment (concurrent register IS supported via RWMutex per LLD §5.3 but discouraged; test exists to ensure no race, not endorse pattern).

## Bench notes

- Race overlay slows tests ~5x; acceptable per test-framework §2.

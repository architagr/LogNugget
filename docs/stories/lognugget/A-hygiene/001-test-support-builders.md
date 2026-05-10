# 001 — TS-01: test/support builders

Owner-role: engineer.
Refs: TS-01, T-13, NF7, F2.
Depends: none. First story; unblocks every subsequent test story.
LOC est: 260.

## Files touched

- NEW `test/support/builders.go`
- NEW `test/support/fakes.go` (FakeWriter, SpyHook, FakePreProc)

## Tests-first gate

- `test/support/builders_test.go` — verifies `ConfigBuilder.Build()` returns cleanup that calls `config.ResetConfig` (test-only, OK pre-D-9 fix; story 006 will swap to test-only helper).
- `test/support/fakes_test.go` — `FakeWriter.Bytes/Count` thread-safe; `SpyHook` records `[]byte` ordered (level recorded by `FakePreProc`, not by hooks).

## Acceptance

1. `support.NewConfigBuilder(tb)` chainable: `MinLevel/Encoder/Output/AddSource/Build`.
2. `support.EntryBuilder.WithFields(n)/WithCtx/WithErr/Build`.
3. `support.EventBuilder` produces `config.LogEvent`.
4. `FakeWriter` mu-guarded; `Bytes() []byte`, `Count() int`.
5. `SpyHook` implements `config.PublishLogMessageHookContract` (signature `PublishLogMessage(entry []byte)` — no level arg); chan-backed record.
6. `FakePreProc` implements `config.preProcessingObserverContract`.
7. `Build()` returns `func()` cleanup; registers `tb.Cleanup`.
8. T-13 mitigation: any test using ConfigBuilder is isolated from singleton leak.

## Bench notes

- Test-only code; bench delta = 0.

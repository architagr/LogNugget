# 026 — Fix D-12: PublishLogMessage atomic swap (T-7/T-8 fix)

Owner-role: engineer.
Refs: D-12, F24, NF9, T-7, T-8, TS-20.
Depends: 025.
LOC est: 280.

## Files touched

- `pipeline_stage/unset_log_post_processor_hook.go` (refactor `PublishLogMessage`; swap-bucket-inline under lock; spawn flush goroutine after Unlock)
- REWRITE `pipeline_stage/unset_log_post_processor_hook_test.go` (TS-20 fix T-7 false-green; T-8 sleep flake)
- NEW `pipeline_stage/flush_test.go` (TS-20 F24 table)

## Tests-first gate

1. `Test_PublishLogMessage_AtomicSwap_NoDoubleWrite` — concurrent publishers at maxBucketSize boundary; FakeWriter.Count() exactly equals enqueued count (T-7 false-green killed: was asserting 6, now 3).
2. `Test_PostProcessor_FlushOnSize` — enqueue maxBucketSize events; assert flush triggered exactly once.
3. `Test_PostProcessor_FlushOnTicker` — `assert.Eventually` (NOT `time.Sleep`) for ticker-driven flush at rate=50ms.
4. `Test_PostProcessor_IdleNoFlush` — no events, ticker fires; FakeWriter.Count() == 0.
5. `Test_PostProcessor_ConcurrentPublishers_NoOverflow` — 100 goroutines × 100 events; FakeWriter.Count() == 10_000; no `-race` violation.

## Acceptance

1. `PublishLogMessage` does NOT do `Unlock`+flush+`Lock` mid-method.
2. Under lock: append; if len >= max, swap pointers; release lock; THEN spawn flush goroutine on swapped slice.
3. T-7 expected count corrected from 6 to N (matching write-once semantic).
4. T-8 `time.Sleep(time.Second)` replaced by `assert.Eventually(t, fn, 2*time.Second, 20*time.Millisecond)`.
5. NF9 `-race` green.

## Bench notes

- Dispatcher-side; off caller hot path.

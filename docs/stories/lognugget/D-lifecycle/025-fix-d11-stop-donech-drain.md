# 025 — Fix D-11: Stop doneCh drain (F30/SC5)

Owner-role: engineer.
Refs: D-11, F30, SC5, TS-21, TS-26.
Depends: 002.
LOC est: 280.

## Files touched

- `pipeline_stage/unset_log_post_processor_hook.go` (add `doneCh chan struct{}`; rewrite `Stop()` + `activeBucketWatcher`)
- NEW `pipeline_stage/stop_drain_test.go` (TS-21 unit)
- NEW `test/integration/stop_drain_test.go` (TS-26 SC5 integration: 2 × maxBufferSize)

## Tests-first gate

1. `Test_PostProcessor_StopDrainsActiveBucket` — enqueue 5; Stop; FakeWriter sees 5 writes; no busy-wait.
2. `Test_PostProcessor_StopSynchronous` — Stop returns only AFTER drain complete; assert via `time.Now()` delta vs ticker rate (no `<rate` busy-wait observable).
3. `Test_Integration_StopDrainsAllQueued` — SC5: maxBucketSize=100, enqueue 200 via `PublishLogMessage` direct (per N-7 scoping), Stop, assert FakeWriter.Count() == 200.
4. `Test_PostProcessor_StopIdempotent` — calling Stop twice doesn't panic, second call returns immediately.

## Acceptance

1. `unsetLogEventPostProcessor` has `doneCh chan struct{}`.
2. `Stop()` closes `stopCh`; watcher loop drains active bucket via `flushLogMessages` until len==0; closes `doneCh`; `Stop()` blocks on `doneCh`.
3. No `time.Sleep(rate)` polling.
4. SC5 green; D-11 closed.
5. N-7 scoping documented in test comment: SC5 drives `PublishLogMessage` directly, not full `entry.Info` channel pipeline (channel-side drain not addressed in v1 per LLD §9.2).

## Bench notes

- Off hot path; no bench impact.

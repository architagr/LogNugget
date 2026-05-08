# 036 — TS-25 + TS-27: integration pipeline + hook fan-out (SC8)

Owner-role: engineer.
Refs: TS-25, TS-27, F21, F22, F23, SC8.
Depends: 002, 028.
LOC est: 280.

## Files touched

- NEW `test/integration/pipeline_test.go` (TS-25 SlowHook backpressure)
- NEW `test/integration/hook_fanout_test.go` (TS-27 SC8 a/b/c)

## Tests-first gate

1. `Test_Integration_ChannelBuffersAndBlocks` — F22/F23: register SlowHook; emit N+1 events where N = buffer size (10 + post-proc max); N+1th caller blocks (assertion via goroutine + timeout); release SlowHook; all eventually drain.
2. `Test_Integration_HookFanout_LevelUnSetGetsAll` — SC8a: hook on LevelUnSet receives every level event.
3. `Test_Integration_HookFanout_LevelOnly` — SC8b: hook on LevelInfo receives only Info events.
4. `Test_Integration_HookFanout_DeRegister` — SC8c: deregister mid-stream; subsequent events not delivered.

## Acceptance

1. SC8 green at integration level.
2. F23 backpressure proven: caller blocks; not silent drop.
3. Tests deterministic (no `time.Sleep`; uses SlowHook + recordingChan).

## Bench notes

- Integration only.

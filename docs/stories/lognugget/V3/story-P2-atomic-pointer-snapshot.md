# P2 — atomic.Pointer[HotSnapshot] Copy-on-Write Snapshot

Owner-role: engineer.
Issue: [#113](https://github.com/architagr/LogNugget/issues/113).
Branch: `feat/113-p2-atomic-pointer-snapshot` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: P1 merged to feat/111-v3-performance (touches same `config.go` region; start after P1 to avoid conflict).
LOC est: 200.

## Summary

Replace `GetHotSnapshot()` — which acquires `configMu.RLock()` and copies a 13-field struct — with an atomic pointer load. A package-level `atomic.Pointer[HotSnapshot]` stores the current snapshot; all `Set*` mutators and `resetConfig` rebuild and atomically store a new snapshot under the existing `configMu.Lock()`. Readers call `hotSnapshotPtr.Load()` without any lock.

**Saving:** ~120 ns per log event (the biggest single win in V3 — one RLock + struct copy eliminated).

## Files touched

- `config/config.go`:
  - Add `var hotSnapshotPtr atomic.Pointer[HotSnapshot]`
  - Add `func storeHotSnapshot()` (called under `configMu.Lock()` by all writers)
  - Rewrite `GetHotSnapshot()` to `return *hotSnapshotPtr.Load()`
  - Update `resetConfig` to call `storeHotSnapshot()` after writing `defaultConfig`
  - Update all 10 `Set*` mutators to call `storeHotSnapshot()` after writing their field
- NEW `config/atomic_snapshot_v3_bench_test.go` — `BenchmarkGetHotSnapshot`

## storeHotSnapshot implementation

```go
// storeHotSnapshot builds a new HotSnapshot from current state and stores it
// atomically. Must be called while holding configMu (write lock).
func storeHotSnapshot() {
    snap := &HotSnapshot{
        AddSource:        defaultConfig.addSource,
        TimeFormat:       defaultConfig.timeFormat,
        DefaultFields:    defaultConfig.defaultFields,
        Rendered:         defaultConfig.defaultFieldsRendered,
        StaticFields:     defaultConfig.parsedStaticFields,
        ContextParser:    defaultConfig.contextParser,
        ContextAppender:  defaultConfig.contextAppender,
        Encoder:          defaultConfig.encoderObj,
        EncoderType:      defaultConfig.encoderType,
        RestrictedFields: restrictedFieldsSet,
    }
    snap.EncoderOpen  = snap.Encoder.OpenBytes()
    snap.EncoderClose = snap.Encoder.CloseBytes()
    hotSnapshotPtr.Store(snap)
}
```

Note: `storeHotSnapshot` must be called **after** any field is written to `defaultConfig` and **while still holding `configMu.Lock()`**. This ensures the stored pointer always points to a fully consistent snapshot.

## Tests-first gate

1. `Test_GetHotSnapshot_NilSafe` — after `resetConfig`, `hotSnapshotPtr.Load()` is not nil.
2. `Test_HotSnapshot_ConsistentWithSetMinLevel` — after `SetMinLevel(LevelWarn)`, `GetHotSnapshot().TimeFormat` equals `GetConfig().TimeFormat` AND `GetAtomicMinLevel()` returns `LevelWarn` — snapshot and atomic are both updated.
3. `Test_HotSnapshot_ConsistentWithSetTimeFormat` — after `SetTimeFormat("2006-01-02")`, `GetHotSnapshot().TimeFormat` equals `"2006-01-02"`.
4. `Test_HotSnapshot_RaceWithSetters` — 4 goroutines calling various `Set*` functions concurrently with 4 goroutines calling `GetHotSnapshot()`, under `-race`; no race detected.
5. `BenchmarkGetHotSnapshot` — ≤ 20 ns/op, 0 allocs/op.

## Acceptance criteria

1. `GetHotSnapshot()` acquires `configMu` zero times.
2. `storeHotSnapshot()` is called in all 10 `Set*` mutators and in `resetConfig`.
3. `Test_HotSnapshot_RaceWithSetters` passes under `go test -race`.
4. `BenchmarkGetHotSnapshot`: ≤ 20 ns/op, 0 allocs/op.
5. The old `GetHotSnapshot` implementation (the one with `configMu.RLock()`) is fully removed.
6. All existing `config/` and `entry/` tests pass without modification.

## Assignment

Assigned to: agent-engineer
Branch: feat/113-p2-atomic-pointer-snapshot
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: Write tests FIRST. Do NOT start implementation until P1 is merged to feat/111-v3-performance. The 10 Set* functions that need updating are: SetMinLevel, SetTimeFormat, SetEncoderType, SetAddSource, SetOutput, SetLogBufferMaxSize, SetRate, SetStaticEnvFieldsParser, SetContextFieldsParser, SetContextFieldsAppender, SetDefaultFields, RegisterHook, DeRegisterHook — verify all are updated before PR.

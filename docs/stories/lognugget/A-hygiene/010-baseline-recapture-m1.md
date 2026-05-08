# 010 — M1 baseline recapture (Project Lead only)

Owner-role: N/A (Project Lead).
Refs: NF6, T-9, T-10, T-12, SC2.
Depends: 001..009 merged into feature branch.
LOC est: 50 (mostly bench file fixes + baseline).

## Files touched

- `test/benchmark/entry_benchmark_test.go` (T-9 zerolog removal already in 005; T-10 dual-sink fix; T-12 ctx hoist)
- `bench-baseline.txt` (regenerated)

## Tests-first gate

- N/A; this is the gate update itself. Project Lead reviews `benchstat` output.

## Acceptance

1. T-10: single sink in benchmark (post-proc + FakeWriter, NOT also `SetOutput`).
2. T-12: ctx hoisted outside timed loop; pre-built array indexed.
3. `./.claude/scripts/bench-check.sh` runs clean on the M1 tree.
4. Project Lead invokes `bench-check.sh --update-baseline`; commit references "M1 baseline post-hygiene".
5. New baseline values recorded in commit message.

## Bench notes

- This story IS the bench-baseline mutation. Only Project Lead may run it.
- All subsequent stories compare against THIS baseline.
- Expected delta vs pre-M1: neutral or improved (`Benchmark_ZeroLog` removed; ctx no longer measured).

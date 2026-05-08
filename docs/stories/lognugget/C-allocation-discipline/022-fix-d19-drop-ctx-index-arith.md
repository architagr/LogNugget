# 022 — Fix D-19: drop ctx-data index arithmetic

Owner-role: engineer.
Refs: D-19, F27.
Depends: 021.
LOC est: 100.

## Files touched

- `entry/entry.go` (remove brittle `i+x` indexing; use sequential AppendField calls for ctx-data)

## Tests-first gate

1. `Test_LogEntry_LogWithCtx_NoFields` — len(fields)=0, len(ctxData)=2: emits both ctx fields, no panic.
2. `Test_LogEntry_LogNoCtx_NoFields` — len(fields)=0, ctx=nil: emits no user/ctx fields.
3. `Test_LogEntry_LogCtxAndFields_AllPresent` — both populated: every key present in output (json.Unmarshal count match).

## Acceptance

1. No `i+x` index arithmetic over a shared aggregator.
2. Loop structure: `for fields { AppendField }; if ctxData { for AppendField }`.
3. D-19 closed.

## Bench notes

- Marginal (~10 ns).

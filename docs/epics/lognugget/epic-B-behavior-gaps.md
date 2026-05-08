# Epic B — Behavior Gap Closure

Milestone: M2 (2026-05-28 → 2026-06-10).
Owner: Project Lead.
Goal: F10 RFC 8259 JSON, F17 source capture, F28 RFC3339 default. Bench gate green after.

## In scope

| Group | Refs |
|---|---|
| Time default | F28, D-4 |
| Source capture | F17, D-1, D-7, ARCH-3, ARCH-4 |
| JSON spec | F10, D-2, D-5, D-16, SC4 |
| Numeric render | D-5 (strconv) |
| Collision lookup | D-8, F14 |
| Factory error doc | D-10, F7 |
| Encoder iface change | ARCH-2 |
| Pre-render default-key prefix | ARCH-6 |

## Out of scope

- Pool / pooled-buf strategies (Epic C, F27 SHOULD).
- Stop drain (Epic D).

## Exit criteria

1. `customTime` defaults to RFC3339; F28 test green.
2. `addSource=false` default; F17 emits `caller` when on; SC3 green; D-1 TODO removed.
3. `JSONEncoder` RFC 8259 conformant; `FuzzJSONEncoder` 30s seed pass; SC4 green.
4. Numerics emitted unquoted via `strconv.AppendInt/AppendFloat`.
5. `restrictedFieldsSet` is `map[string]struct{}`; F14 / SC7 green.
6. Encoder iface = `Append(dst, body []byte) []byte` + `Name() string`. Legacy `Write` shim removed by epic close.
7. `defaultFieldsRendered` cache populated on `SetDefaultFields`.
8. Bench gate green: `addSource=off,fields=16` ≤ 1,000 ns; `addSource=on,fields=16` ≤ 1,000 ns. If `addSource=on` ≥ 950 ns, escalate ARCH-8 (pull F27 into M2).

## Stories

| Story | Refs |
|---|---|
| 011 | D-4 / F28 RFC3339 default + test |
| 012 | D-7 / ARCH-4 flip addSource default |
| 013 | D-1 / F17 runtime.Caller wiring |
| 014 | D-8 collision-set map |
| 015 | ARCH-2 encoder iface change |
| 016 | D-2 / F10 JSONEncoder escape |
| 017 | D-5 strconv ParseLogField rewrite |
| 018 | D-16 separator + AppendField |
| 019 | ARCH-6 pre-render default-key prefix |
| 020 | D-10 factory error doc + variant |

## Bench gate notes

- Stage budgets: LLD §3 table.
- Story 013 (F17) increases `addSource=on` by ~250 ns vs off. Ceiling 1,000.
- Story 016+017+018 must NET reduce ns/op by ≥ 200 ns vs M1 baseline (current ~1,246 → target ~720 off / ~970 on per LLD §3.1).
- Story 011 should be alloc-neutral (UTC clamp + format).
- N-1 escalation: if 020 closes with `addSource=on` ≥ 950 ns, Project Lead pulls F27 forward.

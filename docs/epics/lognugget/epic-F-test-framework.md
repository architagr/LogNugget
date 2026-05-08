# Epic F — Test Framework Coverage

Milestone: spans M1–M5 (test work shipped alongside owning code epic).
Owner: Project Lead.
Goal: every TS-* from test-framework.md §18 lands; T-* defects fixed; SC1–SC8 mapped to named tests.

## Routing summary

Test stories ship in same milestone as the production code they cover. Index:

| TS-* | Routed to epic | Story id |
|---|---|---|
| TS-01 support builders | A | 001 |
| TS-02 support doubles | A | 002 |
| TS-03 corpus + golden | A | 003 |
| TS-04 levels_test refactor (T-2, T-4) | A | 009 (with reset test) — see note |
| TS-05 level_gate_test (F6) | B | bundled into 012 (addSource flip) — see note |
| TS-06 source_capture (F17/SC3) | B | with 013 (F17) |
| TS-07 reset_test (F26) | A | 009 |
| TS-08 pool_test (F3/F4/F25) | C | with 023 |
| TS-09 append_field alloc-free | C | with 021 |
| TS-10 parse_log_field types | B | with 017 |
| TS-11 default_fields | B | with 019 |
| TS-12 collision_test (F14/SC7) | B | with 014 |
| TS-13 static_parser | B | bundled into 033 |
| TS-14 context_parser | B | bundled into 033 |
| TS-15 encoder_factory + reset_config | A | with 006/008 |
| TS-16 json_encoder_test | B | with 016 |
| TS-17 json_encoder_fuzz | B | bundled into 034 |
| TS-18 text_encoder + factory | B | with 015 |
| TS-19 pre_processing_stage expand (T-5/T-6) | A | with 007 |
| TS-20 unset_post_processor_hook + flush (T-7/T-8) | D | with 026 |
| TS-21 stop_drain_test (F30) | D | with 025 |
| TS-22 concurrent_publish + race | D | bundled into 035 |
| TS-23 custom_time test (F28) | B | with 011 |
| TS-24 zero_config + shutdown (SC1/F31) | D | with 027/028 |
| TS-25 integration pipeline (F21–F23) | D | bundled into 036 |
| TS-26 integration stop_drain (SC5) | D | with 025 |
| TS-27 integration hook_fanout (SC8) | D | bundled into 036 |
| TS-28 integration collision_prefix (SC7) | B | with 014 |
| TS-29 entry_benchmark rewrite (T-9–T-12) | A | bundled into 010 (baseline) |
| TS-30 parallel/filtered/json bench | C | bundled into 037 |
| TS-31 golden files | A | with 003 |
| TS-32 CI -shuffle=on | E | 029 |
| TS-33 CI coverage threshold | E | 030 |

Bundled stories created where multiple tightly-coupled tests share an LOC budget under 300:

- 033 (B): TS-13 + TS-14 parser tests (~240 LOC)
- 034 (B): TS-17 fuzz + corpus seed (~180 LOC; TS-03 corpus already in A)
- 035 (D): TS-22 race tests (~220 LOC)
- 036 (D): TS-25 + TS-27 integration pipeline + fanout (~280 LOC; ≤ 300)
- 037 (C): TS-30 parallel + filtered + json escape benches (~240 LOC)

## Out of scope

- Mutation testing (DEFERRED-V2).
- goleak (DEFERRED-V2 per test-framework §15).
- p99 instrumentation (DEFERRED-V2).
- synctest (Go 1.24+ — repo floor 1.21).

## Exit criteria

- All 33 TS-* stories merged.
- T-1..T-15 defects closed.
- SC1–SC8 each map to ≥ 1 named test that passes.
- Coverage ≥ 85% per package; CI runs with `-shuffle=on`.
- Bench gate green at every milestone close.

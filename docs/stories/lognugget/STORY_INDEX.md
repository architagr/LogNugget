# LogNugget v1 — Story Index

Umbrella issue: **#12** — https://github.com/architagr/LogNugget/issues/12.
Feature branch: `feat/12-lognugget-v1` (off `develop`).
Total stories: **32** (10 hygiene + 10 behavior-gap + 5 alloc-discipline + 6 lifecycle + 4 release; test work bundled per Epic F routing).
Total epics: 7 (A, B, C, D, E, F-test-framework, G-defect-register).

## Status legend

PENDING / TESTS_DRAFTED / TESTS_APPROVED / IMPL_IN_PROGRESS / DRAFT_PR / READY / MERGED / CLOSED.

## Index

| # | Issue | Title | Epic | Owner-role | Refs | LOC | Deps | Status |
|---|---|---|---|---|---|---|---|---|
| 001 | #13 | TS-01 test/support builders | A | engineer | TS-01, T-13, NF7 | 260 | — | MERGED |
| 002 | #14 | TS-02 test/support doubles | A | engineer | TS-02, T-1 | 200 | 001 | PENDING |
| 003 | #15 | TS-03+TS-31 corpus + golden + files | A | engineer | TS-03, TS-31, SC4 | 220 | 001 | PENDING |
| 004 | #16 | Fix D-14 go.mod 1.21 | A | engineer | D-14, NF11 | 30 | 001 | MERGED |
| 005 | #17 | Fix D-15 relocate demo | A | engineer | D-15, NF12, T-9 | 60 | 004 | PENDING |
| 006 | #18 | Fix D-9 init/ResetConfig test-only | A | engineer | D-9, ARCH-15, NF7, NF8 | 220 | 004 | PENDING |
| 007 | #19 | Fix D-3+D-20 EventPreProcessor singleton | A | engineer | D-3, D-20, T-5, T-6, F18, F19 | 280 | 001, 002 | PENDING |
| 008 | #20 | Fix D-17 JSONEncoder dead field | A | engineer | D-17, F7 | 80 | — | MERGED |
| 009 | #21 | Fix D-18 reset() exhaustive + entry tests | A | engineer | D-18, T-2, T-4, F26 | 280 | 001 | PENDING |
| 010 | #22 | M1 baseline recapture | A | N/A (Project Lead) | NF6, T-10, T-12, SC2 | 50 | 001-009 | PENDING |
| 011 | #23 | Fix D-4 RFC3339 default | B | engineer | D-4, F28, TS-23 | 140 | 001, 010 | PENDING |
| 012 | #24 | Fix D-7+D-13 addSource off | B | engineer | D-7, D-13, F6, R4 | 160 | 001, 010 | PENDING |
| 013 | #25 | Fix D-1 source capture (F17) | B | engineer | D-1, F17, SC3, ARCH-3 | 200 | 012 | PENDING |
| 014 | #26 | Fix D-8 collision-set + SC7 | B | engineer | D-8, F14, SC7 | 280 | 001 | PENDING |
| 015 | #27 | ARCH-2 encoder iface Append | B | engineer | ARCH-2, ARCH-14, F7-F9 | 240 | 008 | PENDING |
| 016 | #28 | Fix D-2 JSON RFC8259 escape | B | engineer | D-2, F10, SC4 | 280 | 015 | PENDING |
| 017 | #29 | Fix D-5 strconv ParseLogField | B | engineer | D-5, F10, NF1 | 260 | 016 | PENDING |
| 018 | #30 | Fix D-16 separator via AppendField | B | engineer | D-16, F10, F12 | 120 | 017 | PENDING |
| 019 | #31 | ARCH-6 pre-render default-key prefix | B | engineer | ARCH-6, F12, F13 | 220 | 014, 017 | PENDING |
| 020 | #32 | Fix D-10 factory error variant | B | engineer | D-10, F7 | 80 | 008, 015 | PENDING |
| 033 | #33 | TS-13+TS-14 static + context parsers | B | engineer | F15, F16 | 240 | 002 | PENDING |
| 034 | #34 | TS-17 FuzzJSONEncoder | B | engineer | TS-17, F10, SC4 | 160 | 003, 016 | PENDING |
| 021 | #35 | Fix D-6 / F27 LogEntry pooled buf | C | engineer | D-6, F27, ARCH-8, NF3 | 280 | 017, 018 | PENDING |
| 022 | #36 | Fix D-19 drop ctx-data index arith | C | engineer | D-19, F27 | 100 | 021 | PENDING |
| 023 | #37 | F4 GenerateInitialPool test | C | engineer | F4, F3, F25 | 200 | 021 | PENDING |
| 024 | #38 | ARCH-7 LogEvent.Data copy semantics | C | engineer | ARCH-7, F21, NF3 | 180 | 021 | PENDING |
| 037 | #39 | TS-30 parallel/filtered/escape benches + M3 baseline | C | engineer + Project Lead | TS-30, R3, NF1, NF3 | 260 | 021, 022, 024 | PENDING |
| 025 | #40 | Fix D-11 Stop doneCh drain | D | engineer | D-11, F30, SC5 | 280 | 002 | PENDING |
| 026 | #41 | Fix D-12 atomic flush swap | D | engineer | D-12, F24, NF9, T-7, T-8 | 280 | 025 | PENDING |
| 027 | #42 | F31 lognugget.Shutdown facade | D | engineer | F31, ARCH-5, N-4 | 180 | 025 | PENDING |
| 028 | #43 | SC1 zero-config init | D | engineer | SC1, F20 | 220 | 027 | PENDING |
| 035 | #44 | TS-22 race + concurrent_publish | D | engineer | TS-22, NF7, NF9 | 220 | 002, 026 | PENDING |
| 036 | #45 | TS-25+TS-27 integration pipeline + fan-out | D | engineer | F21-F23, SC8 | 280 | 002, 028 | PENDING |
| 029 | #46 | TS-32 CI -shuffle + t.Parallel sweep | E | engineer | TS-32, T-13, T-14 | 80 | most | PENDING |
| 030 | #47 | TS-33 CI cover ≥ 85% | E | engineer | TS-33 | 100 | 029 | PENDING |
| 031 | #48 | README + buffered-hook example | E | engineer | NF12, F31, ARCH-10/11/13, N-2/3 | 220 | 027, 028 | PENDING |
| 032 | #49 | release/v1.0.0 cut + tag | E | N/A (Project Lead) | PRD §10, M5 | 30 | 029, 030, 031, all M1-M4 | PENDING |
| 038 | #53 | bench-check.sh path doc fix | A | N/A (PL) | doc | ≤10 | — | DOC-FIXED |
| 039 | #54 | Fix config.ResetConfig race | A | engineer | NF7, NF9, race | 200 | 006 | PENDING |
| 040 | #55 | Story 001 doc drift fix | A | N/A (PL) | doc | ≤30 | — | DONE |

## SC traceability

| SC | Stories that close it |
|---|---|
| SC1 | 028 (zero-config init), 015 (newline) |
| SC2 | 010 (baseline), 037 (final M3 baseline), 032 (final v1 baseline) |
| SC3 | 013 (source capture) |
| SC4 | 016 (RFC8259 escape), 034 (fuzz) |
| SC5 | 025 (Stop drain) |
| SC6 | 029 (-shuffle), CI green |
| SC7 | 014 (collision set), 028 doc |
| SC8 | 007 (unit), 036 (integration) |

## Notes

- Test stories (TS-*) integrated alongside production stories per Epic F routing. Bundled stories: 033 (TS-13+14), 034 (TS-17), 035 (TS-22), 036 (TS-25+27), 037 (TS-30).
- Sequential blockers: 001→002→007/036; 015→016→017→018→019; 021 blocks all of Epic C; 025 blocks 026/027/028; 028 blocks 036; 029→030→031→032.
- Parallel-able first wave: 001, 004, 008.
- ARCH-8 pull-forward trigger: re-evaluate Epic C scope after Epic B closes.

# Epic G — Defect Register Routing

Milestone: cross-cuts M1–M4.
Owner: Project Lead.
Goal: explicit map of every D-* (LLD §12) and T-* (test-framework §17) → owning epic + story.

## Production defects

| ID | Severity | Routed to | Story | Notes |
|---|---|---|---|---|
| D-1 | F17 non-functional (TODO) | B | 013 | feature gap; SC3 |
| D-2 | JSON not RFC 8259 | B | 016 | spec violation; SC4 |
| D-3 | Fake sync.Once | A | 007 | clarity / NF8 |
| D-4 | RFC822 vs RFC3339 | B | 011 | F28 |
| D-5 | fmt.Sprintf in ParseLogField | B | 017 | NF1 / F10 |
| D-6 | bad slice cap | C | 021 | panic risk; F27 |
| D-7 | addSource=true default | B | 012 | NF1 / R4 |
| D-8 | restrictedFields slice O(n) | B | 014 | F14 |
| D-9 | exported ResetConfig + init re-run | A | 006 | NF7/NF8 foot-gun |
| D-10 | encoder factory swallows error | B | 020 | F7 |
| D-11 | Stop busy-wait | D | 025 | SC5 / F30 |
| D-12 | non-atomic flush mid-method | D | 026 | F24 / NF9 |
| D-13 | nil-check ordering | B | bundled into 012 | trivial w/ D-7 fix |
| D-14 | go.mod 1.18 | A | 004 | NF11 |
| D-15 | gin/zerolog in lib go.mod | A | 005 | NF12 |
| D-16 | strings.Join separator | B | 018 | superseded by F10 path |
| D-17 | dead json.NewEncoder field | A | 008 | F7 |
| D-18 | reset() not exhaustive | A | 009 | F26 |
| D-19 | ctx-data index arithmetic | C | 022 | F27 |
| D-20 | hook iter order undocumented | A | bundled into 007 | comment only |

## Test defects

| ID | Routed to | Story |
|---|---|---|
| T-1 time.Sleep flake | A | 002 (doubles cover Eventually pattern) + per-test fixes inline |
| T-2 ParseLogField string-coupled assertions | A | 009 (refactor via 001) |
| T-3 false-green caller assertion | B | 013 (TS-06) |
| T-4 not table-driven | A | 009 |
| T-5 Name() typo | A | 007 |
| T-6 spurious DeRegister | A | 007 |
| T-7 false-green Count()==6 | D | 026 |
| T-8 time.Sleep at rate=1s | D | 026 |
| T-9 zerolog import | A | 005 / 010 (T-9 lives in bench file; relocate w/ demo) |
| T-10 dual-sink in bench | A | 010 |
| T-11 no parallel bench | C | 037 |
| T-12 ctx alloc inside loop | A | 010 |
| T-13 singleton state leak | A | 001 (ConfigBuilder.Build cleanup) |
| T-14 no t.Parallel | E | 029 (TS-32 sweep) |
| T-15 missing coverage | F | (covered across epics) |

## Architecture trade-offs (LLD §13)

| ARCH-* | Routed to | Story |
|---|---|---|
| ARCH-1 singleton | n/a (PRD baseline) | — |
| ARCH-2 encoder iface change | B | 015 |
| ARCH-3 runtime.Caller single-frame | B | 013 |
| ARCH-4 addSource default false | B | 012 |
| ARCH-5 lognugget package | D | 027 |
| ARCH-6 pre-render key prefix | B | 019 |
| ARCH-7 LogEvent.Data copy | C | 024 |
| ARCH-8 F27 conditional pull-fwd | C | (whole epic; trigger at M2 close) |
| ARCH-9 sync.Pool over slab | C | 037 (R3 monitor) |
| ARCH-10 block-on-full | n/a (doc only) | 031 |
| ARCH-11 hook iter order | A | 007 (doc) |
| ARCH-12 ctx parser map | n/a (v2) | — |
| ARCH-13 silent writer drop | n/a (doc) | 031 |
| ARCH-14 newline in encoder | B | 015 |
| ARCH-15 ResetConfig test-only | A | 006 |

## Stop conditions

- Every D-* must close before its epic closes.
- Every T-* must close before its routing story merges.
- ARCH-8 trigger reviewed by Project Lead at M2 close (per N-1).

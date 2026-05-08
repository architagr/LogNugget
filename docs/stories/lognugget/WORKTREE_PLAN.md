# LogNugget v1 — Worktree Plan

Recommended parallel worktrees: **3** (cap; per CLAUDE.md prudence — small library surface, merge cost grows with N).

## First wave — 3 independent stories

| Worktree | Story | Owner | Why parallel |
|---|---|---|---|
| `lognugget-v1-001` | 001 (test/support builders) | engineer-A | foundation; no code-side touch |
| `lognugget-v1-004` | 004 (go.mod 1.21 bump) | engineer-B | tooling-only; one file |
| `lognugget-v1-008` | 008 (D-17 dead json field) | engineer-A or B | encoder-only; isolated |

Three are independent: no overlapping files, no behavior coupling. Engineers can claim 001 + (004 or 008) in pairs.

## Worktree commands

Worktree root: `/Users/architagarwal/code/LogNugget-worktrees/`. Branches off `feat/12-lognugget-v1`.

```bash
# Worktree for story 001
git -C /Users/architagarwal/code/LogNugget worktree add \
  /Users/architagarwal/code/LogNugget-worktrees/lognugget-v1-001 \
  -b feat/12-lognugget-v1/001-test-support-builders feat/12-lognugget-v1

# Worktree for story 004
git -C /Users/architagarwal/code/LogNugget worktree add \
  /Users/architagarwal/code/LogNugget-worktrees/lognugget-v1-004 \
  -b feat/12-lognugget-v1/004-go-mod-1-21 feat/12-lognugget-v1

# Worktree for story 008
git -C /Users/architagarwal/code/LogNugget worktree add \
  /Users/architagarwal/code/LogNugget-worktrees/lognugget-v1-008 \
  -b feat/12-lognugget-v1/008-jsonencoder-dead-field feat/12-lognugget-v1
```

After each story merges into `feat/12-lognugget-v1`:

```bash
# Sync remaining worktrees with the latest feature branch
cd /Users/architagarwal/code/LogNugget-worktrees/lognugget-v1-XXX
git fetch origin && git rebase origin/feat/12-lognugget-v1
```

## Sequential blockers (cannot parallelize)

| Block | Reason |
|---|---|
| 001 → 002 | doubles depend on builders |
| 002 → 007, 025, 026, 035, 036 | doubles needed by hook + drain tests |
| 004 → 005, 006 | go.mod must be 1.21 before module surgery |
| 015 → 016 → 017 → 018 → 019 | encoder iface, then escape, then strconv render, then separator, then prefix cache; each depends on the previous render shape |
| 021 → 022, 023, 024, 037 | all alloc-discipline work depends on pooled buf |
| 025 → 026, 027, 028 | Stop drain primitive needed before atomic flush, Shutdown facade, init wiring |
| 028 → 036 | integration tests need zero-config init to be live |
| 010 → all of Epic B | M1 baseline must be locked before behavior-change benches |
| 037 → all of Epic D's bench-sensitive stories | M3 baseline gate |
| 029 → 030 → 031 → 032 | release sequence |

## Wave plan

### Wave 1 (parallel): 001, 004, 008
Foundational. Closes after 001 lands first; 004/008 land in parallel.

### Wave 2 (sequential after Wave 1): 002, 003, 005, 006, 009
- 002 needs 001.
- 003 needs 001.
- 005 needs 004.
- 006 needs 004.
- 009 needs 001.
Up to 3 worktrees can run 002 + 005 + 009 in parallel (no overlapping files).

### Wave 3 (sequential): 007, 010
- 007 closes Epic A test-side; needs 001+002.
- 010 is Project Lead only; closes after 001-009.

### Wave 4 (Epic B): 011, 012, 014 in parallel; then 013, 015 sequential; then 016→017→018→019; 020 + 033 + 034 in parallel.

### Wave 5 (Epic C): 021 first; then 022/023/024 in parallel; then 037.

### Wave 6 (Epic D): 025 first; then 026/027 in parallel; 028 needs 027; 035 + 036 in parallel after 028.

### Wave 7 (Epic E): 029 → 030 → 031 → 032.

## Worktree teardown

After every story merges, remove the worktree:

```bash
git -C /Users/architagarwal/code/LogNugget worktree remove \
  /Users/architagarwal/code/LogNugget-worktrees/lognugget-v1-XXX
```

Project Lead audits `git worktree list` weekly; stale worktrees are a sign of stalled stories.

## Cap rationale

- 3 worktrees max keeps merge surface small enough that the feature branch doesn't accumulate conflicts faster than reviews close.
- Small library: `entry`, `config`, `encoder`, `pipeline_stage` are the only edited packages. >3 parallel touches concentrated stories are likely to collide on `entry/entry.go` or `config/config.go`.
- Exception: pure-test stories (003, 034, 035, 036) and pure-doc stories (031) are safe to run concurrent with any code story.

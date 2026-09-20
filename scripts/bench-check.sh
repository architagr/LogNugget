#!/usr/bin/env bash
# Performance gate for the sub-1µs SLO.
# - Runs all package benchmarks.
# - Fails if any benchmark's mean across the -count runs exceeds the latency budget
#   (default 1,000 ns/op = 1 µs).
# - Fails when a benchmark is more than BENCH_REGRESSION_TOL slower than ./bench-baseline.txt.
#
# Usage:
#   ./scripts/bench-check.sh                    # run gate
#   ./scripts/bench-check.sh --update-baseline  # accept current numbers as the new baseline (Project Lead only)
#
# Env:
#   BENCH_THRESHOLD_NS   override the per-benchmark hard ceiling (ns/op). Default 1000.
#   BENCH_PACKAGES       override the package selector. Default './...'.
#   BENCH_EXCLUDE_RE     awk-style regex of benchmark names exempt from the hard ceiling.
#                        These still run and are still checked for regressions.
#                        Default: AddSourceTrue|CtxParser
#                          - AddSourceTrue: source capture calls runtime.Callers — inherently > 1 µs.
#                            It is opt-in (SetAddSource) and off by default.
#                          - CtxParser: the deprecated map[string]any context parser, kept only so the
#                            cost of the legacy API stays visible next to SetContextFields. Holding a
#                            deprecated path to the SLO of the supported one would only encourage
#                            deleting the comparison.
#                        Timestamp_Direct was exempt until the benchmarks were given a production-shaped
#                        entry pool; it now runs at ~320 ns/op and is held to the ceiling like the rest.
#   BENCH_REGRESSION_TOL how much slower than the baseline a benchmark may get before the gate
#                        fails, as a multiplier. Default 1.25 (25%) — enough to absorb runner noise
#                        and thermal drift, far below a real regression on this hot path.
#   BENCH_REGRESSION_MIN_NS  a regression must also be at least this many ns/op in absolute terms.
#                        Default 50. why: a contended micro-benchmark like RingBuffer_Push swings
#                        94 → 133 ns between runs — 42%, and 39 ns of a 1,000 ns budget. Without an
#                        absolute floor the gate cries wolf on noise while a real hot-path
#                        regression (25% of ~330 ns = 80+ ns) still trips it.
#
# Benchmarks run with -p 1. why: `go test ./...` runs package test binaries in parallel, so
# benchmarks in different packages compete for the same cores and the same code measures 25-40%
# differently depending on how the packages happen to overlap. Serialising them costs wall-clock
# time and buys numbers that can be compared between runs.

set -euo pipefail

THRESHOLD_NS="${BENCH_THRESHOLD_NS:-1000}"
PACKAGES="${BENCH_PACKAGES:-./...}"
REGRESSION_TOLERANCE="${BENCH_REGRESSION_TOL:-1.25}"
REGRESSION_MIN_NS="${BENCH_REGRESSION_MIN_NS:-50}"
EXCLUDE_RE="${BENCH_EXCLUDE_RE:-AddSourceTrue|CtxParser}"
BASELINE_FILE="bench-baseline.txt"
NEW_FILE="$(mktemp -t bench-new.XXXXXX)"
trap 'rm -f "$NEW_FILE"' EXIT

if ! command -v go >/dev/null 2>&1; then
  echo "bench-check: go is not on PATH" >&2
  exit 2
fi

# No modules → nothing to benchmark; this is acceptable on an empty checkout.
if [ ! -f go.mod ] && [ ! -f go.work ]; then
  echo "bench-check: no go.mod / go.work in $(pwd); skipping"
  exit 0
fi

if [ "${1:-}" = "--update-baseline" ]; then
  echo "bench-check: updating baseline at $BASELINE_FILE"
  go test -tags testing -bench=. -benchmem -count=10 -p 1 -run='^$' "$PACKAGES" | tee "$BASELINE_FILE"
  echo "bench-check: baseline updated. Commit it with the PR that justified the change."
  exit 0
fi

echo "bench-check: running benchmarks (threshold ${THRESHOLD_NS} ns/op = $(echo "scale=3; $THRESHOLD_NS/1000" | bc) µs; excluding ceiling for: ${EXCLUDE_RE:-none})"
go test -tags testing -bench=. -benchmem -count=10 -p 1 -run='^$' "$PACKAGES" | tee "$NEW_FILE"

# Latency hard ceiling check (excluded benchmarks still run; only exempt from the ceiling).
#
# The ceiling applies to each benchmark's mean across the -count=10 runs, not
# to individual iterations. why: a single noisy sample — a GC cycle, another
# process waking up, the laptop throttling — used to fail the gate on an
# unchanged tree. The SLO is a statement about what a log call typically
# costs, and the mean over ten runs is the measurement of that.
violations="$(awk -v thr="$THRESHOLD_NS" -v excl="$EXCLUDE_RE" '
  /^Benchmark/ {
    name=$1
    if (excl != "" && name ~ excl) next
    sum[name] += $3
    n[name]++
  }
  END {
    for (name in sum) {
      mean = sum[name] / n[name]
      if (mean > thr) {
        printf "  %s = %.0f ns/op mean of %d runs (threshold %d)\n", name, mean, n[name], thr
      }
    }
  }
' "$NEW_FILE" | sort)"

if [ -n "$violations" ]; then
  echo "" >&2
  echo "bench-check: FAIL — benchmarks exceed ${THRESHOLD_NS} ns/op:" >&2
  printf "%s\n" "$violations" >&2
  echo "" >&2
  echo "Remediation options:" >&2
  echo "  1. Optimize the path (algorithm, allocations, query plan)." >&2
  echo "  2. Reduce allocations (pooled buffers, escape-precomputation, sync.Pool)." >&2
  echo "  3. Reduce work in the request path (async, batch, precompute)." >&2
  exit 1
fi

# Regression check vs baseline.
#
# why not "benchstat; if it fails": benchstat exits 0 whether or not it found a
# regression — it is a report, not a gate. Pinning the check to its exit code
# meant the regression half of this script never failed anything. The means are
# compared here instead, and benchstat is printed alongside for the detail.
if [ -f "$BASELINE_FILE" ]; then
  if command -v benchstat >/dev/null 2>&1; then
    echo ""
    echo "bench-check: comparing against $BASELINE_FILE"
    benchstat "$BASELINE_FILE" "$NEW_FILE" || true
  else
    echo "bench-check: NOTE — benchstat not installed; the comparison below still runs." >&2
    echo "             install it for per-benchmark detail:" >&2
    echo "             go install golang.org/x/perf/cmd/benchstat@latest" >&2
  fi

  # Mean ns/op per benchmark, baseline vs new. A benchmark must breach both the
  # relative tolerance and the absolute floor to count as a regression.
  regressions="$(awk -v tol="$REGRESSION_TOLERANCE" -v minns="$REGRESSION_MIN_NS" '
    function mean(sum, n) { return n > 0 ? sum / n : 0 }
    FNR == NR {
      if ($1 ~ /^Benchmark/) { base_sum[$1] += $3; base_n[$1]++ }
      next
    }
    $1 ~ /^Benchmark/ { new_sum[$1] += $3; new_n[$1]++ }
    END {
      for (name in new_sum) {
        if (!(name in base_sum)) continue          # new benchmark: nothing to compare
        b = mean(base_sum[name], base_n[name])
        n = mean(new_sum[name], new_n[name])
        if (b <= 0) continue
        if (n > b * tol && n - b > minns) {
          printf "  %s: %.1f ns/op → %.1f ns/op (%+.1f%%)\n", name, b, n, (n / b - 1) * 100
        }
      }
    }
  ' "$BASELINE_FILE" "$NEW_FILE" | sort)"

  if [ -n "$regressions" ]; then
    echo "" >&2
    echo "bench-check: FAIL — benchmarks regressed more than $(awk -v t="$REGRESSION_TOLERANCE" 'BEGIN{printf "%.0f%%", (t-1)*100}') vs $BASELINE_FILE:" >&2
    printf "%s\n" "$regressions" >&2
    echo "" >&2
    echo "Either fix the regression, or — if the new cost is deliberate and justified —" >&2
    echo "the Project Lead re-baselines with: ./scripts/bench-check.sh --update-baseline" >&2
    exit 1
  fi
else
  echo "bench-check: NOTE — no $BASELINE_FILE yet. Project Lead can establish one with --update-baseline." >&2
fi

echo "bench-check: PASS"
exit 0
#!/usr/bin/env bash
# Performance gate for the sub-5ms SLO.
# - Runs all package benchmarks.
# - Fails if any benchmark mean exceeds the latency budget (default 5,000,000 ns/op = 5 ms).
# - Fails on a statistically significant regression vs ./bench-baseline.txt (requires `benchstat`).
#
# Usage:
#   ./scripts/bench-check.sh                    # run gate
#   ./scripts/bench-check.sh --update-baseline  # accept current numbers as the new baseline (Project Lead only)
#
# Env:
#   BENCH_THRESHOLD_NS   override the per-benchmark hard ceiling (ns/op). Default 5000000.
#   BENCH_PACKAGES       override the package selector. Default './...'.

set -euo pipefail

THRESHOLD_NS="${BENCH_THRESHOLD_NS:-5000000}"
PACKAGES="${BENCH_PACKAGES:-./...}"
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
  go test -bench=. -benchmem -count=10 -run='^$' "$PACKAGES" | tee "$BASELINE_FILE"
  echo "bench-check: baseline updated. Commit it with the PR that justified the change."
  exit 0
fi

echo "bench-check: running benchmarks (threshold ${THRESHOLD_NS} ns/op = $((THRESHOLD_NS/1000000)) ms)"
go test -bench=. -benchmem -count=10 -run='^$' "$PACKAGES" | tee "$NEW_FILE"

# Latency hard ceiling check.
violations="$(awk -v thr="$THRESHOLD_NS" '
  /^Benchmark/ {
    name=$1
    nsop=$3 + 0
    if (nsop > thr) {
      printf "  %s = %.0f ns/op (threshold %d)\n", name, nsop, thr
    }
  }
' "$NEW_FILE")"

if [ -n "$violations" ]; then
  echo "" >&2
  echo "bench-check: FAIL — benchmarks exceed ${THRESHOLD_NS} ns/op:" >&2
  printf "%s\n" "$violations" >&2
  echo "" >&2
  echo "Remediation options:" >&2
  echo "  1. Optimize the path (algorithm, allocations, query plan)." >&2
  echo "  2. Add a cache (Redis cross-instance, in-memory LRU single-instance)." >&2
  echo "  3. Reduce work in the request path (async, batch, precompute)." >&2
  exit 1
fi

# Regression check vs baseline.
if [ -f "$BASELINE_FILE" ]; then
  if ! command -v benchstat >/dev/null 2>&1; then
    echo "bench-check: WARN — benchstat not installed; skipping regression compare." >&2
    echo "             install with: go install golang.org/x/perf/cmd/benchstat@latest" >&2
  else
    echo ""
    echo "bench-check: comparing against $BASELINE_FILE"
    if ! benchstat "$BASELINE_FILE" "$NEW_FILE"; then
      echo "bench-check: FAIL — benchstat reported regression(s)." >&2
      exit 1
    fi
  fi
else
  echo "bench-check: NOTE — no $BASELINE_FILE yet. Project Lead can establish one with --update-baseline." >&2
fi

echo "bench-check: PASS"
exit 0
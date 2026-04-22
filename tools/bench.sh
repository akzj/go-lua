#!/usr/bin/env bash
set -euo pipefail

# tools/bench.sh — Run all go-lua benchmarks and format results for commit messages.
#
# Usage:
#   ./tools/bench.sh                     # Run benchmarks, print summary
#   ./tools/bench.sh --compare FILE      # Run benchmarks, compare against previous run
#   ./tools/bench.sh --help              # Show this help
#
# Results are saved to benchmarks/latest.txt (raw) and benchmarks/latest-summary.txt (formatted).

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BENCH_DIR="$REPO_ROOT/benchmarks"
RAW_FILE="$BENCH_DIR/latest.txt"
SUMMARY_FILE="$BENCH_DIR/latest-summary.txt"
COMPARE_FILE=""
COUNT=3

usage() {
    sed -n '3,9s/^# \?//p' "$0"
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --compare)
            COMPARE_FILE="$2"
            shift 2
            ;;
        --count)
            COUNT="$2"
            shift 2
            ;;
        --help|-h)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

mkdir -p "$BENCH_DIR"

# Gather metadata
TIMESTAMP="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
GIT_HASH="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
GIT_BRANCH="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')"
GO_VERSION="$(go version | awk '{print $3}')"

echo "Running benchmarks (count=$COUNT)..."
echo "  commit: $GIT_HASH ($GIT_BRANCH)"
echo "  go:     $GO_VERSION"
echo ""

# Run benchmarks — capture raw output
cd "$REPO_ROOT"
go test -bench=. -benchmem -count="$COUNT" -timeout=10m ./internal/stdlib/ 2>&1 | tee "$RAW_FILE"

echo ""
echo "Raw results saved to $RAW_FILE"
echo ""

# ---------------------------------------------------------------------------
# Parse raw output into a summary table
# ---------------------------------------------------------------------------

format_summary() {
    local raw_file="$1"

    # Extract benchmark lines, compute averages across runs using awk
    awk '
    /^Benchmark/ && /ns\/op/ {
        # Parse: BenchmarkName-N  <iters>  <ns/op>  <B/op>  <allocs/op>
        name = $1
        sub(/-[0-9]+$/, "", name)    # strip -N suffix
        sub(/^Benchmark/, "", name)   # strip Benchmark prefix

        ns = 0; bop = 0; aop = 0
        for (i = 3; i <= NF; i++) {
            if ($(i) == "ns/op")     ns = $(i-1)
            if ($(i) == "B/op")      bop = $(i-1)
            if ($(i) == "allocs/op") aop = $(i-1)
        }

        count[name]++
        sum_ns[name]   += ns
        sum_bop[name]  += bop
        sum_aop[name]  += aop

        # Track insertion order
        if (count[name] == 1) {
            order[++n] = name
        }
    }
    END {
        # Header
        printf "| %-35s | %14s | %12s | %10s |\n", "Benchmark", "ns/op", "B/op", "allocs/op"
        printf "| %-35s | %14s | %12s | %10s |\n", "-----------------------------------", "--------------", "------------", "----------"

        log_sum = 0
        log_count = 0

        for (i = 1; i <= n; i++) {
            name = order[i]
            c = count[name]
            avg_ns  = sum_ns[name]  / c
            avg_bop = sum_bop[name] / c
            avg_aop = sum_aop[name] / c

            printf "| %-35s | %14.1f | %12.0f | %10.0f |\n", name, avg_ns, avg_bop, avg_aop

            if (avg_ns > 0) {
                log_sum += log(avg_ns)
                log_count++
            }
        }

        # Geometric mean of ns/op
        if (log_count > 0) {
            geomean = exp(log_sum / log_count)
            printf "| %-35s | %14s | %12s | %10s |\n", "-----------------------------------", "--------------", "------------", "----------"
            printf "| %-35s | %14.1f | %12s | %10s |\n", "**Geometric mean (ns/op)**", geomean, "-", "-"
        }
    }
    ' "$raw_file"
}

# ---------------------------------------------------------------------------
# Compare two runs (if --compare given)
# ---------------------------------------------------------------------------

format_comparison() {
    local old_file="$1"
    local new_file="$2"

    awk '
    # Process both files — old first (ARGIND==1), new second (ARGIND==2)
    FNR == 1 { filenum++ }
    /^Benchmark/ && /ns\/op/ {
        name = $1
        sub(/-[0-9]+$/, "", name)
        sub(/^Benchmark/, "", name)

        ns = 0; bop = 0; aop = 0
        for (i = 3; i <= NF; i++) {
            if ($(i) == "ns/op")     ns = $(i-1)
            if ($(i) == "B/op")      bop = $(i-1)
            if ($(i) == "allocs/op") aop = $(i-1)
        }

        if (filenum == 1) {
            old_count[name]++
            old_ns[name] += ns
            old_bop[name] += bop
        } else {
            new_count[name]++
            new_ns[name] += ns
            new_bop[name] += bop
            if (new_count[name] == 1) order[++n] = name
        }
    }
    END {
        printf "| %-30s | %12s | %12s | %10s | %10s |\n", "Benchmark", "old ns/op", "new ns/op", "delta", "mem delta"
        printf "| %-30s | %12s | %12s | %10s | %10s |\n", "------------------------------", "------------", "------------", "----------", "----------"

        for (i = 1; i <= n; i++) {
            name = order[i]
            if (old_count[name] > 0 && new_count[name] > 0) {
                o_ns = old_ns[name] / old_count[name]
                n_ns = new_ns[name] / new_count[name]
                o_bop = old_bop[name] / old_count[name]
                n_bop = new_bop[name] / new_count[name]

                if (o_ns > 0) {
                    pct = ((n_ns - o_ns) / o_ns) * 100
                    if (pct < 0)
                        delta = sprintf("%.1f%%", pct)
                    else
                        delta = sprintf("+%.1f%%", pct)
                } else {
                    delta = "N/A"
                }

                if (o_bop > 0) {
                    mem_pct = ((n_bop - o_bop) / o_bop) * 100
                    if (mem_pct < 0)
                        mem_delta = sprintf("%.1f%%", mem_pct)
                    else
                        mem_delta = sprintf("+%.1f%%", mem_pct)
                } else if (n_bop == 0) {
                    mem_delta = "0.0%"
                } else {
                    mem_delta = "N/A"
                }

                printf "| %-30s | %12.1f | %12.1f | %10s | %10s |\n", name, o_ns, n_ns, delta, mem_delta
            } else if (new_count[name] > 0) {
                n_ns = new_ns[name] / new_count[name]
                printf "| %-30s | %12s | %12.1f | %10s | %10s |\n", name, "(new)", n_ns, "—", "—"
            }
        }
    }
    ' "$old_file" "$new_file"
}

# ---------------------------------------------------------------------------
# Build the full summary
# ---------------------------------------------------------------------------

{
    echo "## Benchmark Results"
    echo ""
    echo "- **Date:** $TIMESTAMP"
    echo "- **Commit:** $GIT_HASH ($GIT_BRANCH)"
    echo "- **Go:** $GO_VERSION"
    echo "- **Runs:** $COUNT per benchmark"
    echo ""
    format_summary "$RAW_FILE"
    echo ""

    if [[ -n "$COMPARE_FILE" && -f "$COMPARE_FILE" ]]; then
        echo "### Comparison (old → new)"
        echo ""
        format_comparison "$COMPARE_FILE" "$RAW_FILE"
        echo ""
    fi
} | tee "$SUMMARY_FILE"

echo ""
echo "Summary saved to $SUMMARY_FILE"
echo "Paste the above into your commit/merge message."

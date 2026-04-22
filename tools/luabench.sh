#!/usr/bin/env bash
set -euo pipefail

# luabench.sh — C Lua vs go-lua performance comparison
#
# Runs identical Lua workloads on both C Lua 5.5.1 and go-lua,
# measures os.clock() CPU time, and produces a Markdown comparison table.
#
# Usage: tools/luabench.sh [RUNS]
#   RUNS: number of runs per benchmark (default: 5, uses median)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR="$SCRIPT_DIR/benchmarks"
CLUA="$REPO_ROOT/lua-master/lua"
GOLUA_SRC="$SCRIPT_DIR/luarunner"
GOLUA="$GOLUA_SRC/luarunner"
RUNS="${1:-5}"

# Output directory
OUT_DIR="$REPO_ROOT/benchmarks"
mkdir -p "$OUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTFILE="$OUT_DIR/compare_${TIMESTAMP}.md"

# Colors for terminal
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BOLD}=== C Lua vs go-lua Performance Comparison ===${NC}"
echo ""

# Verify C Lua exists
if [[ ! -x "$CLUA" ]]; then
    echo "ERROR: C Lua not found at $CLUA" >&2
    exit 1
fi

# Build go-lua runner
echo -e "${YELLOW}Building go-lua runner...${NC}"
(cd "$REPO_ROOT" && go build -o "$GOLUA" ./tools/luarunner/)
echo -e "${GREEN}Built: $GOLUA${NC}"
echo ""

# Get git info
GIT_HASH=$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(cd "$REPO_ROOT" && git branch --show-current 2>/dev/null || echo "unknown")

# Collect C Lua version
CLUA_VERSION=$("$CLUA" -v 2>&1 | head -1 || echo "C Lua (unknown version)")

# Sorted list of benchmark files
BENCHMARKS=($(ls "$BENCH_DIR"/*.lua | sort))

# Friendly names (derived from filenames)
friendly_name() {
    local base
    base=$(basename "$1" .lua)
    echo "$base" | sed 's/_/ /g' | sed 's/\b\(.\)/\u\1/g'
}

# Run a single benchmark and return the time (float seconds)
run_one() {
    local runner="$1"
    local script="$2"
    "$runner" "$script" 2>/dev/null
}

# Compute median of sorted values (reads from stdin, one per line)
median() {
    local -a vals
    while IFS= read -r v; do
        vals+=("$v")
    done
    local n=${#vals[@]}
    if (( n == 0 )); then echo "0"; return; fi
    # Sort numerically
    IFS=$'\n' sorted=($(sort -g <<<"${vals[*]}")); unset IFS
    local mid=$(( n / 2 ))
    if (( n % 2 == 1 )); then
        echo "${sorted[$mid]}"
    else
        # Average of two middle values
        awk "BEGIN { printf \"%.6f\", (${sorted[$mid-1]} + ${sorted[$mid]}) / 2 }"
    fi
}

# Arrays to collect results
declare -a NAMES C_TIMES GO_TIMES RATIOS

echo -e "${BOLD}Running $RUNS iterations per benchmark...${NC}"
echo ""

for script in "${BENCHMARKS[@]}"; do
    name=$(friendly_name "$script")
    echo -ne "  ${name}... "

    # Run C Lua
    c_results=()
    for (( i=0; i<RUNS; i++ )); do
        t=$(run_one "$CLUA" "$script")
        c_results+=("$t")
    done
    c_median=$(printf '%s\n' "${c_results[@]}" | median)

    # Run go-lua
    go_results=()
    for (( i=0; i<RUNS; i++ )); do
        t=$(run_one "$GOLUA" "$script")
        go_results+=("$t")
    done
    go_median=$(printf '%s\n' "${go_results[@]}" | median)

    # Compute ratio and convert to ms
    c_ms=$(awk "BEGIN { printf \"%.2f\", $c_median * 1000 }")
    go_ms=$(awk "BEGIN { printf \"%.2f\", $go_median * 1000 }")
    ratio=$(awk "BEGIN { if ($c_median > 0) printf \"%.2f\", $go_median / $c_median; else print \"inf\" }")

    NAMES+=("$name")
    C_TIMES+=("$c_ms")
    GO_TIMES+=("$go_ms")
    RATIOS+=("$ratio")

    # Color the ratio
    if (( $(awk "BEGIN { print ($ratio <= 2.0) }" ) )); then
        color="$GREEN"
    elif (( $(awk "BEGIN { print ($ratio <= 5.0) }" ) )); then
        color="$YELLOW"
    else
        color="$RED"
    fi
    echo -e "${color}${ratio}x${NC}  (C: ${c_ms}ms, go: ${go_ms}ms)"
done

# Compute geometric mean of ratios
geo_mean=$(printf '%s\n' "${RATIOS[@]}" | awk '
    BEGIN { logsum = 0; n = 0 }
    { logsum += log($1); n++ }
    END { if (n > 0) printf "%.2f", exp(logsum / n); else print "0" }
')

echo ""
echo -e "${BOLD}Geometric mean ratio: ${geo_mean}x${NC}"
echo ""

# Write Markdown report
{
    echo "# C Lua vs go-lua Performance Comparison"
    echo ""
    echo "- **Date:** $(date '+%Y-%m-%d %H:%M:%S')"
    echo "- **Branch:** $GIT_BRANCH @ $GIT_HASH"
    echo "- **C Lua:** $CLUA_VERSION"
    echo "- **Runs per benchmark:** $RUNS (median)"
    echo "- **Timing method:** \`os.clock()\` (CPU time, measured inside Lua)"
    echo ""
    echo "## Results"
    echo ""
    echo "| Benchmark | C Lua (ms) | go-lua (ms) | Ratio (go/C) |"
    echo "|-----------|----------:|------------:|-------------:|"

    for (( i=0; i<${#NAMES[@]}; i++ )); do
        printf "| %-35s | %10s | %11s | %12sx |\n" \
            "${NAMES[$i]}" "${C_TIMES[$i]}" "${GO_TIMES[$i]}" "${RATIOS[$i]}"
    done

    echo ""
    printf "| **Geometric Mean** | | | **%sx** |\n" "$geo_mean"
    echo ""
    echo "## Interpretation"
    echo ""
    echo "- **Ratio < 2x**: Competitive with C Lua"
    echo "- **Ratio 2-5x**: Acceptable for a Go implementation"
    echo "- **Ratio > 5x**: Potential optimization target"
    echo ""
    echo "## Benchmark Descriptions"
    echo ""
    echo "| Benchmark | What it tests |"
    echo "|-----------|--------------|"
    echo "| Closure Creation | Closure/upvalue allocation overhead |"
    echo "| Concat Multi | Multi-value string concatenation (a..b..c..d..e..f) |"
    echo "| Concat Operator | Incremental string .. operator (s = s..\"x\" loop) |"
    echo "| Coroutine Create | coroutine.create() overhead |"
    echo "| Coroutine Create Resume Finish | Full coroutine lifecycle |"
    echo "| Coroutine Yield Resume | yield/resume cycle throughput |"
    echo "| Fibonacci | Recursive function calls + arithmetic |"
    echo "| For Loop | Tight numeric for-loop (VM dispatch speed) |"
    echo "| Gc | Allocation pressure + collectgarbage() |"
    echo "| Method Call | Metatable method dispatch (OOP pattern) |"
    echo "| Pattern Match | string.find/gsub pattern matching |"
    echo "| String Concat | tostring() + table.concat() |"
    echo "| Table Ops | Table creation, sequential write, sequential read |"
} > "$OUTFILE"

echo -e "${GREEN}Report saved: $OUTFILE${NC}"
echo ""

# Also save as latest
cp "$OUTFILE" "$OUT_DIR/compare_latest.md"
echo -e "Symlinked: $OUT_DIR/compare_latest.md"

# Print the table to terminal too
echo ""
cat "$OUTFILE"

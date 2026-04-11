#!/usr/bin/env bash
set -euo pipefail

# Conduit Load Test Runner
# Runs Go benchmark-based load tests and reports results.

BENCHTIME="${1:-10s}"

echo "Running Conduit load tests (benchtime=${BENCHTIME})..."
echo "==========================================="
echo ""

# Run parallel benchmarks
go test ./tests/load/ -bench=. -benchtime="$BENCHTIME" -benchmem -cpu=1,2,4 -count=1 2>&1

echo ""
echo "==========================================="
echo "Load test complete."

#!/usr/bin/env bash
set -euo pipefail

# Conduit Test Coverage Check
# Runs tests with coverage and fails if below threshold.

THRESHOLD="${1:-60}"

echo "Running tests with coverage (threshold: ${THRESHOLD}%)..."

go test ./... -coverprofile=coverage.out -count=1 2>&1

# Calculate total coverage
TOTAL=$(go tool cover -func=coverage.out | grep "^total:" | awk '{print $3}' | tr -d '%')

echo ""
echo "Total coverage: ${TOTAL}%"
echo "Threshold:      ${THRESHOLD}%"

# Compare (handle decimals by using bc or awk)
PASS=$(awk "BEGIN {print (${TOTAL} >= ${THRESHOLD}) ? 1 : 0}")

if [[ "$PASS" == "1" ]]; then
  echo "PASS: Coverage meets threshold"
  exit 0
else
  echo "FAIL: Coverage ${TOTAL}% is below threshold ${THRESHOLD}%"
  exit 1
fi

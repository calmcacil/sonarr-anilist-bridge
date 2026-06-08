#!/usr/bin/env bash
set -euo pipefail

# Generate baseline data for integration tests.
# Usage:
#   ./testdata/generate-baseline.sh
#   ./testdata/generate-baseline.sh SPRING 2025

SEASON="${1:-WINTER}"
YEAR="${2:-$(date +%Y)}"

echo "Generating baselines for ${SEASON} ${YEAR}"

for CATEGORY in series series-new; do
  echo "--- ${CATEGORY} ---"
  INTEGRATION=1 go test -v -run TestIntegration_DataPipeline \
    -update \
    -season="${SEASON}" \
    -year="${YEAR}" \
    -category="${CATEGORY}" \
    ./internal/scheduler/
done

echo "Done. Baselines written to internal/scheduler/testdata/${SEASON}_${YEAR}_*.json"

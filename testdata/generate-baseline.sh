#!/usr/bin/env bash
set -euo pipefail

# generate-baseline.sh — Create baseline JSON files for integration tests.
#
# Usage: ./testdata/generate-baseline.sh <SEASON> <YEAR>
#
# Runs the integration test which saves current AniList output to
# internal/scheduler/testdata/ if no baseline exists yet. Delete the
# baseline files first to force regeneration.

SEASON="${1:?usage: $0 <SEASON> <YEAR>}"
YEAR="${2:?usage: $0 <SEASON> <YEAR>}"

cd "$(dirname "$0")/.." || exit 1

export INTEGRATION=1
export PREWARM_YEARS="$YEAR"

# Clean existing baselines to force regeneration
BASEDIR="internal/scheduler/testdata"
rm -f "$BASEDIR"/baseline-*.json

go test -run TestIntegration_DataPipeline ./internal/scheduler/ -v

echo ""
echo "Baselines generated in $BASEDIR/"
ls -la "$BASEDIR"/baseline-*.json 2>/dev/null || echo "(no baseline files found)"

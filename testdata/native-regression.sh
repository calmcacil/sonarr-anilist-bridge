#!/usr/bin/env bash
# native-regression.sh — Run full regression tests natively (no Docker).
#
# Builds both the candidate (current tree) and reference (latest release
# tag) binaries, starts each against live AniList data, exercises all
# endpoints, and compares the output tvdbId sets. Catches regressions in
# filtering, resolution, sorting, or the data pipeline.
#
# Usage:
#   ./testdata/native-regression.sh
#
# Prerequisites: go, curl, jq

set -euo pipefail

cd "$(dirname "$0")/.."

CAND_BIN="/tmp/sab-cand-server"
REF_BIN="/tmp/sab-ref-server"
REF_WORKTREE="/tmp/sab-ref-worktree"

CAND_DATA=$(mktemp -d)
REF_DATA=$(mktemp -d)
CAND_PORT=18081
REF_PORT=18082
CAND_PID=""
REF_PID=""

cleanup() {
  [ -n "$CAND_PID" ] && kill "$CAND_PID" 2>/dev/null || true
  [ -n "$REF_PID" ] && kill "$REF_PID" 2>/dev/null || true
  wait 2>/dev/null || true
  rm -rf "$CAND_DATA" "$REF_DATA"
  if [ -d "$REF_WORKTREE" ]; then
    git worktree remove -f "$REF_WORKTREE" 2>/dev/null || true
  fi
}
trap cleanup EXIT

LATEST_TAG=$(gh release list --limit 1 --json tagName --jq '.[0].tagName' 2>/dev/null || echo "")
if [ -z "$LATEST_TAG" ]; then
  echo "ERROR: could not determine latest release tag. Is gh authenticated?"
  exit 1
fi
echo "=== Latest release tag: $LATEST_TAG ==="

# ── 1. Build candidate ──────────────────────────────────────────────────────
echo "=== Building candidate (current tree) ==="
go build -ldflags="-s -w" -o "$CAND_BIN" ./cmd/server

# ── 2. Build reference from latest tag ───────────────────────────────────────
echo "=== Building reference ($LATEST_TAG) ==="
git worktree add --detach "$REF_WORKTREE" "$LATEST_TAG" 2>&1
(cd "$REF_WORKTREE" && go build -ldflags="-s -w" -o "$REF_BIN" ./cmd/server)
git worktree remove "$REF_WORKTREE"
REF_WORKTREE=""  # prevent double-cleanup

# ── 3. Start candidate ──────────────────────────────────────────────────────
echo ""
echo "=== Starting candidate (port $CAND_PORT) ==="
PORT="$CAND_PORT" \
  CACHE_DB_PATH="$CAND_DATA/cache.db" \
  MAPPING_PATH="$CAND_DATA/mappings.json.zst" \
  PREWARM_YEARS="$(date +%Y)" \
  PREWARM_SEASONS="winter" \
  LOG_LEVEL="info" \
  "$CAND_BIN" &
CAND_PID=$!

# ── 4. Start reference ──────────────────────────────────────────────────────
echo "=== Starting reference (port $REF_PORT) ==="
PORT="$REF_PORT" \
  CACHE_DB_PATH="$REF_DATA/cache.db" \
  MAPPING_PATH="$REF_DATA/mappings.json.zst" \
  PREWARM_YEARS="$(date +%Y)" \
  PREWARM_SEASONS="winter" \
  LOG_LEVEL="info" \
  "$REF_BIN" &
REF_PID=$!

# ── 5. Wait for both to be ready ────────────────────────────────────────────
echo ""
echo "=== Waiting for servers (up to 40s) ==="
for i in $(seq 1 40); do
  cand_ok=0
  ref_ok=0
  curl -sf "http://localhost:${CAND_PORT}/health" >/dev/null 2>&1 && cand_ok=1
  curl -sf "http://localhost:${REF_PORT}/health" >/dev/null 2>&1 && ref_ok=1
  if [ "$cand_ok" -eq 1 ] && [ "$ref_ok" -eq 1 ]; then
    echo "Both ready after ${i}s"
    break
  fi
  if [ "$i" -eq 40 ]; then
    echo "ERROR: Servers failed to start within 40s"
    [ "$cand_ok" -eq 0 ] && echo "  candidate: NOT ready"
    [ "$ref_ok" -eq 0 ] && echo "  reference: NOT ready"
    exit 1
  fi
  sleep 1
done

# Give prewarm a moment (prewarm completes before ListenAndServe).
sleep 2

# ── 6. Health check ───────────────────────────────────────────────────────
echo ""
echo "=== Health check ==="
echo "--- candidate ---"
curl -sf "http://localhost:${CAND_PORT}/health" | python3 -m json.tool
echo "--- reference ---"
curl -sf "http://localhost:${REF_PORT}/health" | python3 -m json.tool

# ── 7. Fetch full output ──────────────────────────────────────────────────
echo ""
echo "=== series ==="
echo "--- candidate ---"
curl -s "http://localhost:${CAND_PORT}/list?season=WINTER&year=$(date +%Y)" \
  | python3 -c "
import sys,json
d=json.load(sys.stdin)
for s in d[:5]:
  print(f'  {s[\"tvdbId\"]}  {s[\"title\"]}')
print(f'  ({len(d)} shows total)')
"
echo "--- reference ---"
curl -s "http://localhost:${REF_PORT}/list?season=WINTER&year=$(date +%Y)" \
  | python3 -c "
import sys,json
d=json.load(sys.stdin)
for s in d[:5]:
  print(f'  {s[\"tvdbId\"]}  {s[\"title\"]}')
print(f'  ({len(d)} shows total)')
"

echo ""
echo "=== series-new ==="
echo "--- candidate ---"
curl -s "http://localhost:${CAND_PORT}/list?season=WINTER&year=$(date +%Y)&category=series-new" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  ({len(d)} shows)')"
echo "--- reference ---"
curl -s "http://localhost:${REF_PORT}/list?season=WINTER&year=$(date +%Y)&category=series-new" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  ({len(d)} shows)')"

# ── 8. Cache stats ───────────────────────────────────────────────────────
echo ""
echo "=== Cache stats ==="
echo "--- candidate ---"
curl -sf "http://localhost:${CAND_PORT}/cache/stats" | python3 -m json.tool
echo "--- reference ---"
curl -sf "http://localhost:${REF_PORT}/cache/stats" | python3 -m json.tool

# ── 9. Backfill trigger ──────────────────────────────────────────────────
echo ""
echo "=== Backfill: /list?season=SPRING&year=$(date +%Y) ==="
echo "--- candidate ---"
curl -s "http://localhost:${CAND_PORT}/list?season=SPRING&year=$(date +%Y)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  backfill: {len(d)} shows')"
echo "--- reference ---"
curl -s "http://localhost:${REF_PORT}/list?season=SPRING&year=$(date +%Y)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  backfill: {len(d)} shows')"

# ── 10. Invalid input ────────────────────────────────────────────────────
echo ""
echo "=== Invalid input ==="
echo "--- candidate ---"
curl -s -w "  HTTP %{http_code}\\n" "http://localhost:${CAND_PORT}/list?season=INVALID&year=2026"
echo "--- reference ---"
curl -s -w "  HTTP %{http_code}\\n" "http://localhost:${REF_PORT}/list?season=INVALID&year=2026"

# ── 11. Save tvdbIds ─────────────────────────────────────────────────────
curl -s "http://localhost:${CAND_PORT}/list?season=WINTER&year=$(date +%Y)" \
  | jq '[.[].tvdbId] | sort' > /tmp/sab-cand-tvdbids.json
curl -s "http://localhost:${CAND_PORT}/list?season=WINTER&year=$(date +%Y)&category=series-new" \
  | jq '[.[].tvdbId] | sort' > /tmp/sab-cand-tvdbids-new.json

curl -s "http://localhost:${REF_PORT}/list?season=WINTER&year=$(date +%Y)" \
  | jq '[.[].tvdbId] | sort' > /tmp/sab-ref-tvdbids.json
curl -s "http://localhost:${REF_PORT}/list?season=WINTER&year=$(date +%Y)&category=series-new" \
  | jq '[.[].tvdbId] | sort' > /tmp/sab-ref-tvdbids-new.json

# ── 12. Graceful shutdown ────────────────────────────────────────────────
echo ""
echo "=== Graceful shutdown ==="
kill "$CAND_PID" "$REF_PID"
wait 2>/dev/null || true
CAND_PID=""
REF_PID=""
echo "Both servers stopped cleanly"

# ── 13. Compare ──────────────────────────────────────────────────────────
echo ""
echo "=== Comparison ==="

series_result=0
new_result=0

echo "--- series ---"
if diff /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json; then
  echo "IDENTICAL to $LATEST_TAG"
else
  ADDED=$(comm -13 /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json | wc -l)
  REMOVED=$(comm -23 /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json | wc -l)
  echo "$ADDED added, $REMOVED removed vs $LATEST_TAG"
  [ "$ADDED" -gt 0 ] || [ "$REMOVED" -gt 0 ] && series_result=1
fi

echo "--- series-new ---"
if diff /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json; then
  echo "IDENTICAL to $LATEST_TAG"
else
  ADDED=$(comm -13 /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json | wc -l)
  REMOVED=$(comm -23 /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json | wc -l)
  echo "$ADDED added, $REMOVED removed vs $LATEST_TAG"
  [ "$ADDED" -gt 0 ] || [ "$REMOVED" -gt 0 ] && new_result=1
fi

echo ""
if [ "$series_result" -eq 0 ] && [ "$new_result" -eq 0 ]; then
  echo "RESULT: Identical to $LATEST_TAG — no regression."
else
  echo "RESULT: Differences detected — review the diff above."
  echo "Minor differences can be expected as AniList data changes over time."
  echo "If the changes are from your code, investigate before merging."
fi

# sonarr-anime-bridge Agent Instructions

Project-specific rules for coding agents working on this repo.

## Before Pull Request

- Run the full regression suite before creating any PR:
  `go vet ./... && go build ./... && go test -race ./...`
- When making behavioral changes, also run side-by-side Docker regression
  tests to confirm no unintended regression. Use default configuration
  (no MAX_PER_SEASON override) so the full seasonal output is fetched.
  Compare the output data (show count and tvdbId list) against a known
  baseline to catch regressions in filtering, resolution, or sorting.
- Only create the PR after all checks pass.

## Docker Regression Tests

These tests verify that the container starts, serves data, and shuts down
correctly — including the concurrency and lifecycle changes that unit tests
can't easily cover.

### Procedure

```bash
# 1. Build the test image
DOCKER_BUILDKIT=1 docker build \
  --build-arg BUILDPLATFORM=linux/arm64 \
  --build-arg TARGETOS=linux \
  --build-arg TARGETARCH=arm64 \
  -t sonarr-anime-bridge:test .

# 2. Run with default config (full production data)
#    Use a temp directory so each run starts fresh.
DATA_DIR=$(mktemp -d)
docker run -d --name sab-regression \
  -v "$DATA_DIR":/data \
  -e PUID="$(id -u)" \
  -e PGID="$(id -g)" \
  -e PREWARM_YEARS="$(date +%Y)" \
  -e PREWARM_SEASONS="winter" \
  -p 18080:8080 \
  sonarr-anime-bridge:test

# 3. Wait for prewarm (adjust sleep if AniList is slow)
sleep 25

# 4. Check health
curl -sf http://localhost:18080/health | python3 -m json.tool

# 5. Capture full output for both categories
curl -s "http://localhost:18080/list?season=WINTER&year=$(date +%Y)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'series: {len(d)} shows'); [print(f'  {s[\"tvdbId\"]}  {s[\"title\"]}') for s in d[:5]]; print(f'  ... and {len(d)-5} more')"

curl -s "http://localhost:18080/list?season=WINTER&year=$(date +%Y)&category=series-new" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'series-new: {len(d)} shows')"

# 6. Cache stats (should show 2 entries after prewarm)
curl -s http://localhost:18080/cache/stats | python3 -m json.tool

# 7. Test non-prewarmed endpoint (backfill trigger)
curl -s "http://localhost:18080/list?season=SPRING&year=$(date +%Y)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'backfill response: {len(d)} shows')"

# 8. Test invalid input
curl -s -w "HTTP %{http_code}\\n" "http://localhost:18080/list?season=INVALID&year=2026"

# 9. Test graceful shutdown (no orphaned goroutines, no panics)
docker stop sab-regression
docker logs sab-regression 2>&1 | grep -E "shutting down|WARN.*goroutine|error|panic" || echo "No warnings — clean shutdown"

# 10. Clean up
docker rm sab-regression
rm -rf "$DATA_DIR"
```

### Data Baseline

Before making changes, record the tvdbId set from the **current released
version** (e.g., `ghcr.io/calmcacil/sonarr-anime-bridge:latest`) as the
reference baseline. Then run the same procedure against your candidate
build and compare:

```bash
# Reference: run against the last released image
REF_DATA=$(mktemp -d)
docker run -d --name sab-ref \
  -v "$REF_DATA":/data \
  -e PUID="$(id -u)" -e PGID="$(id -g)" \
  -e PREWARM_YEARS="$(date +%Y)" -e PREWARM_SEASONS="winter" \
  -p 18082:8080 \
  ghcr.io/calmcacil/sonarr-anime-bridge:latest
sleep 25
curl -s "http://localhost:18082/list?season=WINTER&year=$(date +%Y)" | jq '[.[].tvdbId] | sort' > /tmp/sab-ref-tvdbids.json
curl -s "http://localhost:18082/list?season=WINTER&year=$(date +%Y)&category=series-new" | jq '[.[].tvdbId] | sort' > /tmp/sab-ref-tvdbids-new.json
docker stop sab-ref && docker rm sab-ref && rm -rf "$REF_DATA"

# Candidate: run against the test build (from step 1)
CAND_DATA=$(mktemp -d)
docker run -d --name sab-cand \
  -v "$CAND_DATA":/data \
  -e PUID="$(id -u)" -e PGID="$(id -g)" \
  -e PREWARM_YEARS="$(date +%Y)" -e PREWARM_SEASONS="winter" \
  -p 18083:8080 \
  sonarr-anime-bridge:test
sleep 25
curl -s "http://localhost:18083/list?season=WINTER&year=$(date +%Y)" | jq '[.[].tvdbId] | sort' > /tmp/sab-cand-tvdbids.json
curl -s "http://localhost:18083/list?season=WINTER&year=$(date +%Y)&category=series-new" | jq '[.[].tvdbId] | sort' > /tmp/sab-cand-tvdbids-new.json
docker stop sab-cand && docker rm sab-cand && rm -rf "$CAND_DATA"

# Compare
if diff /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json; then
  echo "series: IDENTICAL to last release"
else
  ADDED=$(comm -13 /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json | wc -l)
  REMOVED=$(comm -23 /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json | wc -l)
  echo "series: $ADDED added, $REMOVED removed vs last release"
fi
if diff /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json; then
  echo "series-new: IDENTICAL to last release"
else
  ADDED=$(comm -13 /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json | wc -l)
  REMOVED=$(comm -23 /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json | wc -l)
  echo "series-new: $ADDED added, $REMOVED removed vs last release"
fi
```

Minor differences are expected as AniList data changes over time. The
comparison catches unintended regressions from logic changes — if a
change to filtering, resolution, or the data pipeline shifts the output
set beyond what upstream data churn explains, investigate before merging.

## Project Commands

- **Build**: `go build ./...`
- **Test**: `go test -race ./...`
- **Lint**: `go vet ./...`
- **Docker build**: `DOCKER_BUILDKIT=1 docker build --build-arg BUILDPLATFORM=linux/arm64 --build-arg TARGETOS=linux --build-arg TARGETARCH=arm64 -t sonarr-anime-bridge:test .`

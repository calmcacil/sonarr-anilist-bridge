# sonarr-anime-bridge Agent Instructions

Project-specific rules for coding agents working on this repo.

## Key Architecture (year-cache + on-the-fly filtering)

- **Cache**: single `year_cache(year)` table stores raw AniList JSON per year
- **Filtering**: `FilterBySeason`, `FilterByFormats`, duration, tags, future-date,
  and first-season filters all applied on-the-fly per request from cached data
- **Resolution**: TVDB IDs resolved on-the-fly via in-memory anibridge mapping
  (no `mapping_version` tracking needed — mapping updates apply immediately)
- **Winter overflow**: December-starting shows from prior year's WINTER season
  merged on WINTER requests. First WINTER request triggers async backfill for
  prior year if not yet cached. Subsequent requests include the overflow shows.
- **No more `PREWARM_SEASONS`** — the server fetches full year data from AniList
  and splits by season locally using the `season` field from the API.

## Before Pull Request

- Run the full regression suite before creating any PR:
  `golangci-lint run ./... && go build ./... && go test -race ./...`
- When making behavioral changes, also run side-by-side regression tests to
  confirm no unintended regression. Use default configuration (no
  MAX_PER_SEASON override) so the full seasonal output is fetched. Compare
  the output data (show count and tvdbId list) against a known baseline to
  catch regressions in filtering, resolution, or sorting.
- Prefer the faster **native** regression test (below) for quick iteration;
  use the **Docker** test for CI-equivalent coverage.
- Only create the PR after all checks pass.

## Native Regression Tests (faster, no Docker needed)

These tests build both the candidate and reference binaries directly from
source and run them outside Docker. Ideal for rapid iteration during
development — no container build, no registry pulls.

### Quick one-shot

```bash
./testdata/native-regression.sh
```

This builds both the current-tree (candidate) and the latest release tag
(reference) binaries, starts each against live AniList data, exercises all
endpoints, and compares the tvdbId output sets. Exit code is zero when
data matches, non-zero when differences are found.

The script handles winter overflow automatically: it makes a warmup WINTER
request that triggers the prior-year backfill, waits for it to complete,
then captures the final output for comparison.

### Manual step-by-step

```bash
# 1. Build candidate binary
go build -ldflags="-s -w" -o /tmp/sab-cand-server ./cmd/server

# 2. Build reference binary from latest release tag
LATEST_TAG=$(gh release list --limit 1 --json tagName --jq '.[0].tagName')
git worktree add --detach /tmp/sab-ref-worktree "$LATEST_TAG"
cd /tmp/sab-ref-worktree
go build -ldflags="-s -w" -o /tmp/sab-ref-server ./cmd/server
cd - && git worktree remove /tmp/sab-ref-worktree

# 3. Start candidate (no PREWARM_SEASONS — fetches full year)
CAND_DATA=$(mktemp -d)
PORT=18081 CACHE_DB_PATH="$CAND_DATA/cache.db" \
  MAPPING_PATH="$CAND_DATA/mappings.json.zst" \
  PREWARM_YEARS="$(date +%Y)" \
  /tmp/sab-cand-server &

# 4. Start reference
REF_DATA=$(mktemp -d)
PORT=18082 CACHE_DB_PATH="$REF_DATA/cache.db" \
  MAPPING_PATH="$REF_DATA/mappings.json.zst" \
  PREWARM_YEARS="$(date +%Y)" \
  /tmp/sab-ref-server &

# 5. Wait for both to be healthy (up to 90s — full-year fetch takes longer)
for i in $(seq 1 90); do
  curl -sf http://localhost:18081/health >/dev/null 2>&1 && \
    curl -sf http://localhost:18082/health >/dev/null 2>&1 && break
  sleep 1
done

# 6. Warmup: trigger winter overflow backfill for prior year
curl -s "http://localhost:18081/list?season=WINTER&year=$(date +%Y)" > /dev/null
for i in $(seq 1 90); do
  entries=$(curl -sf "http://localhost:18081/cache/stats" | python3 -c \
    "import sys,json;print(json.load(sys.stdin)['Entries'])" 2>/dev/null || echo 0)
  [ "$entries" -ge 2 ] && break
  sleep 1
done

# 7-10. Same curl commands as the Docker procedure using both ports
# 11. Graceful shutdown (kill both PIDs)
# 12. Compare tvdbId sets with diff
```

### Data baseline comparison

Both the candidate and reference are built from source, so no external
artifacts are needed. The script uses `gh release list` to find the latest
release tag and checks it out via `git worktree add`.

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
  -p 18080:8080 \
  sonarr-anime-bridge:test

# 3. Wait for prewarm (adjust sleep if AniList is slow)
sleep 25

# 4. Check health
curl -sf http://localhost:18080/health | python3 -m json.tool

# 5. Warmup: trigger winter overflow backfill for prior year
curl -s "http://localhost:18080/list?season=WINTER&year=$(date +%Y)" > /dev/null
for i in $(seq 1 90); do
  entries=$(curl -sf "http://localhost:18080/cache/stats" | python3 -c \
    "import sys,json;print(json.load(sys.stdin)['Entries'])" 2>/dev/null || echo 0)
  [ "$entries" -ge 2 ] && break
  sleep 1
done

# 6. Capture full output for both categories (now with overflow data)
curl -s "http://localhost:18080/list?season=WINTER&year=$(date +%Y)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'series: {len(d)} shows'); [print(f'  {s[\"tvdbId\"]}  {s[\"title\"]}') for s in d[:5]]; print(f'  ... and {len(d)-5} more')"

curl -s "http://localhost:18080/list?season=WINTER&year=$(date +%Y)&category=series-new" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'series-new: {len(d)} shows')"

# 7. Cache stats (should show 2 entries — current + prior year after warmup)
curl -s http://localhost:18080/cache/stats | python3 -m json.tool

# 8. Test non-prewarmed endpoint (backfill trigger)
curl -s "http://localhost:18080/list?season=SPRING&year=$(date +%Y)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'backfill response: {len(d)} shows')"

# 9. Test invalid input
curl -s -w "HTTP %{http_code}\\n" "http://localhost:18080/list?season=INVALID&year=2026"

# 10. Test graceful shutdown (no orphaned goroutines, no panics)
docker stop sab-regression
docker logs sab-regression 2>&1 | grep -E "shutting down|WARN.*goroutine|error|panic" || echo "No warnings — clean shutdown"

# 11. Clean up
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
  -e PREWARM_YEARS="$(date +%Y)" \
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
  -e PREWARM_YEARS="$(date +%Y)" \
  -p 18083:8080 \
  sonarr-anime-bridge:test
sleep 25
# Warmup: trigger winter overflow
curl -s "http://localhost:18083/list?season=WINTER&year=$(date +%Y)" > /dev/null
for i in $(seq 1 90); do
  entries=$(curl -sf "http://localhost:18083/cache/stats" | python3 -c \
    "import sys,json;print(json.load(sys.stdin)['Entries'])" 2>/dev/null || echo 0)
  [ "$entries" -ge 2 ] && break
  sleep 1
done
curl -s "http://localhost:18083/list?season=WINTER&year=$(date +%Y)" | jq '[.[].tvdbId] | sort' > /tmp/sab-cand-tvdbids.json
curl -s "http://localhost:18083/list?season=WINTER&year=$(date +%Y)&category=series-new" | jq '[.[].tvdbId] | sort' > /tmp/sab-cand-tvdbids-new.json
docker stop sab-cand && docker rm sab-cand && rm -rf "$CAND_DATA"

# Compare
if diff /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json; then
  echo "series: IDENTICAL to last release"
else
  ADDED=$(comm -13 /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json 2>/dev/null | wc -l)
  REMOVED=$(comm -23 /tmp/sab-ref-tvdbids.json /tmp/sab-cand-tvdbids.json 2>/dev/null | wc -l)
  echo "series: $ADDED added, $REMOVED removed vs last release"
fi
if diff /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json; then
  echo "series-new: IDENTICAL to last release"
else
  ADDED=$(comm -13 /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json 2>/dev/null | wc -l)
  REMOVED=$(comm -23 /tmp/sab-ref-tvdbids-new.json /tmp/sab-cand-tvdbids-new.json 2>/dev/null | wc -l)
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
- **Lint**: `golangci-lint run ./...` (install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`; PATH may need `$(go env GOPATH)/bin` prepended)
- **Docker build**: `DOCKER_BUILDKIT=1 docker build --build-arg BUILDPLATFORM=linux/arm64 --build-arg TARGETOS=linux --build-arg TARGETARCH=arm64 -t sonarr-anime-bridge:test .`

# Product Spec: Dockerized Sonarr Seasonal Lists Service

## Problem

The existing `anilistgen` is a CLI tool that writes static JSON files to disk,
intended for GitHub Pages deployment. Users who self-host Sonarr on a home
server or NAS need a containerized service that serves the data directly over
HTTP on their internal Docker network.

## Solution

A long-running Go HTTP server packaged as a multi-arch Docker image that:

1. Serves Sonarr-compatible seasonal anime list JSON at `/list`
2. Caches results in SQLite to avoid hammering the AniList API
3. Resolves AniList show IDs to TVDB IDs via the `anibridge/anibridge-mappings`
   database (MAL→TVDB + AniList→TVDB fallback for shows without MAL cross-ref)
4. Prewarms configured years/seasons synchronously at startup
5. Returns empty data immediately on cache miss (so Sonarr can proceed), then
   backfills asynchronously
6. Periodically refreshes cached data (weekly for current year, monthly for past)
7. Prunes cache entries that haven't been requested recently
8. Refreshes the anibridge mapping database daily via conditional HTTP (ETag)

## User Experience

### Deployment

```yaml
# docker-compose.yml (truncated)
services:
  sonarr-seasonal:
    image: ghcr.io/calmcacil/sonarr-anime-bridge:latest
    ports:
      - "8080:8080"
    volumes:
      - seasonal-data:/data
    environment:
      - PUID=${PUID:-1000}
      - PGID=${PGID:-1000}
```

### Sonarr Configuration

Sonarr → Settings → Import Lists → Add → Custom List:

- **URL:** `http://sonarr-seasonal:8080/list?season=all&year=2026`
- Add separate entries for `series-new` if desired:
  `http://sonarr-seasonal:8080/list?season=all&year=2026&category=series-new`

### Expected Behavior

| Scenario | Behavior |
|----------|----------|
| First request for a prewarmed season/year | Returns populated JSON array (fetched synchronously at startup) |
| First request for a non-prewarmed season/year | Returns `[]` (empty array). Triggers async backfill. |
| Subsequent request (backfill complete) | Returns full JSON array of shows |
| Request for stale cached data | Returns cached data. Triggers async refresh. |
| Request for past year (unchanging) | Returns cached data. Refreshes monthly. |
| Entry not requested in > configured days | Pruned from cache. Next request starts fresh. |

### Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/list` | GET | Sonarr import list (query params: season, year, category) |
| `/health` | GET | Liveness check |
| `/cache/stats` | GET | Cache debug stats (entries, hits, misses) |

### Query Parameters for `/list`

| Param | Values | Default |
|-------|--------|---------|
| `season` | `WINTER`, `SPRING`, `SUMMER`, `FALL`, `all` | `all` |
| `year` | any year integer | current year |
| `category` | `series`, `series-new` (excludes prequels/parents) | `series` |

## Configuration (all via environment variables)

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | HTTP listen port |
| `PREWARM_YEARS` | current year | Comma-separated years to fetch on startup |
| `PREWARM_SEASONS` | `all` | Comma-separated: `winter,spring,summer,fall` or `all` |
| `MAX_PER_SEASON` | `100` | Max shows per season from AniList |
| `ALG_ANILIST_INCLUDE_ONA` | `false` | Include ONA format in series |
| `ALG_ANILIST_WINTER_OVERFLOW` | `true` | Merge December premieres from previous year |
| `ALG_ANILIST_EXCLUDE_TAGS` | — | Comma-separated AniList content tags to exclude |
| `AHEAD_MONTHS` | `3` | Max months ahead for future shows (`ALG_ANILIST_AHEAD_MONTHS` also accepted) |
| `CACHE_DB_PATH` | `/data/cache.db` | SQLite file path |
| `CACHE_STALE_DAYS` | `14` | Evict entries not hit in N days |
| `REFRESH_CURRENT_DAYS` | `7` | Refresh interval for current year |
| `REFRESH_PAST_DAYS` | `30` | Refresh interval for past years |
| `ALG_ANILIST_TIMEOUT_MINUTES` | `10` | AniList API timeout |
| `ALG_ANIBRIDGE_MAPPING_PATH` | `/data/anibridge_mappings.json.zst` | Cached anibridge mapping file |
| `ALG_ANIBRIDGE_REFRESH_DAYS` | `1` | How often to check upstream for mapping updates |
| `ALG_ANIBRIDGE_URL` | `https://github.com/anibridge/anibridge-mappings/releases/download/v3/mappings.json.zst` | Upstream anibridge URL |
| `PUID` | `1000` | User ID to drop privileges to (runtime, docker-compose) |
| `PGID` | `1000` | Group ID to drop privileges to (runtime, docker-compose) |
| `LOG_LEVEL` | `info` | Logging level |

## Success Criteria

- [x] `docker compose up` starts the service and serves `/health`
- [ ] First `curl /list?season=WINTER&year=2026` returns populated data (prewarmed) or `[]` (non-prewarmed)
- [ ] Within 60s, same request returns populated JSON array
- [ ] Data format matches Sonarr Custom List expectations (`[{"tvdbId":...,"title":"..."}]`)
- [x] Image built for both `linux/amd64` and `linux/arm64`
- [x] Service survives restart (cache persists via volume)
- [x] Past-year data refreshes monthly, current-year weekly

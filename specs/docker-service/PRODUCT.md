# Product Spec: Dockerized Sonarr Seasonal Lists Service

A long-running Go HTTP server packaged as a multi-arch Docker image that:

1. Serves Sonarr-compatible seasonal anime list JSON at `/list`
2. Caches raw AniList JSON per year in SQLite
3. Resolves AniList IDs to TVDB IDs via `anibridge/anibridge-mappings`
4. Filters on-the-fly per request (season, format, duration, tags, future-date, first-season)
5. Returns `[]` on cache miss, then backfills asynchronously
6. Refreshes stale data (current year daily, past weekly) and prunes cold entries (14 days)
7. Refreshes the anibridge mapping daily via conditional HTTP (ETag)

## Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/list` | GET | Sonarr import list |
| `/health` | GET | Liveness check |
| `/cache/stats` | GET | Cache debug stats (entries, hits, misses) |
| `/cache/clear` | POST | Wipe all cached data |

### `/list` query parameters

| Param | Values | Default |
|-------|--------|---------|
| `season` | `WINTER`, `SPRING`, `SUMMER`, `FALL`, `all` | `all` |
| `year` | any year (clamped to ±10 of current) | current year |
| `category` | `series`, `series-new` (excludes prequels/parents) | `series` |

### Expected behavior

| Scenario | Behavior |
|----------|----------|
| Prewarmed year | Returns populated JSON immediately |
| Non-prewarmed year | Returns `[]`, triggers async backfill |
| Backfill complete | Returns full JSON array |
| Stale cached data | Returns data, triggers async refresh |
| Entry not hit in 14 days | Pruned |
| WINTER + prior year uncached | Triggers prior-year backfill, merges December starts |

## Configuration

All via environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | HTTP listen port |
| `PREWARM_YEARS` | current year | CSV years to fetch at startup |
| `INCLUDE_TYPES` | `TV,ONA` | AniList formats: `TV`, `ONA`, `TV_SHORT`, `OVA`, `SPECIAL`, `MOVIE` |
| `EXCLUDE_TAGS` | — | CSV AniList tags to exclude |
| `FILTER_FUTURE_ENABLED` | `true` | Drop shows >3 months in the future |
| `MAPPING_PATH` | `/data/anibridge_mappings.json.zst` | Cached anibridge mapping |
| `MAPPING_URL` | anibridge release URL | Upstream mapping source |
| `CACHE_DB_PATH` | `/data/cache.db` | SQLite path |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PUID` | `1000` | User ID (Docker only) |
| `PGID` | `1000` | Group ID (Docker only) |

### Hardcoded values

| Parameter | Value |
|-----------|-------|
| AniList HTTP timeout | 30 s |
| Anibridge HTTP timeout | 60 s |
| Winter overflow | always enabled |
| Future filter window | 3 months |
| Cache freshness: current year | 24 h |
| Cache freshness: past years | 7 days |
| Cache eviction | 14 days |
| Mapping refresh | 24 h |

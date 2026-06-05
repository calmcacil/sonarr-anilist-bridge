# Sonarr AniList Bridge

Sonarr-compatible seasonal anime lists from AniList, served as a Docker container
with a built-in HTTP server and SQLite cache.

## Quick start

```bash
docker compose up -d
```

Point Sonarr at `http://localhost:8080/list?season=all&year=2026`.

## Usage

Add a **Custom List** in Sonarr:
```
http://<host>:8080/list?season=all&year=2026
```

### Query parameters

| Param | Values | Default |
|-------|--------|---------|
| `season` | `WINTER`, `SPRING`, `SUMMER`, `FALL`, `all` | `all` |
| `year` | any year | current year |
| `category` | `series`, `series-new` (excludes prequels/parents) | `series` |

If the requested season/year is included in the `PREWARM_YEARS` / `PREWARM_SEASONS`
configuration, data is fetched synchronously at startup — the first request returns
populated data immediately.

For years/seasons *not* covered by prewarm, the first request returns an empty list
and triggers an async backfill. Subsequent requests return populated data once the
backfill completes.

## Configuration

All via environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | HTTP listen port |
| `PREWARM_YEARS` | current year | CSV years to fetch at startup |
| `PREWARM_SEASONS` | `all` | CSV seasons: `winter,spring,summer,fall` |
| `MAX_PER_SEASON` | `100` | Max shows per season |
| `CACHE_DB_PATH` | `/data/cache.db` | SQLite file path |
| `CACHE_STALE_DAYS` | `14` | Evict entries not hit in N days |
| `REFRESH_CURRENT_DAYS` | `7` | Refresh interval, current year |
| `REFRESH_PAST_DAYS` | `30` | Refresh interval, past years |
| `ALG_ANILIST_TIMEOUT_MINUTES` | `10` | API timeout |
| `ALG_ANILIST_INCLUDE_ONA` | `false` | Include ONA |
| `ALG_ANILIST_WINTER_OVERFLOW` | `true` | Merge December premieres |
| `ALG_ANILIST_EXCLUDE_TAGS` | — | Comma-separated tags to exclude |
| `ALG_ANIBRIDGE_MAPPING_PATH` | `/data/anibridge_mappings.json.zst` | Cached anibridge mapping file |
| `ALG_ANIBRIDGE_REFRESH_DAYS` | `1` | How often to check upstream for mapping updates |
| `ALG_ANIBRIDGE_URL` | `https://github.com/anibridge/anibridge-mappings/releases/download/v3/mappings.json.zst` | Upstream anibridge URL |
| `AHEAD_MONTHS` | `3` | Max months ahead for future shows (`ALG_ANILIST_AHEAD_MONTHS` also accepted) |
| `PUID` | `1000` | User ID to drop privileges to (runtime, via docker-compose) |
| `PGID` | `1000` | Group ID to drop privileges to (runtime, via docker-compose) |
| `LOG_LEVEL` | `info` | debug/info/warn/error |

## How it works

1. **Startup**: Server loads the anibridge mapping database, then prewarms the
   configured years/seasons synchronously before accepting requests.
2. **`/list`**: Sonarr hits the endpoint → checks SQLite cache.
3. **Cache hit**: Returns cached JSON array of `[{"tvdbId":...,"title":"..."}]`.
4. **Cache miss** (non-prewarmed data): Returns `[]`, triggers async backfill from AniList.
5. **Backfill pipeline**: Fetches from AniList GraphQL → merges winter overflow → filters
   (duration, blacklist, tags, future dates) → resolves MAL/AniList IDs to TVDB IDs via
   anibridge mapping → stores in SQLite cache.
6. **Background scheduler**: Refreshes stale entries (weekly for current year, monthly for
   past), prunes entries not requested in `CACHE_STALE_DAYS`, and checks for upstream
   mapping updates daily.
7. **Health check**: `GET /health` returns `{"status":"ok"}`.
8. **Debug**: `GET /cache/stats` returns cache hit/miss/entry counts.

## Building

```bash
go build ./cmd/server
```

Multi-arch Docker image published to `ghcr.io` via GitHub Actions on push to main
or tag.

## History

This project was extracted from [`calmcacil/sonarr-seasonal-lists`](https://github.com/calmcacil/sonarr-seasonal-lists)
and supersedes the archived [`calmcacil/sonarr-anime-lists`](https://github.com/calmcacil/sonarr-anime-lists)
(replaced `shinkro/community-mapping` YAML with `anibridge/anibridge-mappings`,
adding AniList→TVDB resolution for ~9,100 additional entries and recovering
previously unresolvable shows).

## Licenses

| Document | Contents |
|---|---|
| [LICENSE](./LICENSE) | MIT License |
| [NOTICE](./NOTICE) | Third-party attribution |

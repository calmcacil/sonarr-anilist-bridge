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
| `category` | `series`, `series-new` | `series` |

The first request for a season/year returns an empty list and triggers an async
backfill. Subsequent requests return populated data.

## Configuration

All via environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | HTTP listen port |
| `PREWARM_YEARS` | current year | CSV years to fetch at startup |
| `PREWARM_SEASONS` | `all` | CSV seasons: `winter,spring,summer,fall` |
| `MAX_PER_SEASON` | `100` | Max shows per season |
| `INCLUDE_ONA` | `false` | Include ONA format |
| `CACHE_DB_PATH` | `/data/cache.db` | SQLite file path |
| `CACHE_STALE_DAYS` | `14` | Evict entries not hit in N days |
| `REFRESH_CURRENT_DAYS` | `7` | Refresh interval, current year |
| `REFRESH_PAST_DAYS` | `30` | Refresh interval, past years |
| `ALG_ANILIST_TIMEOUT_MINUTES` | `10` | API timeout |
| `ALG_ANILIST_INCLUDE_ONA` | `false` | Include ONA |
| `ALG_ANILIST_WINTER_OVERFLOW` | `true` | Merge December premieres |
| `ALG_ANILIST_EXCLUDE_TAGS` | — | Comma-separated tags to exclude |
| `LOG_LEVEL` | `info` | debug/info/warn/error |

## How it works

1. Sonarr hits `/list` → checks SQLite cache
2. Cache miss → returns `[]`, triggers async backfill from AniList
3. Cache hit → returns cached JSON array of `[{"tvdbId":...,"title":"..."}]`
4. Background scheduler refreshes stale entries (weekly for current year, monthly for past)
5. Entries not requested in `CACHE_STALE_DAYS` are pruned

## Building

```bash
go build ./cmd/server
```

Multi-arch Docker image published to `ghcr.io` via GitHub Actions on push to main
or tag.

## Source

Extracted from [`calmcacil/sonarr-seasonal-lists`](https://github.com/calmcacil/sonarr-seasonal-lists)
at commit [`9326e7c`](https://github.com/calmcacil/sonarr-seasonal-lists/commit/9326e7c8b0edddb8ed9d2f84c9bab2498ac0eb96)
on the `docker` branch. The parent repository hosts the original static-file CLI
generator (and its weekly-sync workflow); this repository continues the Dockerized
HTTP server variant independently.

## Licenses

| Document | Contents |
|---|---|
| [LICENSE](./LICENSE) | MIT License |
| [NOTICE](./NOTICE) | Third-party attribution |

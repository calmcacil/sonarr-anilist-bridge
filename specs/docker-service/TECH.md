# Tech Spec: Dockerized Sonarr Seasonal Lists Service

## Architecture

```
cmd/server/main.go
  ├── internal/config/        → env-var configuration (no YAML/CLI)
  ├── internal/cache/         → SQLite year-cache (modernc.org/sqlite)
  ├── internal/scheduler/     → pipeline + background refresh goroutines
  ├── internal/anilist/       → AniList GraphQL client (paginated, rate-limited)
  ├── internal/filter/        → on-the-fly filtering (season, format, duration, tags, future)
  └── internal/mapping/       → TVDB ID resolver (anibridge, atomic.Pointer)
```

**Key design**: The server fetches full-year data from AniList (all seasons, all
formats) and caches the raw JSON per year in `year_cache(year)`. Filtering and
TVDB resolution happen on-the-fly per request. No per-season or per-category
cache entries.

## Core Pipeline

```
request → db.GetYear(year)
  ├─ MISS → trigger async FetchAndStore(year) → return []
  ├─ WINTER + prior year missing → trigger async FetchAndStore(year-1)
  └─ HIT → sched.Process(rawData, season, year, category)
       ├─ Unmarshal raw JSON
       ├─ Winter overflow merge (December starts from prior year)
       ├─ FilterBySeason → select matching season
       ├─ FilterByFormats → keep configured formats
       ├─ Filter → exclude by duration ≤10 min and tags
       ├─ FilterFuture(3) → exclude shows >3 months out
       ├─ FilterFirstSeason → exclude prequels/parents (series-new only)
       ├─ ResolveBatch → AniList IDs → TVDB IDs
       └─ Marshal → JSON response
```

## Component Details

### `internal/config/`

Pure `os.Getenv`. All values validated/clamped on load.

| Field | Env Var | Default |
|-------|---------|---------|
| `Port` | `PORT` | `8080` (clamped 1–65535) |
| `PrewarmYears` | `PREWARM_YEARS` | `[current year]` |
| `MaxPerSeason` | `MAX_PER_SEASON` | `100` (clamped 1–500) |
| `IncludeTypes` | `INCLUDE_TYPES` | `["TV", "ONA"]` |
| `ExcludeTags` | `EXCLUDE_TAGS` | `nil` |
| `FilterFutureEnabled` | `FILTER_FUTURE_ENABLED` | `true` |
| `CacheDBPath` | `CACHE_DB_PATH` | `/data/cache.db` |
| `LogLevel` | `LOG_LEVEL` | `"info"` |
| `AnibridgeMappingPath` | `MAPPING_PATH` | `/data/anibridge_mappings.json.zst` |
| `AnibridgeURL` | `MAPPING_URL` | anibridge release URL |

### `internal/cache/`

Pure-Go SQLite (WAL mode). One row per year:

```sql
CREATE TABLE year_cache (year INTEGER PRIMARY KEY, data BLOB, fetched_at INTEGER, last_hit INTEGER DEFAULT 0);
```

Freshness: 24 h for current year, 7 days for past years. Hit/miss counts via
`atomic.Int64`. Key methods: `GetYear`, `SetYear`, `HasYear`, `Clear`,
`NeedsRefreshYears`, `PruneStaleYears`, `Stats`, `Ping`.

### `internal/scheduler/`

Owns the fetch → cache → filter → resolve pipeline. Background goroutines:

- **Stale refresh** (every 10 min): refreshes years stale beyond 1 day (current) or 7 days (past), prunes entries with last_hit >14 days old, vacuums SQLite.
- **Mapping refresh** (every 24 h): HEAD-checks upstream anibridge ETag, downloads if changed, swaps atomically.

In-flight deduplication: `sync.Map` prevents concurrent fetches for the same year.
Panic recovery and context-cancellation-aware in all goroutines.

### `internal/anilist/`

Paginated GraphQL client. 50 results/page. Rate limiting: 700 ms +
jitter between requests, 5 s backoff after 429. 5 retries with exponential
backoff. `FetchYear` returns all anime for a year regardless of season/format.

Show predicates (used by filters): `IsSeries`, `IsNew`, `SkipByDuration`,
`HasTag`, `IsWithinMonths`, `IsWinterStart`, `DisplayTitle`.

### `internal/filter/`

All filtering is on-the-fly from cached raw data. Functions:
`FilterBySeason` (with month-based fallback for empty season field),
`FilterByFormats`, `Filter` (duration + tags), `FilterFuture`, `FilterFirstSeason`.

### `internal/mapping/`

Zstd-compressed JSON mapping (~8 MB compressed). `LoadOrFetch` uses conditional
HTTP: HEAD for ETag match, full download on change, fallback to cache on error.
MD5 verification against `x-ms-blob-content-md5` header. Atomic temp-file writes.
TVDB extraction prefers `s1` scope, falls back to highest episode count.

Resolver uses `atomic.Pointer[AnibridgeMapping]` — mapping swaps don't block
in-flight lookups. Resolution order: MAL first, AniList fallback.

### `cmd/server/main.go`

Stdlib `net/http`. Startup: load config → open cache → load resolver → prewarm
configured years (blocking) → start background goroutines → listen.

Graceful shutdown: cancel context → `server.Shutdown(10s)` → `sched.Wait(5s)`.

Endpoints: `/list`, `/health`, `/cache/stats`, `/cache/clear`. Middleware:
logging (method, path, status, duration) and panic recovery.

## Docker

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
COPY . . && RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates su-exec wget
COPY --from=builder /server /server
COPY entrypoint.sh /entrypoint.sh
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s CMD wget --spider http://localhost:8080/health || exit 1
ENTRYPOINT ["/entrypoint.sh"]
```

CI builds `linux/amd64` and `linux/arm64`.

## Dependencies

| Direct | Purpose |
|--------|---------|
| `modernc.org/sqlite` | Pure-Go SQLite |
| `github.com/klauspost/compress/zstd` | Zstd decompression |

(Plus indirect transitive deps from sqlite.)

## File Layout

| Path | Purpose |
|------|---------|
| `cmd/server/main.go` | HTTP server entrypoint |
| `internal/config/config.go` | Env-var config |
| `internal/cache/cache.go` | SQLite year cache |
| `internal/scheduler/scheduler.go` | Pipeline + background workers |
| `internal/anilist/anilist.go` | AniList GraphQL client |
| `internal/filter/filter.go` | On-the-fly filtering |
| `internal/mapping/anibridge.go` | Mapping loader/parser |
| `internal/mapping/resolve.go` | TVDB resolver |
| `internal/testutil/testutil.go` | Shared test helpers |
| `entrypoint.sh` | PUID/PGID privilege drop |
| `Dockerfile` | Multi-stage multi-arch build |
| `docker-compose.yml` | Quick-start composition |

# Tech Spec: Dockerized Sonarr Seasonal Lists Service

## Architecture

```
cmd/server/main.go
  ├── internal/config/        → env-var configuration (env-only, no YAML/CLI)
  ├── internal/cache/         → SQLite cache layer via modernc.org/sqlite
  ├── internal/scheduler/     → background refresh + mapping refresh goroutines
  ├── internal/anilist/       → AniList GraphQL client (paginated, rate-limited)
  ├── internal/filter/        → show filtering (duration, blacklist, tags, future)
  └── internal/mapping/       → TVDB ID resolver using anibridge/anibridge-mappings
       ├── anibridge.go       → zstd JSON parser, HTTP conditional fetch, sidecar metadata
       └── resolve.go         → Resolver with atomic.Pointer mapping swap
```

The CLI entry point (`cmd/anilistgen/`) was removed on extraction.
`internal/logging/` and `internal/output/` were also removed (logging uses
stdlib `log/slog`; JSON output is served directly from cache).

## Component Details

### `internal/config/` (Adapted)

Strip YAML loading, file search paths, `init-config`, `validate`, and CLI flag
support. Replace with pure environment variable loading via `os.Getenv`.

```go
type Config struct {
    Port               int
    PrewarmYears       []int
    PrewarmSeasons     []string
    MaxPerSeason       int
    IncludeONA         bool
    WinterOverflow     bool
    AheadMonths        *int
    ExcludeTags        []string
    CacheDBPath        string
    CacheStaleDays     int
    RefreshCurrentDays int
    RefreshPastDays    int
    AniListTimeoutMin  int
    LogLevel           string

    // AniBridge mapping
    AnibridgeMappingPath string
    AnibridgeRefreshDays int
    AnibridgeURL         string
}
```

### `internal/cache/` (Existing)

Pure-Go SQLite via `modernc.org/sqlite` (no CGO, easy cross-compilation).

Schema:
```sql
CREATE TABLE IF NOT EXISTS season_cache (
    season    TEXT NOT NULL,
    year      INTEGER NOT NULL,
    category  TEXT NOT NULL,
    data      BLOB NOT NULL,
    is_empty  INTEGER NOT NULL DEFAULT 0,
    fetched_at INTEGER NOT NULL,
    last_hit  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (season, year, category)
);
```

Exported API:
```go
type Cache struct { ... }

func Open(path string) (*Cache, error)
func (c *Cache) Close() error
func (c *Cache) Get(season string, year int, category string) (data []byte, fresh bool, isPending bool, ok bool)
func (c *Cache) Set(season string, year int, category string, data []byte) error
func (c *Cache) SetEmptyIfNotExists(season string, year int, category string) (bool, error)
func (c *Cache) MarkHit(season string, year int, category string) error
func (c *Cache) Exists(season string, year int, category string) bool
func (c *Cache) PruneStale(staleDays int) (int, error)
func (c *Cache) NeedsRefresh(currentYear int, currentRefreshDays, pastRefreshDays int) ([]CacheKey, error)
func (c *Cache) Stats() CacheStats
```

### `internal/scheduler/` (Existing)

Before the background goroutine starts, the scheduler runs a synchronous Prewarm
for configured years/seasons.

The background goroutine ticks periodically and:

1. Queries cache for entries needing refresh (every 10 min)
2. For each: fetches from AniList → filters → resolves → updates cache
3. Refreshes the anibridge mapping from upstream (every 24 h, configurable via ALG_ANIBRIDGE_REFRESH_DAYS)

```go
type Scheduler struct { ... }

func New(cache *cache.Cache, cfg *config.Config) *Scheduler
func (s *Scheduler) LoadResolver()                          // sync load of anibridge mapping before server starts
func (s *Scheduler) Prewarm(ctx context.Context) error      // synchronous warmup
func (s *Scheduler) StartBackground(ctx context.Context)    // launches background goroutines
func (s *Scheduler) FetchAndStore(ctx context.Context, season string, year int, category string) error // backfill on cache miss
func (s *Scheduler) Refresh(ctx context.Context, season string, year int, category string) error
```

### `internal/mapping/anibridge.go` (New)

The `LoadOrFetch` function replaces the old `shinkro/community-mapping` YAML
loader. Key design:

- **Zstd-compressed JSON** — the anibridge dataset is ~8 MB compressed, ~80 MB
  uncompressed. Parsed with `github.com/klauspost/compress/zstd`.
- **Conditional HTTP** — a sidecar metadata file (`.meta.json`) stores ETag,
  MD5, and key snapshots. On each load, a HEAD request checks the upstream
  ETag; a match short-circuits the download.
- **MD5 verification** — GitHub Release assets expose `x-ms-blob-content-md5`;
  downloads are verified against this header. Mismatches are reported as errors.
- **Key snapshots** — after each successful download, the MAL and AniList key
  sets are persisted. On subsequent updates, a diff is logged:
  `Updated anibridge database, 12 new, 3 removals, 18091 total entries`.

```go
func LoadOrFetch(ctx context.Context, path, url string) (*AnibridgeMapping, Metadata, error)
func Head(ctx context.Context, url string) (Metadata, error)
func Fetch(ctx context.Context, url string) ([]byte, Metadata, error)
```

### `internal/mapping/resolve.go` (New)

The `Resolver` wraps the parsed `AnibridgeMapping` and provides safe concurrent
access via `sync/atomic.Pointer`:

- Prefers MAL→TVDB lookups
- Falls back to AniList→TVDB when a show has no MAL ID or MAL lookup misses
- `SetMapping` swaps the underlying pointer without blocking in-flight lookups

```go
type Resolver struct { mapping atomic.Pointer[AnibridgeMapping] }

func NewResolver() *Resolver
func (r *Resolver) SetMapping(m *AnibridgeMapping)
func (r *Resolver) Mapping() *AnibridgeMapping
func (r *Resolver) Resolve(s anilist.Show) (int, bool)
func (r *Resolver) ResolveBatch(shows []anilist.Show) map[int]ResolvedShow
```

### `cmd/server/main.go` (Existing)

HTTP server using `net/http` (stdlib, no framework):

- `GET /list` — handler that calls cache.Get, triggers async backfill on miss
  for non-prewarmed data, returns JSON
- `GET /health` — returns `{"status":"ok"}`
- `GET /cache/stats` — returns cache stats JSON (debug endpoint)
- Synchronous prewarm on startup (blocking, before `ListenAndServe`)
- Graceful shutdown on SIGTERM/SIGINT
- Structured logging via `log/slog`

### Refreshing Logic

The core fetch → filter → resolve pipeline:

1. `anilist.Client.FetchSeason(ctx, season, year, max, formats)` — paginated GraphQL
2. `winter overflow` logic for WINTER seasons (merges December-starting shows
   from the previous year's winter)
3. `filterSeries(shows)` — filters to TV/ONA formats only
4. `filterWinterMonth(shows, season)` — for WINTER seasons, restricts to shows
   starting December–March
5. `filter.Filter(shows, cfg)` — duration ≤10 min exclusion, blacklist (MAL ID
   or title substring), AniList tag exclusion
6. `filter.FilterFuture(shows, months)` — drops shows starting more than N
   months in the future
7. `mapping.Resolver.ResolveBatch(shows)` — resolves via MAL first, then
   AniList fallback
8. Marshal to `[]Show` JSON, store in cache

For `series-new` category, the same pipeline runs and then shows with
`PREQUEL` or `PARENT` relations are excluded (handled server-side in
`scheduler.processSeason`).

## Docker Build

Multi-stage Dockerfile:

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates shadow su-exec
COPY --from=builder /server /server
COPY entrypoint.sh /entrypoint.sh
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/entrypoint.sh"]
```

CI workflow (`publish.yml`) builds for `linux/amd64` and `linux/arm64` using
`docker/setup-qemu` and `docker/build-push-action`.

## Dependencies

- `modernc.org/sqlite` — pure-Go SQLite (no CGO)
- `github.com/klauspost/compress/zstd` — zstd decompression for anibridge mapping
- No other external runtime dependencies

## File Layout

| Path | Notes |
|------|-------|
| `cmd/server/main.go` | HTTP server entry point |
| `internal/config/config.go` | Env-var configuration |
| `internal/cache/cache.go` | SQLite cache layer |
| `internal/scheduler/scheduler.go` | Background refresh goroutines |
| `internal/anilist/anilist.go` | AniList GraphQL client |
| `internal/filter/filter.go` | Show filtering pipeline |
| `internal/mapping/anibridge.go` | AniBridge mapping loader / parser / HTTP |
| `internal/mapping/resolve.go` | Thread-safe TVDB ID resolver |
| `entrypoint.sh` | PUID/PGID privilege drop |
| `Dockerfile` | Multi-stage, multi-arch build |
| `docker-compose.yml` | Quick-start composition |
| `.github/workflows/publish.yml` | CI/CD: test, version, build, publish |

## Testing

- `go test ./...` must pass
- `internal/cache/` tests with in-memory SQLite
- `internal/mapping/` tests with zstd fixtures, HTTP test servers, and metadata I/O
- `internal/anilist/` tests cover show predicates (IsSeries, IsNew, IsWinterStart, etc.)
- `internal/scheduler/` tests cover series filtering, winter-month filtering, and first-season filtering
- `internal/filter/` tests cover duration, blacklist, tag exclusion, and future-date filters

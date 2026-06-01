# Tech Spec: Dockerized Sonarr Seasonal Lists Service

## Architecture

```
cmd/server/main.go
  ├── internal/config/        → env-var configuration (simplified from YAML)
  ├── internal/cache/         → SQLite cache layer (new)
  ├── internal/scheduler/     → background refresh goroutine (new)
  ├── internal/anilist/       → AniList GraphQL client (reused as-is)
  ├── internal/filter/        → show filtering (reused as-is)
  └── internal/mapping/       → MAL→TVDB resolver (reused as-is)
```

The CLI entry point (`cmd/anilistgen/`) is removed. `internal/logging/` and
`internal/output/` are removed (logging is configured directly in the server;
JSON output is served directly from cache).

## Component Details

### `internal/config/` (Adapted)

Strip YAML loading, file search paths, `init-config`, `validate`, and CLI flag
support. Replace with pure environment variable loading via `os.Getenv` with
the `ALG_` prefix retained for backwards compatibility plus new service-specific
vars without a prefix.

```go
type Config struct {
    Port               int
    PrewarmYears       []int
    PrewarmSeasons     []string
    MaxPerSeason       int
    IncludeONA         bool
    CacheDBPath        string
    CacheStaleDays     int
    RefreshCurrentDays int
    RefreshPastDays    int
    AniListTimeoutMin  int
    LogLevel           string
}
```

### `internal/cache/` (New)

Pure-Go SQLite via `modernc.org/sqlite` (no CGO, easy cross-compilation).

Schema:
```sql
CREATE TABLE IF NOT EXISTS season_cache (
    season    TEXT NOT NULL,
    year      INTEGER NOT NULL,
    category  TEXT NOT NULL,
    data      BLOB NOT NULL,
    is_empty  BOOLEAN NOT NULL DEFAULT 0,
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

// Get returns cached data and whether it's fresh (within TTL).
// If the entry exists but is empty (pending backfill), isEmpty is true.
func (c *Cache) Get(season string, year int, category string) (data []byte, fresh bool, isPending bool, ok bool)

// SetEmpty marks an entry as pending (empty data, backfill in progress).
func (c *Cache) SetEmpty(season string, year int, category string) error

// Set stores resolved show data.
func (c *Cache) Set(season string, year int, category string, data []byte) error

// MarkHit updates last_hit timestamp.
func (c *Cache) MarkHit(season string, year int, category string) error

// PruneStale removes entries where last_hit is older than staleDays.
func (c *Cache) PruneStale(staleDays int) (int, error)

// NeedsRefresh returns entries whose fetched_at is older than their TTL.
func (c *Cache) NeedsRefresh(currentYear int, currentRefreshDays, pastRefreshDays int) ([]CacheKey, error)

// Stats returns hit/miss/refresh counts.
func (c *Cache) Stats() CacheStats
```

### `internal/scheduler/` (New)

Background goroutine that ticks periodically (every 10 minutes) and:

1. Queries cache for entries needing refresh
2. For each: fetches from AniList → filters → resolves → updates cache
3. If `PREWARM_YEARS` configured, runs initial backfill on startup

```go
type Scheduler struct { ... }

func New(cache *cache.Cache, cfg *config.Config) *Scheduler
func (s *Scheduler) Start(ctx context.Context)
func (s *Scheduler) Prewarm(ctx context.Context) error
func (s *Scheduler) Refresh(ctx context.Context, season string, year int, category string) error
```

### `cmd/server/main.go` (New)

HTTP server using `net/http` (stdlib, no framework):

- `GET /list` — handler that calls cache.Get, triggers async backfill on miss,
  returns JSON
- `GET /health` — returns `{"status":"ok"}`
- `GET /cache/stats` — returns cache stats JSON (debug endpoint)
- Graceful shutdown on SIGTERM/SIGINT
- Context propagation for cancellation

### Refreshing Logic

The core fetch → filter → resolve pipeline is reused from the existing codebase:

1. `anilist.Client.FetchSeason(ctx, season, year, max, formats)` — paginated GraphQL
2. `winter overflow` logic for WINTER seasons
3. `filter.Filter(shows, cfg)` + `filter.FilterFuture(shows, months)`
4. `mapping.Resolver.ResolveBatch(shows)` → extract resolved TVDB IDs
5. Marshal to `[]output.Show` JSON, store in cache

Community mapping (`shinkro/community-mapping`) is downloaded on startup if not
already present, same as existing logic.

## Docker Build

Multi-stage Dockerfile:

```dockerfile
# Stage 1: Build
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /server /server
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/server"]
```

CI workflow builds for `linux/amd64` and `linux/arm64` using `docker/setup-qemu`
and `docker/build-push-action`.

## Dependencies

- `modernc.org/sqlite` — pure-Go SQLite (no CGO)
- `gopkg.in/yaml.v3` — for community mapping YAML parsing (existing)
- No new external dependencies beyond `modernc.org/sqlite`

## File Changes

| Action | Path |
|--------|------|
| REMOVE | `cmd/anilistgen/` |
| REMOVE | `internal/logging/` |
| REMOVE | `internal/output/` |
| REMOVE | `docs/` (in-tree docs) |
| REMOVE | `.github/workflows/weekly-sync.yml` |
| MODIFY | `internal/config/config.go` — env-var only |
| MODIFY | `internal/config/config_test.go` — updated tests |
| NEW | `internal/cache/cache.go` |
| NEW | `internal/cache/cache_test.go` |
| NEW | `internal/scheduler/scheduler.go` |
| NEW | `cmd/server/main.go` |
| NEW | `Dockerfile` |
| NEW | `docker-compose.yml` |
| NEW | `.github/workflows/publish.yml` |
| NEW | `.dockerignore` |

## Testing

- `go test ./...` must pass
- `internal/cache/` tests with in-memory SQLite
- `internal/scheduler/` tests mock the AniList client
- `cmd/server/` integration test with test cache

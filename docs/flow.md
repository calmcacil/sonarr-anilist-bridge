# sonarr-anime-bridge Container Flow

## Startup Sequence

```
1. Entrypoint (entrypoint.sh)
   ├─ Creates appuser (PUID/PGID)
   ├─ chown -R /data
   └─ exec su-exec /server

2. main.go run()
   ├─ Load config (environment variables)
   ├─ Open SQLite cache (/data/cache.db)
   ├─ Load anibridge mappings (/data/anibridge_mappings.json.zst)
   │   └─ Downloads from GitHub releases if missing
   ├─ Create Scheduler (holds anilist.Client + Cache)
   ├─ Start HTTP server (LISTENS IMMEDIATELY)
   │   ├─ GET /list        → handleList
   │   ├─ GET /health      → handleHealth
   │   ├─ GET /cache/stats → handleCacheStats
   │   └─ POST /cache/clear → handleCacheClear
   ├─ Start background workers (stale refresh, prune, stats)
   └─ Prewarm (runs after HTTP listener starts)
       ├─ For each PREWARM_YEARS
       │   └─ If cache fresh (<24h) → SKIP
       │   └─ Else FetchAndStore(year) → AniList API → cache
```

## Request Handling: `GET /list?season=X&year=Y`

```
1. Parse params (season, year, category)
2. GetYear(year) from cache
   ├─ HIT  → fresh/ok → process pipeline
   └─ MISS → synchronous FetchAndStore(year)
       ├─ Inflight deduplication (channel wait if same year already fetching)
       ├─ AniList GraphQL fetch (paginated, 50 results/page)
       │   ├─ 700ms throttle between pages
       │   ├─ Retry-After + exponential backoff on 429
       │   └─ ±25% jitter on all delays
       └─ Store in cache (year_cache table)
3. Winter overflow check (if season=WINTER or ALL)
    └─ If year-1 missing → FetchAndStore(year-1) [async, fire-and-forget]
4. Process pipeline (applied in order)
   ├─ Merge winter overflow shows (if WINTER/ALL)
   ├─ FilterBySeason (skipped if season=ALL)
   ├─ FilterByFormats (TV, ONA)
   ├─ Filter (duration >10min, exclude tags)
   ├─ FilterFuture (3 months ahead, if FILTER_FUTURE_ENABLED=true)
   ├─ FilterFirstSeason (if category=series-new)
   └─ Resolve TVDB IDs via anibridge mapping (MAL/AniList → TVDB)
5. Return JSON: [{tvdbId, title}, ...]
```

## Data Flow

```
AniList GraphQL API → JSON → SQLite cache → Filter pipeline → TVDB resolution → HTTP JSON response
```

## Cache

- **Backend**: SQLite (WAL mode) at `/data/cache.db`
- **Table**: `year_cache(year PK, data BLOB, fetched_at INT, last_hit INT)`
- **Freshness thresholds**:
  - Current year: 24 hours
  - Past years: 7 days
- **Operations**:
  - `GetYear` → returns (data, fresh, ok)
  - `SetYear` → INSERT OR REPLACE
  - `HasYear` → existence check
  - `Clear` → truncate table
  - `Stats` → entries, hits, misses

## Rate Limiting (AniList Client)

- **Global per-process**: Single `anilist.Client` with `sync.Mutex`
- **Proactive throttle**: 700ms minimum gap between API calls
- **Post-429 backoff**: 5s minimum gap for 30 seconds after any 429
- **Retry-After header**: Respected — exact sleep from server
- **Exponential backoff**: 2s → 4s → 8s → 16s (5 total attempts max)
- **Jitter**: ±25% on all delays to prevent thundering herd
- **Inflight deduplication**: `scheduler.FetchAndStore` uses per-year channels

## Shutdown

```
SIGTERM → graceful HTTP shutdown (10s timeout) → cancel context 
         → background workers stop → db.Close()
```
---
type: planning
entity: phase
plan: "anibridge-mappings-adoption"
phase: 1
status: pending
created: "2026-06-05"
updated: "2026-06-05"
---

# Phase 1: Anibridge loader & index

> Part of [anibridge-mappings-adoption](../plan.md)

## Objective

Add the foundational anibridge-mappings dataset loader: download (or read from disk), zstd-decompress, JSON-parse, and build the in-memory `AniList → TVDB` series-level index. Standalone — does not yet touch the resolver or scheduler.

## Scope

### Includes

- New file `internal/mapping/anibridge.go` containing:
  - `AnibridgeMapping` type with `anilistShow` and `anilistMovie` indexes
  - `LoadAnibridgeMappingWithAge(path string, maxAge time.Duration) (*AnibridgeMapping, error)` mirroring the shinkro loader's `WithAge` pattern
  - zstd downloader (`github.com/klauspost/compress/zstd`)
  - `parseAnibridge(data []byte)` that walks the top-level JSON, extracts `anilist:*` source descriptors, and projects each `tvdb_show:*:sN` / `tvdb_movie:*` target into the index
  - `splitDescriptor(d string) (provider string, id int, scope string)` helper
  - `LookupShow(anilistID int) (tvdbRef, bool)` and `LookupMovie(anilistID int) (tvdbRef, bool)` accessors
- Dependency: `github.com/klauspost/compress` (zstd) added to `go.mod` / `go.sum`
- Unit tests in `internal/mapping/anibridge_test.go`:
  - `splitDescriptor` round-trips for show, movie, and AniList-without-scope forms
  - `parseAnibridge` with a minimal in-memory fixture (3-5 entries) verifies index population
  - `LookupShow` / `LookupMovie` hit and miss cases
  - `LoadAnibridgeMappingWithAge` skips re-download when file is fresh, re-downloads when stale, falls back to cache on download error (mirrors shinkro behavior tests)

### Excludes (deferred to later phases)

- Any change to `internal/mapping/resolve.go` — the existing `Resolver` stays
- Any change to `internal/scheduler/scheduler.go` — old resolver still in use
- New config fields — added in Phase 2
- Stats endpoint, Dockerfile pre-seed, CI workflow

## Prerequisites

- [ ] Working tree on `main` of `sonarr-anilist-bridge`
- [ ] Go 1.24+ toolchain (already required by `go.mod`)

## Deliverables

- [ ] `internal/mapping/anibridge.go` exists and compiles
- [ ] `go.mod` lists `github.com/klauspost/compress` (direct dep)
- [ ] `internal/mapping/anibridge_test.go` exists with passing tests
- [ ] `go test ./internal/mapping/...` green
- [ ] `go test -race ./internal/mapping/...` green
- [ ] `go vet ./internal/mapping/...` clean
- [ ] No change to any other file

## Acceptance Criteria

- [ ] Unit tests cover parse, split, lookup, and the three `WithAge` paths (fresh file, stale file re-download, download failure fallback)
- [ ] A manual run of `go test -v ./internal/mapping/...` shows the new tests and they pass
- [ ] A one-off `go run` snippet (or test-only code) that loads the real `v3` release prints counts in the expected range: `anilist_shows ~9000-10000`, `anilist_movies ~400-600`
- [ ] No regression in the existing shinkro-based tests

## Dependencies on Other Phases

| Phase | Relationship | Notes |
|-------|-------------|-------|
| 2 | blocks | Phase 2 needs the `AnibridgeMapping` type and loader to build the resolver |
| 3 | blocks | Phase 3 needs the resolver from Phase 2 |
| 4 | independent | Phase 4 tests the integrated result; can be developed in parallel after Phase 3 lands |
| 5 | independent | Phase 5 is post-merge polish |

## Notes

- The anibridge JSON is large (~14 MB decompressed) but parsing is fast (~50 ms). Use `json.RawMessage` in the intermediate type to skip unmarshaling values we don't need (only the keys matter at the top level).
- The schema is `provider:id[:scope]`. AniList sources have no scope. `tvdb_show` targets have a season scope (`s0`, `s1`, ...). `tvdb_movie` targets have no scope. The `splitDescriptor` helper handles all three.
- We do NOT need to handle `tmdb_show` / `tmdb_movie` / `imdb_movie` targets at this stage — they're out of scope for this plan.
- The shinkro `CommunityMapping` loader (`internal/mapping/community.go`) is unchanged in this phase.

# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases are automated via the `publish.yml` workflow: each push to `main`
with conventional commits (`feat:`, `fix:`, `perf:`, etc.) triggers a
version bump and GitHub Release with auto-generated notes.



## [1.0.3] — 2026-06-05

## [1.0.2] — 2026-06-05

## [1.0.0] — 2026-06-05

### Added

- **AniBridge data source** (`internal/mapping/anibridge.go`):
  Replaced `shinkro/community-mapping` (YAML, ~3,387 MAL→TVDB entries) with
  `anibridge/anibridge-mappings` (zstd-compressed JSON, ~8,900 MAL→TVDB +
  ~9,100 AniList→TVDB entries). Shows without a MAL cross-reference are now
  resolved via AniList fallback — recovering ~47 previously-unresolvable
  shows across 18 years of data.
- **Conditional HTTP caching**: The anibridge mapping is persisted in a
  sidecar metadata file with ETag/MD5 tracking. On startup or refresh, a
  HEAD request checks the upstream ETag — a `304 Not Modified` skips the
  ~8 MB download entirely.
- **Background mapping refresh**: The in-memory mapping is checked for
  updates every hour. When changes are detected, the mapping is swapped
  atomically (`sync/atomic.Pointer`) without blocking lookups. Each refresh
  logs a diff: `Updated anibridge database, 12 new, 3 removals, 18091 total`.
- **PUID/PGID support** (`entrypoint.sh`): Drops privileges at runtime
  using `su-exec`. The entrypoint creates a user/group matching the
  provided UID/GID (default `1000:1000`), chowns `/data`, and runs the
  server as that user.
- **Clean `docker-compose.yml`**: Removed hardcoded default overrides
  that would silently break in future years (e.g. `PREWARM_YEARS=2026`).
  Documented available env vars as commented examples.

### Changed

- **Go dependency**: `gopkg.in/yaml.v3` removed (no longer needed),
  `github.com/klauspost/compress/zstd` added for mapping decompression.
- **Config env vars**: New `ALG_ANIBRIDGE_MAPPING_PATH`,
  `ALG_ANIBRIDGE_REFRESH_DAYS`, `ALG_ANIBRIDGE_URL`.
- **NOTICE**: Updated attribution from `shinkro/community-mapping` to
  `anibridge/anibridge-mappings`.

### Removed

- `internal/mapping/community.go` — old shinkro YAML loader.

[1.0.0]: https://github.com/calmcacil/sonarr-anilist-bridge/releases/tag/v1.0.0
[1.0.2]: https://github.com/calmcacil/sonarr-anilist-bridge/releases/tag/v1.0.2
[1.0.3]: https://github.com/calmcacil/sonarr-anilist-bridge/releases/tag/v1.0.3

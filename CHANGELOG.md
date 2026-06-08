# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases are automated via the `publish.yml` workflow: each push to `main`
with conventional commits (`feat:`, `fix:`, `perf:`, etc.) triggers a
version bump and GitHub Release with auto-generated notes.













## [2.3.0] — 2026-06-08

### Added
add cache robustness fixes — db.Ping, index, placeholder/JSON validation, and VACUUM (#16)

## [2.2.0] — 2026-06-08

### Added
cache uplift — AniList raw cache, mapping versioning, periodic stats, integration tests (#15)

## [2.1.0] — 2026-06-08

### Added
add cache entry count to startup logs (#12)

## [2.0.0] — 2026-06-08

### Changed
simplify config env vars, remove 8 rarely-needed flags (#11)

## [1.1.0] — 2026-06-08

### Added
enable ONA format by default (#9)

## [1.0.8] — 2026-06-08

### Miscellaneous
improve CI, testing, Docker, and code organization (#7)

## [1.0.7] — 2026-06-06

### Fixed
apply architecture review findings — concurrency hardening, shutdown lifecycle, config validation, entrypoint safety

## [1.0.6] — 2026-06-05

### Fixed
propagate fetch errors, add HEALTHCHECK, harden input validation

## [1.0.5] — 2026-06-05

### Fixed
resolve all code review findings (#4)

### Documentation
sync README and specs to current architecture
add project AGENTS.md with PR regression check instructions

## [1.0.4] — 2026-06-05

### Fixed
strip git log prefix and tags in CHANGELOG generator

### Miscellaneous
rename repo to sonarr-anime-bridge

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

[1.0.0]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.0
[1.0.2]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.2
[1.0.3]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.3
[1.0.4]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.4
[1.0.5]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.5
[1.0.6]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.6
[1.0.7]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.7
[1.0.8]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.0.8
[1.1.0]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v1.1.0
[2.0.0]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v2.0.0
[2.1.0]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v2.1.0
[2.2.0]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v2.2.0
[2.3.0]: https://github.com/calmcacil/sonarr-anime-bridge/releases/tag/v2.3.0

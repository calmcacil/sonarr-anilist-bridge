# sonarr-anime-bridge Agent Instructions

Project-specific rules for coding agents working on this repo.

## Key Architecture (year-cache + on-the-fly filtering)

- **Cache**: single `year_cache(year)` table stores raw AniList JSON per year
- **Filtering**: `FilterBySeason`, `FilterByFormats`, duration, tags, future-date,
  and first-season filters all applied on-the-fly per request from cached data
- **Resolution**: TVDB IDs resolved on-the-fly via in-memory anibridge mapping
  (no `mapping_version` tracking needed — mapping updates apply immediately)
- **Winter overflow**: December-starting shows from prior year's WINTER season
  merged on WINTER requests. First WINTER request triggers async backfill for
  prior year if not yet cached. Subsequent requests include the overflow shows.
- **No more `PREWARM_SEASONS`** — the server fetches full year data from AniList
  and splits by season locally using the `season` field from the API.

## Before Pull Request

Agents MUST run `docs/PREFLIGHT_TEST.md` checks before creating any PR.
Run phases in order — each phase depends on the prior ones passing.

- **Phase 1** (every change): lint, build, test
- **Phase 2** (behavioral changes): native regression comparing against latest release
- **Phase 3** (container/lifecycle changes): Docker regression tests
- **Phase 4** (data pipeline changes): integration tests
- **Phase 5** (code review findings): validation tests from issue #31
- **Phase 6** (startup/shutdown): container lifecycle tests

Only create the PR after all relevant phases pass.
See `docs/PREFLIGHT_TEST.md` for full procedures and commands.

## Test Procedures

See `docs/PREFLIGHT_TEST.md` for full test procedures, organized by phase:

- **Phase 1** (every change): lint, build, test
- **Phase 2** (behavioral changes): native regression comparing against latest release
- **Phase 3** (container/lifecycle changes): Docker regression tests
- **Phase 4** (data pipeline changes): integration tests
- **Phase 5** (code review findings): validation tests from issue #31
- **Phase 6** (startup/shutdown): container lifecycle tests

## Project Commands

- **Build**: `go build ./...`
- **Test**: `go test -race ./...`
- **Lint**: `golangci-lint run ./...` (install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`; PATH may need `$(go env GOPATH)/bin` prepended)
- **Docker build**: `DOCKER_BUILDKIT=1 docker build --build-arg BUILDPLATFORM=linux/arm64 --build-arg TARGETOS=linux --build-arg TARGETARCH=arm64 -t sonarr-anime-bridge:test .`

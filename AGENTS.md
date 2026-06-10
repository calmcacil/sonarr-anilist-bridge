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

## Release Workflow

When the user asks for a release:

1. Create a feature branch off `main` (e.g., `feat/description`)
2. Commit changes with a conventional commit message (`feat:`, `fix:`, `perf:`, etc.)
3. Push the branch and create a PR against `main`
4. Wait for CI tests to pass on the PR
5. Merge the PR — the `publish.yml` workflow auto-generates the version bump,
   changelog, and GitHub Release from the conventional commit
6. **Do not** manually edit `CHANGELOG.md` — it is managed by the automated release workflow

## Project Commands

- **Build**: `go build ./...` (also runs via pre-commit on every commit with Go changes)
- **Test**: `go test -race ./...` (also runs via pre-commit on every commit with Go changes)
- **Lint**: `golangci-lint run ./...` (install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`; PATH may need `$(go env GOPATH)/bin` prepended; also runs via pre-commit on every commit with Go changes)
- **Docker build**: `DOCKER_BUILDKIT=1 docker build --build-arg BUILDPLATFORM=linux/arm64 --build-arg TARGETOS=linux --build-arg TARGETARCH=arm64 -t sonarr-anime-bridge:test .`

## Pre-commit Hooks

The project uses [pre-commit](https://pre-commit.com) to enforce code quality and commit conventions.

### Setup (one-time)

```bash
pip install pre-commit
pre-commit install          # installs all hook types (pre-commit + commit-msg)
```

### What runs on every commit

| Hook | Stage | When it triggers |
|------|-------|-----------------|
| `trailing-whitespace` | pre-commit | always |
| `end-of-file-fixer` | pre-commit | always |
| `check-yaml` | pre-commit | always |
| `check-json` | pre-commit | always |
| `check-added-large-files` | pre-commit | always |
| `detect-private-key` | pre-commit | always |
| `markdownlint` | pre-commit | only when markdown files changed |
| `golangci-lint run ./...` | pre-commit | only when Go files changed |
| `go build ./...` | pre-commit | only when Go files changed |
| `go test -race ./...` | pre-commit | only when Go files changed |
| `conventional-commit` | commit-msg | every commit (validates message) |

### Manual usage

```bash
pre-commit run --all-files        # run all hooks on every file
pre-commit run golangci-lint      # run a single hook
pre-commit run go-test            # run a single hook
pre-commit run markdownlint       # run a single hook
SKIP=markdownlint git commit -m "..."  # skip specific hooks temporarily
```

### CI enforcement

The `ci.yml` workflow runs `pre-commit/action@v3.0.1` on every PR to catch issues from skipped hooks (`--no-verify`). This runs in addition to the explicit lint/build/test steps.

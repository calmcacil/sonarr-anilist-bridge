---
type: planning
entity: plan
plan: "anibridge-mappings-adoption"
status: draft
created: "2026-06-05"
updated: "2026-06-05"
---

# Plan: anibridge-mappings-adoption

## Objective

Replace the single-source `shinkro/community-mapping` (MAL→TVDB only, manual updates) with `anibridge/anibridge-mappings` as the primary ID-resolution dataset, while keeping the shinkro mapping as a fallback for the ~95-entry coverage gap. The change is gated by a regression test that compares the new server's output against the existing 2025/2026 GitHub Pages baselines.

## Motivation

The current `internal/mapping/community.go:15` loads a 947 KB YAML file from `shinkro/community-mapping/tvdb-mal.yaml`. The dataset has 5,241 entries, all keyed by MAL ID. This forces a two-hop resolution path (AniList ID → `idMal` from API response → shinkro lookup → TVDB ID) and silently drops any AniList entry that lacks a `idMal` field.

`anibridge-mappings` is a daily-released, MIT-licensed dataset of 76,353 cross-provider descriptors covering AniList, AniDB, MAL, TVDB (show + movie), TMDB (show + movie), and IMDB (movie). Analysis of the `v3` release on 2026-06-05 shows 9,428 AniList entries with a direct TVDB target — none of which the current pipeline can resolve.

For the long-running Docker server, direct AniList→TVDB resolution is the most valuable change: it cuts the resolution path from 2 hops to 1, and unlocks Radarr support (movies) without a separate pipeline.

The parent CLI tool's published output on `gh-pages` is the existing ground truth. The regression test compares new output against that baseline to ensure no currently-resolved show falls off.

## Requirements

### Functional

- [ ] Load `anibridge-mappings` v3 from the official GitHub Releases URL (`mappings.json.zst`)
- [ ] Build in-memory `AniList → TVDB (show | movie)` series-level index at startup
- [ ] Keep the shinkro `CommunityMapping` loader and use it as a fallback for AniList IDs that have no direct anibridge entry
- [ ] Add config fields `AnibridgePath` and `AnibridgeMaxAge` with env-var overrides (`ALG_ANIBRIDGE_PATH`, `ALG_ANIBRIDGE_MAX_AGE`)
- [ ] Continue serving the existing `/list` endpoint contract unchanged
- [ ] Provide an ops-visibility endpoint `/mappings-stats` returning counts + dataset version
- [ ] Pre-seed `/data/anibridge.json.zst` in the Docker image so cold start has no network requirement
- [ ] Add a regression test binary that compares 2025/2026 output against `gh-pages` baselines
- [ ] Add a CI workflow that runs the regression test and blocks merge on regression

### Non-Functional

- [ ] In-memory anibridge index ≤ 5 MB resident memory
- [ ] Cold-start parse (zstd decompress + JSON parse + index build) ≤ 500 ms on a Pi 4 / arm64
- [ ] No new runtime HTTP dependencies on every request (download happens only on `maxAge` expiry)
- [ ] All existing tests pass; new code covered at ≥ 80% per package
- [ ] No breaking change to the `/list` JSON response shape

## Scope

### In Scope

- New `internal/mapping/anibridge.go` (loader, index, lookup)
- New `mapping.AniListGraphResolver` in `internal/mapping/resolve.go`
- Two new config fields + env vars
- `internal/scheduler/scheduler.go` swap from `*mapping.Resolver` to `*AniListGraphResolver`
- New `cmd/regression-test/` binary
- New `.github/workflows/regression.yml`
- New `internal/server/stats.go` handler for `/mappings-stats`
- `Dockerfile` update to pre-seed anibridge snapshot
- `README.md` update with new env vars + `/mappings-stats` reference

### Out of Scope

- Radarr support (movie category) — schema work is done but the endpoint and category routing are deferred to a follow-up plan
- AniDB / TMDB / IMDB resolution paths — anibridge exposes them but the docker server's contract is `[{tvdbId, title}]`; supporting alternate IDs is a separate change
- Episode-level mapping — anibridge is episode-grained; we only project to series-level
- Hot-reload of the anibridge dataset while the server is running — restart is acceptable
- Replacement of the shinkro loader — kept as fallback path

## Definition of Done

- [ ] All phases complete with passing tests
- [ ] Regression test passes against 2025 and 2026 `gh-pages` baselines (zero TVDB ID regressions)
- [ ] CI workflow file present and runnable
- [ ] `Dockerfile` builds successfully for both `linux/amd64` and `linux/arm64`
- [ ] `go test -race ./...` clean
- [ ] `go vet ./...` clean
- [ ] `/mappings-stats` returns the loaded dataset version and entry counts
- [ ] README documents new env vars
- [ ] Memory profile of in-process server ≤ 25 MB RSS (current baseline + anibridge index)

## Testing Strategy

- [ ] Unit tests in `internal/mapping/` for anibridge parser, descriptor splitter, lookup logic, and resolver
- [ ] Unit tests in `internal/config/` for new env-var parsing and defaulting
- [ ] Integration test in `cmd/regression-test/` that:
  - Fetches `gh-pages` JSON for each season in 2025 and 2026 (4 seasons × 2 years = 8 baseline files per category)
  - Starts the docker server in-process (or as a subprocess on a known port)
  - Queries `/list?season=X&year=Y&category=series` for each baseline
  - Asserts: every TVDB ID in the baseline is present in the new output (zero regressions)
  - Reports: gains (new TVDB IDs in the new output, not in the baseline) and title diffs (informational, not failing)
- [ ] CI workflow (`.github/workflows/regression.yml`) runs on PR open/sync targeting `main` and on `workflow_dispatch`
- [ ] Manual smoke test against the current year (2026) before tagging a release

## Phases

| Phase | Title | Scope | Status |
|-------|-------|-------|--------|
| 1 | Anibridge loader & index | `internal/mapping/anibridge.go`, zstd dep, unit tests for parse/lookup | pending |
| 2 | AniListGraphResolver & config | New resolver, fallback logic, two new config fields, unit tests | pending |
| 3 | Wire resolver into scheduler | Swap scheduler to use new resolver, build, manual smoke | pending |
| 4 | Regression test harness & CI gate | `cmd/regression-test/`, `.github/workflows/regression.yml`, run against 2025/2026 | pending |
| 5 | Stats endpoint, Dockerfile seed, README | `/mappings-stats` handler, image pre-seed, env-var docs | pending |

## Risks & Open Questions

| Risk/Question | Impact | Mitigation/Answer |
|---------------|--------|-------------------|
| zstd dep license | Adds one runtime dep | `github.com/klauspost/compress` is BSD-3-Clause (compatible) |
| Anibridge download at startup blocks server boot | Slow first-request if not pre-seeded | Pre-seed in Dockerfile; runtime download only on `maxAge` expiry |
| Anibridge dataset breaks episode mapping for some shows | Some shows may now resolve to a different TVDB series | Regression test catches this; fallback to shinkro for any flagged entry |
| Anibridge schema v4 changes | New fields may need handling | Track upstream; the v3-stable URL is pinned in the loader for now |
| `gh-pages` baseline drift (entries added between baseline fetch and test) | False regression signal | Fetch baseline at test start, not from a checked-in snapshot |
| In-process vs subprocess server for the regression test | Subprocess is more realistic but slower | Start with subprocess; can switch to httptest.NewServer in-process for speed if needed |
| Stats endpoint requires a refactor to thread the mapping handle into the server | Touches `cmd/server/main.go` | Pass the mapping through `cache.Stats()`-style accessor; small change |
| 9,011 validation issues in anibridge (per their stats) | Some episode-level mappings are inconsistent | We project to series-level, so episode issues don't surface at our resolution layer |

## Changelog

### 2026-06-05

- Plan created

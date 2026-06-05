---
type: planning
entity: phase
plan: "anibridge-mappings-adoption"
phase: 3
status: pending
created: "2026-06-05"
updated: "2026-06-05"
---

# Phase 3: Wire resolver into scheduler

> Part of [anibridge-mappings-adoption](../plan.md)

## Objective

Swap the scheduler from the old `*mapping.Resolver` (which does MAL→TVDB only) to the new `*AniListGraphResolver` (which does direct AniList→TVDB with MAL fallback). Remove the now-unused `Resolver` and `NewResolver` types from the `mapping` package.

## Scope

### Includes

- `internal/scheduler/scheduler.go` changes:
  - `Scheduler.resolver` field type changes from `*mapping.Resolver` to `*mapping.AniListGraphResolver`
  - `loadResolver` now loads both anibridge (via `LoadAnibridgeMappingWithAge`) and shinkro (via `LoadCommunityMappingWithAge`), constructing the new resolver
  - `resolveShows` iterates shows and calls `resolver.Resolve(show)` instead of `resolver.ResolveBatch` + index-lookup
  - Update log message: `"resolved via community mapping"` → `"resolved via anibridge"` or `"resolved via shinkro fallback"` (the new resolver's `Resolve` method already logs the path; remove the duplicate log in the scheduler)
- `internal/mapping/resolve.go` changes:
  - Remove the `Resolver` struct, `NewResolver`, `Resolve`, `ResolveBatch`, `ResolvedShow` (replaced by `AniListGraphResolver`)
  - Keep `model` import only if other code in the file needs it
- Update `cmd/server/main.go` if it references the old `Resolver` type directly (it should not, since the scheduler owns it)
- Update or remove tests in `internal/mapping/` that exercise the removed types

### Excludes (deferred to later phases)

- New `cmd/regression-test/` binary — Phase 4
- CI workflow — Phase 4
- Stats endpoint, Dockerfile pre-seed — Phase 5

## Prerequisites

- [ ] Phase 1 complete — `AnibridgeMapping` type exists
- [ ] Phase 2 complete — `AniListGraphResolver` exists and is unit-tested
- [ ] `go test ./...` green before starting this phase

## Deliverables

- [ ] Scheduler builds and runs
- [ ] No reference to `mapping.Resolver` anywhere in the codebase (`grep` returns nothing)
- [ ] `go test ./...` green
- [ ] `go test -race ./...` green
- [ ] `go vet ./...` clean
- [ ] Manual smoke test: run server locally, `curl /list?season=WINTER&year=2026` returns a non-empty array
- [ ] Logs show resolution path breakdown (how many hits via anibridge vs shinkro fallback vs unresolved)

## Acceptance Criteria

- [ ] `git grep "mapping.Resolver" -- ':!**/migrations/**' ':!**/vendor/**'` returns no results in non-test source files
- [ ] A manual `curl` against a freshly-started server for a known season returns at least as many TVDB IDs as the same call would have returned before this change (informal check — exact regression coverage is Phase 4's job)
- [ ] Server starts in ≤ 2 seconds on the test machine
- [ ] Log line `"resolution summary"` from `resolveShows` shows non-zero counts

## Dependencies on Other Phases

| Phase | Relationship | Notes |
|-------|-------------|-------|
| 1, 2 | blocks | Needs both loader and resolver types |
| 4 | blocked-by | Phase 4's regression test runs against this phase's output. Phase 4 must pass before this phase is merged. |
| 5 | independent | Post-merge polish |

## Notes

- This phase does NOT add the `/mappings-stats` endpoint — that comes in Phase 5. Operators confirm the dataset loaded via the `"loaded anibridge mapping"` log line for now.
- The docker image at this point will download anibridge on first request that triggers `loadResolver` (which happens at scheduler `Start` time during prewarm). This is OK for dev but Phase 5 will pre-seed the image.
- A pre-merge sanity check: run the server against `PREWARM_YEARS=2025,2026 PREWARM_SEASONS=winter,spring,summer,fall` and watch the log for resolved counts. Expected: each season resolves to ~30-50 shows, ≥ 90% via anibridge.

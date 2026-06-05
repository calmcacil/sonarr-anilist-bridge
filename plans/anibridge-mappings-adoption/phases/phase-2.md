---
type: planning
entity: phase
plan: "anibridge-mappings-adoption"
phase: 2
status: pending
created: "2026-06-05"
updated: "2026-06-05"
---

# Phase 2: AniListGraphResolver & config

> Part of [anibridge-mappings-adoption](../plan.md)

## Objective

Introduce the new resolver that uses anibridge as the primary AniList→TVDB path and the shinkro `CommunityMapping` as a fallback for the ~95-entry coverage gap. Add the new config fields and env-var bindings. Scheduler still uses the old resolver.

## Scope

### Includes

- New type `AniListGraphResolver` in `internal/mapping/resolve.go` (or split into `internal/mapping/graph.go` if `resolve.go` becomes too long)
  - Constructor: `NewAniListGraphResolver(ab *AnibridgeMapping, fb *CommunityMapping) *AniListGraphResolver`
  - Method: `Resolve(show anilist.Show) (tvdbID int, ok bool)` — primary anibridge lookup, fallback shinkro via `show.IDMal`
  - Either can be nil; behavior degrades gracefully (anibridge-only or shinkro-only or no-op)
- Two new config fields on `internal/config/config.go`:
  - `AnibridgePath string` — default `/data/anibridge.json.zst`, env `ALG_ANIBRIDGE_PATH`
  - `AnibridgeMaxAge time.Duration` — default `168h` (7 days), env `ALG_ANIBRIDGE_MAX_AGE`
- Config tests in `internal/config/config_test.go`:
  - Defaults are applied
  - Env-var overrides take effect
  - Invalid duration string falls back to default (consistent with existing `CommunityMappingMaxAge` handling)
- Resolver unit tests in `internal/mapping/` (extend `resolve_test.go` or add `graph_test.go`):
  - AniList ID hit in anibridge → returns anibridge's TVDB ID
  - AniList ID not in anibridge, `idMal` hits shinkro → returns shinkro's TVDB ID
  - AniList ID not in anibridge, `idMal` not in shinkro → returns `(0, false)`
  - AniList ID not in anibridge, no `idMal` → returns `(0, false)`
  - Anibridge is nil, shinkro hits → returns shinkro's TVDB ID
  - Anibridge hits, shinkro is nil → returns anibridge's TVDB ID
  - Both nil → returns `(0, false)`

### Excludes (deferred to later phases)

- Any change to `internal/scheduler/scheduler.go` — old `*mapping.Resolver` still in use
- Stats endpoint, Dockerfile pre-seed, CI workflow

## Prerequisites

- [ ] Phase 1 complete — `AnibridgeMapping` type and `LoadAnibridgeMappingWithAge` exist
- [ ] Existing `CommunityMapping` and `Resolver` from before this plan still in place

## Deliverables

- [ ] `AniListGraphResolver` type compiles
- [ ] Two new config fields are loaded from env vars with documented defaults
- [ ] All resolver and config unit tests pass
- [ ] `go test ./...` green
- [ ] `go test -race ./...` green
- [ ] `go vet ./...` clean

## Acceptance Criteria

- [ ] All resolver behavior cases listed above are covered by named tests
- [ ] Config defaults match the values documented in the README pre-PR
- [ ] A test asserts the resolver prefers anibridge over shinkro when both have an entry (anibridge wins)
- [ ] No change to the existing `Resolver` type — it stays for now (Phase 3 removes it)

## Dependencies on Other Phases

| Phase | Relationship | Notes |
|-------|-------------|-------|
| 1 | blocks | Needs `AnibridgeMapping` type |
| 3 | blocks | Phase 3 swaps scheduler to this resolver |
| 4 | independent | Tests the integrated result |
| 5 | independent | Post-merge polish |

## Notes

- Keep the existing `Resolver` type and `NewResolver` constructor intact during this phase. Phase 3 removes them. Removing them in this phase would force a scheduler change that isn't ready.
- The `tvdbRef` struct from Phase 1 carries a `Season` field even though the current `/list` endpoint only exposes `tvdbId`. Keeping the field lets Phase 5's stats endpoint report season coverage later.
- Config defaults: 7 days is reasonable because anibridge releases daily but most changes are corrections, not new entries. A weekly refresh keeps the cold-start cost amortized.

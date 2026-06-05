---
type: planning
entity: phase
plan: "anibridge-mappings-adoption"
phase: 5
status: pending
created: "2026-06-05"
updated: "2026-06-05"
---

# Phase 5: Stats endpoint, Dockerfile seed, README

> Part of [anibridge-mappings-adoption](../plan.md)

## Objective

Polish the integration: give operators visibility into the loaded dataset, eliminate the first-request network dependency by pre-seeding the Docker image, and document the new env vars.

## Scope

### Includes

- New endpoint `GET /mappings-stats` on the server:
  - Response: `{"version": "3.0.3", "generated_on": "2026-06-05T06:17:18Z", "anilist_shows": 9428, "anilist_movies": 484, "shinkro_entries": 5241, "loaded_at": "2026-06-05T13:25:00Z"}`
  - Threading: the `*mapping.AnibridgeMapping` handle is exposed via the `cache.Cache` (similar pattern to the existing `db.Stats()`) OR the server constructor accepts it as a parameter; pick the simpler path
  - Update `cmd/server/main.go` to register the handler
- `Dockerfile` change:
  - Add a `RUN` step that downloads `mappings.json.zst` to `/data/anibridge.json.zst` at build time
  - Pin the release tag in the build arg (e.g., `ANIBRIDGE_VERSION=v3`) so rebuilds are reproducible
  - Verify the file exists at the expected path in the final image
- `docker-compose.yml` change:
  - Mount `/data/anibridge.json.zst` as a writable volume only if the user wants runtime updates; default to read-only mount of the pre-seeded file is also acceptable
  - Add a commented-out `ALG_ANIBRIDGE_MAX_AGE` example
- `README.md` changes:
  - Add `ALG_ANIBRIDGE_PATH` and `ALG_ANIBRIDGE_MAX_AGE` to the env-var table
  - Add `GET /mappings-stats` to the endpoints section with a sample response
  - Brief note that the anibridge dataset is MIT-licensed and bundled in the image

### Excludes (deferred to later plans)

- Hot-reload of the anibridge dataset (current behavior: restart on `maxAge` expiry is fine)
- Multiple dataset versions side-by-side
- Per-provider stats (only anilist show/movie counts are reported; we don't load other providers yet)
- Radarr support (movies in `/list?category=movies`) — separate plan

## Prerequisites

- [ ] Phase 4 complete — the regression test gate has passed for phases 1–3
- [ ] Phases 1–4 merged to `main` (or this phase is part of the same merge — either is acceptable)

## Deliverables

- [ ] `GET /mappings-stats` returns valid JSON
- [ ] `docker build` succeeds and the image contains `/data/anibridge.json.zst`
- [ ] `README.md` documents new env vars
- [ ] `docker compose config` validates
- [ ] Multi-arch build still produces both `linux/amd64` and `linux/arm64` images
- [ ] `go test ./...` green
- [ ] `go test -race ./...` green

## Acceptance Criteria

- [ ] `curl http://localhost:8080/mappings-stats` returns the documented JSON shape
- [ ] The anibridge version field matches the pinned `ANIBRIDGE_VERSION` build arg
- [ ] A fresh container with no mounted volumes can serve `/list?season=WINTER&year=2026` without making a network call to anibridge (verify via log: no "downloading anibridge mappings" line)
- [ ] The README env-var table is sorted and consistent with the existing format
- [ ] LICENSE attribution note is added (anibridge is MIT; same as this project)

## Dependencies on Other Phases

| Phase | Relationship | Notes |
|-------|-------------|-------|
| 1, 2, 3, 4 | blocks | The mapping types and resolver must exist |
| — | — | This is the final phase |

## Notes

- The stats endpoint is purely informational; it does not change server behavior. It exists so operators can confirm the dataset is loaded and at what version without tailing logs.
- Pinning `ANIBRIDGE_VERSION=v3` is intentional: anibridge uses `v{major}` for breaking changes, so `v3` will continue to work even when they tag `v4` and beyond. To move to `v4`, a deliberate Dockerfile change is required.
- The pre-seed approach trades image size for cold-start latency. Current expected size impact: ~2 MB compressed (zstd is highly compressible for this kind of repetitive JSON). This is acceptable for a ~30 MB Go binary.
- The `/mappings-stats` endpoint is a good place to add Prometheus metrics later (e.g., `anibridge_loaded_entries`, `anibridge_dataset_age_seconds`) — out of scope for this plan but the JSON shape leaves room.

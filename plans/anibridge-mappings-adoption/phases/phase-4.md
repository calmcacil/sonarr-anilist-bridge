---
type: planning
entity: phase
plan: "anibridge-mappings-adoption"
phase: 4
status: pending
created: "2026-06-05"
updated: "2026-06-05"
---

# Phase 4: Regression test harness & CI gate

> Part of [anibridge-mappings-adoption](../plan.md)

## Objective

Add a regression test that compares the new server's output against the 2025 and 2026 `gh-pages` baselines, and a CI workflow that runs it on PRs and on `workflow_dispatch`. **This is the merge gate for phases 1–3.**

## Scope

### Includes

- New binary `cmd/regression-test/main.go` that:
  1. Reads years and categories from flags (default `2025,2026` and `series,series-new`)
  2. For each `(year, season, category)` combination:
     - Fetches `https://raw.githubusercontent.com/calmcacil/sonarr-seasonal-lists/gh-pages/<year>/<season>-<category>.json` and parses the TVDB ID set
     - Starts the docker server as a subprocess on a free port, with `PREWARM_YEARS=<year> PREWARM_SEASONS=<season>` and a temp `CACHE_DB_PATH`
     - Polls `/list?season=<season>&year=<year>&category=<category>` until non-empty (or 60 s timeout) — the cold-start backfill takes a few seconds
     - Decodes the response into a TVDB ID set
     - Compares:
       - **Regressions** (baseline \ new): FAIL the test
       - **Gains** (new \ baseline): print to log, do not fail
       - **Title diffs** (same TVDB ID, different title): print to log, do not fail
  3. Exits non-zero if any regression is found
  4. Prints a summary table at the end: `year | season | category | baseline | new | regressions | gains`
  5. Cleans up the subprocess and temp files on exit (signal handler)
- New file `.github/workflows/regression.yml`:
  - Triggers: `pull_request` to `main`, `workflow_dispatch`, `schedule` (weekly Sunday 06:00 UTC, optional)
  - Steps: checkout, setup Go, build server, build regression-test binary, run `go run ./cmd/regression-test/`
  - Uploads the printed summary as a workflow artifact (optional, for the first iteration just print)
- New unit test in `cmd/regression-test/` covering the diff/summary logic with hardcoded baselines (so the diff algorithm itself is tested without a live server)
- Documentation in the phase deliverable: how to run locally, how to interpret output, how to triage a regression

### Excludes (deferred to later phases)

- Per-PR comment posting via `gh pr comment` — defer until the basics work
- Slack / Discord / email notifications on regression — defer
- Storing historical baselines for trend analysis — defer

## Prerequisites

- [ ] Phase 3 complete — the server binary is ready to be tested
- [ ] `gh-pages` branch of `calmcacil/sonarr-seasonal-lists` is accessible (verified via API on 2026-06-05)
- [ ] Network access from CI to `raw.githubusercontent.com` and `api.github.com`

## Deliverables

- [ ] `cmd/regression-test/main.go` exists, builds, runs
- [ ] `cmd/regression-test/diff_test.go` covers the diff/summary logic
- [ ] `.github/workflows/regression.yml` exists, syntax-valid
- [ ] Local run against 2025 + 2026 shows zero regressions
- [ ] `README.md` "Development" section documents how to run the regression test
- [ ] `go test ./...` green
- [ ] `go test -race ./...` green

## Acceptance Criteria

- [ ] `go run ./cmd/regression-test/ --years=2025,2026 --categories=series,series-new` exits 0 and prints the summary table
- [ ] For 2025/2026 across all seasons, the regression count in the summary is 0
- [ ] A forced regression (e.g., temporarily deleting an entry from the anibridge index in a test build) is detected and the binary exits non-zero
- [ ] The CI workflow file is syntactically valid (`gh workflow view` after push, or `actionlint`)
- [ ] The CI workflow runs to completion on a draft PR (sanity check, not a merge requirement for this phase)

## Dependencies on Other Phases

| Phase | Relationship | Notes |
|-------|-------------|-------|
| 1, 2, 3 | blocks | Tests the integrated server from Phase 3 |
| 5 | independent | Phase 5 is post-merge polish |

## Notes

- **This phase is the merge gate.** Phases 1–3 are not merged until this phase's test passes.
- The test fetches the baseline at run time, not from a checked-in snapshot, so source-data drift on `gh-pages` between releases is not a stale-baseline problem.
- "Regressions" here means: a TVDB ID that was in the published baseline but is not in the new output. Investigating a regression: is the show in anibridge? If yes, why didn't the resolver hit? If no, did shinkro cover it before? If no, this is a genuine data loss and needs an anibridge issue / shinkro PR.
- "Gains" are expected and welcome. They are typically entries that the CLI tool missed because it had no `idMal` for them.
- The subprocess server approach is more realistic but slower. If a CI run exceeds 10 minutes, switch to `httptest.NewServer` with the handler functions called directly — no network round-trip, no subprocess management.
- The 60 s per-season timeout covers the worst-case anibridge backfill: 4 AniList fetches × ~5 s each (with throttling) plus parse time.
- Do not gate merges on the network call to `gh-pages` failing — if the baseline fetch fails for any reason, the test should fail with a clear error and the operator should investigate (baseline should be a stable URL).

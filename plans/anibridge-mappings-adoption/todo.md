---
type: planning
entity: todo
plan: "anibridge-mappings-adoption"
updated: "2026-06-05"
---

# Todo: anibridge-mappings-adoption

> Tracking [anibridge-mappings-adoption](plan.md)

## Active Phase: 1 - Anibridge loader & index

### Phase Context

- **Scope**: [Phase 1](phases/phase-1.md)
- **Implementation**: [Phase 1 Plan](implementation/phase-1-impl.md) (to be authored via `author-and-verify-implementation-plan` before execution)
- **Latest Handover**: _none yet_
- **Relevant Docs**: _module docs to be created/updated as phases land_

### Pending

- [ ] Add `github.com/klauspost/compress` dependency to go.mod / go.sum <!-- added: 2026-06-05 -->
- [ ] Implement `internal/mapping/anibridge.go` (AnibridgeMapping, LoadAnibridgeMappingWithAge, parseAnibridge, splitDescriptor, LookupShow, LookupMovie) <!-- added: 2026-06-05 -->
- [ ] Write unit tests in `internal/mapping/anibridge_test.go` (splitDescriptor, parseAnibridge, LookupShow, LookupMovie, WithAge paths) <!-- added: 2026-06-05 -->
- [ ] Verify a one-off load of the real v3 release prints expected counts <!-- added: 2026-06-05 -->
- [ ] Confirm `go test -race ./internal/mapping/...` and `go vet ./internal/mapping/...` are clean <!-- added: 2026-06-05 -->

### In Progress

- _none_

### Completed

- _none_

### Blocked

- [ ] Phases 2–5 are blocked on Phase 1 <!-- blocked: 2026-06-05, reason: Phase 1 delivers the AnibridgeMapping type that all later phases depend on -->

## Changelog

### 2026-06-05

- Plan and 5 phase documents created
- Phase 1 set as active phase
- Plan placed at `plans/anibridge-mappings-adoption/` in worktree of `calmcacil/sonarr-anilist-bridge`

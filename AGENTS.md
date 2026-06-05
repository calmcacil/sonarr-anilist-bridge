# sonarr-anime-bridge Agent Instructions

Project-specific rules for coding agents working on this repo.

## Before Pull Request

- Run the full regression suite before creating any PR:
  `go vet ./... && go build ./... && go test -race ./...`
- When making behavioral changes, also run side-by-side Docker regression
  tests to confirm no unintended regression.
- Only create the PR after all checks pass.

## Project Commands

- **Build**: `go build ./...`
- **Test**: `go test -race ./...`
- **Lint**: `go vet ./...`
- **Docker build**: `DOCKER_BUILDKIT=1 docker build --build-arg BUILDPLATFORM=linux/arm64 --build-arg TARGETOS=linux --build-arg TARGETARCH=arm64 -t sonarr-anime-bridge:test .`

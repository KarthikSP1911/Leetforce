# 0002. Go module layout: one module per component with go.work

**Status:** accepted (Phase 0)

## Context
The Go code is split into `judge/`, `runner/` and `api/`. The API uses Gin and pgx (Neon Postgres). CLAUDE.md requires that runners never connect to the database and talk only to Redis and the API. The runner also needs the judge engine.

## Decision
- One Go module per component: `leetforce/judge`, `leetforce/runner`, `leetforce/api`, each with its own `go.mod`.
- A committed `go.work` at the repo root lists the modules, so they resolve each other locally without `replace` lines.
- Gin and pgx appear only in `api/go.mod`. The runner's `go.mod` must not list either.
- Shared job and verdict types, if needed, go in a small separate module added later.

## Alternatives
- **Single root module:** simplest, but any package can import any dependency, so the "runner never touches the database" rule is enforced only by review.
- **Per-component modules with `replace` directives instead of `go.work`:** works, but needs edits in every `go.mod` and is easy to leave in a broken state.

## Consequences
- The module boundary enforces the security rule at build time.
- Each component's Docker image contains only its own dependencies.
- Docker and CI builds must handle `go.work` (copy it into the build context, or set `GOWORK=off` with `replace`); Phase 1's Makefile will define this.
- More files to maintain (three `go.mod` files plus `go.work`).

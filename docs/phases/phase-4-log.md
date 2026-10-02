# Phase 4 working log: API and database

**Branch:** `phase/4-api-database`
**Range:** `phase-4-start..phase-4-done`
**Status:** in progress

## Units of work
- [ ] `feat/4-migrations`: schema (`problems`, `submissions`, `verdicts`), migration tool, `make migrate-up`
- [ ] `feat/4-api-skeleton`: `api/` module (Gin, pgx), config, `/healthz`, pool settings
- [ ] `feat/4-problems-endpoints`: `GET /problems`, `GET /problems/:slug`
- [ ] `feat/4-submissions`: `POST /submissions`, `GET /submissions/:id`, enqueue with the test-set version
- [ ] `feat/4-verdict-ingest`: results-stream consumer writing idempotent verdicts; dead-letter watcher marks `IE`
- [ ] `test/4-idempotency`: duplicate verdicts change nothing; API to runner end-to-end
- [ ] `docs/4-adr-report`: ADR, `docs/FLOW.md`, report, summary, `PROGRESS.md` to in review
- [ ] review, merge to `main`, tag `phase-4-done`

## Decisions (2026-10-02)
- Recap question: not answered by the owner; the owner replied with the Neon connection string and "remaining all okay", so the defaults of the session plan apply. Who decided: Claude, on the owner's delegation.
- Migration tool: goose (SQL files). Compile errors on Submit (Phase 2 decision B): only the `CE` label, no compiler text. Development and end-to-end tests on the EC2 dev host.
- Neon: the owner supplied one connection string (pooler endpoint, database `neondb`). No separate test branch exists yet, so tests use a throwaway schema per run (to be recorded in unit 1), never the real tables.

## Session log

### Start of session (2026-10-02)
1. Claude read `CLAUDE.md`, `docs/PROGRESS.md`, `phase-3-summary.md`, and the Phase 4 section of `docs/PLAN.md`. Repo matched: branch `main` at `7773a7d`, tags `phase-0..3-start|done`, only `web/AGENTS.md` and `web/CLAUDE.md` untracked.
2. Claude (repo): `git checkout -b phase/4-api-database`, `git tag phase-4-start`.
3. Claude (read-only): `ssh leetforce-dev` works; Go 1.27.1; the host checkout is on `phase/3-queue-runner`; no `psql` or `goose` installed yet.
4. Owner: pasted the Neon connection string in chat. Claude wrote it as `DATABASE_URL` into `.env` on the Windows checkout and into `~/Leetforce/.env` on the host (mode 600), both git-ignored; added `DATABASE_URL` and `LEETFORCE_MIGRATE_DATABASE_URL` (blank) to `.env.example`. The secret is in no commit or doc. Because it was pasted in chat, rotating the Neon password in the console after the phase is advisable.

## File and path index
- `docs/phases/phase-4-log.md`: this log
- `.env.example`: added `DATABASE_URL`, `LEETFORCE_MIGRATE_DATABASE_URL`

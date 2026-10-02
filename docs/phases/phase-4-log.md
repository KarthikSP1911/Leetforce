# Phase 4 working log: API and database

**Branch:** `phase/4-api-database`
**Range:** `phase-4-start..phase-4-done`
**Status:** in progress

## Units of work
- [x] `feat/4-migrations`: schema (`problems`, `submissions`, `verdicts`), migration tool, `make migrate-up`
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

### Unit 1: `feat/4-migrations` (2026-10-02)
1. Claude (host): `go install github.com/pressly/goose/v3/cmd/goose@latest` on `leetforce-dev` (goose v3.28.0 in `~/go/bin`). The compile was slow on the 1 GB host (several minutes), so the first command timed out locally and finished in the background. Added to `scripts/setup-dev-host.sh`.
2. Claude (repo): `api/migrations/00001_init.sql` creates `problems` (slug PK, difficulty check, tags, `test_set_version`), `submissions` (uuid PK, language and status checks, `test_set_version` recorded at accept time) and `verdicts` (`submission_id` PK referencing submissions, verdict check AC/WA/TLE/MLE/RE/CE/OLE/IE, runtime, memory, passed, total, `test_set_version` it was judged against, runner id). No compiler text, test data or stderr is stored (decision B). The primary key is what makes a duplicate verdict write a no-op.
3. Claude (repo): `make migrate-up|migrate-down|migrate-status`, goose over `api/migrations`, URL from `LEETFORCE_MIGRATE_DATABASE_URL` or else `DATABASE_URL` in `.env`.
4. Mistake and fix: the first `make migrate-status` on the host said "set DATABASE_URL in .env". Cause: the Neon URL contains `&`, and an unquoted `.env` line is sourced by the shell, which ran the first part as a background command so the variable was never set. Fix: the value is now single-quoted in both `.env` files, and `.env.example` carries a comment saying so. (Any script that sources `.env` is affected by an unquoted `&`.)
5. Verified on the host against the real Neon database (pooler endpoint, which worked for goose, so no direct endpoint was needed): `make migrate-status` showed `Pending 00001_init.sql` (database empty), `make migrate-up` applied it (about 340 ms), `make migrate-down` then `make migrate-up` again worked, final status "Applied 00001_init.sql", version 1. `LEETFORCE_MIGRATE_DATABASE_URL` stays an optional override.
6. Claude (host): installed Trivy from the official Aqua apt repository (key dearmored into `/usr/share/keyrings/trivy.gpg`, source file `/etc/apt/sources.list.d/trivy.list`, package `trivy`), version 0.75.0. The same steps are in `scripts/setup-dev-host.sh`. The `apt-get install` also printed the usual "services need restarting" notice; nothing was restarted.
7. Claude (repo): `scripts/scan-staged.sh` runs `trivy fs --scanners secret --exit-code 1` on the staged tree (`git archive $(git write-tree)` piped to the host), because scanning the working directory would see the git-ignored `.env`. Result for this unit: clean, exit 0.

## File and path index
- `docs/phases/phase-4-log.md`: this log
- `api/migrations/00001_init.sql`: schema (problems, submissions, verdicts)
- `Makefile`: `migrate-up`, `migrate-down`, `migrate-status`
- `scripts/setup-dev-host.sh`: goose and Trivy install steps
- `scripts/scan-staged.sh`: Trivy secret scan of the staged tree via the dev host
- `.env.example`: added `DATABASE_URL`, `LEETFORCE_MIGRATE_DATABASE_URL`

# Phase 4 working log: API and database

**Branch:** `phase/4-api-database`
**Range:** `phase-4-start..phase-4-done`
**Status:** in progress

## Units of work
- [x] `feat/4-migrations`: schema (`problems`, `submissions`, `verdicts`), migration tool, `make migrate-up`
- [x] `feat/4-api-skeleton`: `api/` module (Gin, pgx), config, `/healthz`, pool settings
- [x] `feat/4-problems-endpoints`: `GET /problems`, `GET /problems/:slug`
- [x] `feat/4-submissions`: `POST /submissions`, `GET /submissions/:id`, enqueue with the test-set version
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

### Unit 2: `feat/4-api-skeleton` (2026-10-02)
1. Claude (repo): new module `api/` (`leetforce/api`, dependencies gin v1.12.0 and pgx v5.11.0), added to `go.work` and to `GO_MODULES`. `go.mod` also has `replace leetforce/queue => ../queue` so `go mod tidy` works (the runner module relies on `go.work` alone; both work).
2. `api/internal/store/store.go`: pgx pool for Neon: `MaxConnIdleTime` 30 s (idle connections close long before Neon suspends the compute), `MaxConnLifetime` 30 min, `MaxConns` 10, health check 30 s, and `QueryExecModeCacheDescribe` because the Neon pooler is pgbouncer in transaction mode where named prepared statements are unreliable.
3. `api/internal/server/server.go`: Gin router with request logging and recovery. `/healthz` does no I/O; `/readyz` pings the database and Redis and returns 503 with `down` per failing dependency, without the error text (it can name hosts; the text goes to the log). `api/cmd/api/main.go`: config from env (`DATABASE_URL`, `LEETFORCE_REDIS_URL`, `LEETFORCE_API_ADDR` default `:8080`, `LEETFORCE_QUEUE_PREFIX`), graceful shutdown. `make build-api`.
4. Test: `TestHealthAndReady` (healthz ignores a down dependency; readyz 200 when all up; 503 and no error text when one is down).
5. Mistake and fix: `make lint` on the host flagged `httptest.NewRequest` (noctx); changed to `NewRequestWithContext`. The first host run was also lost because the SSH command moved to the background; rerunning with the output saved to `/tmp/p4-unit2.log` on the host worked. The first compile of gin and pgx on the 1 GB host took several minutes.
6. Verified on the host: `make fmt lint test` all green (0 issues in every module); `bin/api` started with the real `DATABASE_URL` and Upstash URL (queue prefix `p4smoke`): `/healthz` 200, `/readyz` 200 with `database: ok, redis: ok` (326 ms), SIGTERM gave "api stopped". `bin/lfq destroy` removed the `p4smoke` keys. Trivy staged secret scan: see the commit step (clean).

### Unit 3: `feat/4-problems-endpoints` (2026-10-02)
1. Claude (repo): `api/internal/catalog/catalog.go` loads `LEETFORCE_PROBLEMS_DIR` (default `problems`) with the judge's `problem.Load` (so validation and the test-set version are the judge's own), refuses a slug that differs from its directory, and exposes `Samples(slug)`, which returns only tests the problem marks as samples. `api` now depends on `leetforce/judge` (`require` plus `replace ../judge` in `api/go.mod`).
2. `api/internal/store/problems.go`: `UpsertProblem` (insert or update, stores the current `test_set_version`), `ListProblems`, `GetProblem` (`ErrNotFound`). At startup `cmd/api/main.go` syncs every catalog problem into Postgres; a broken problem stops startup.
3. `api/internal/server/problems.go`: `GET /problems` (`{"problems": [...]}`, always an array) and `GET /problems/:slug` (metadata plus `samples`; 404 `problem not found`). The response does not include the test-set version. A database error returns a generic 500 and logs the real error. All problem and sample data goes through two small interfaces so handlers are tested with fakes.
4. Tests: `catalog_test.go` (the repo's real `problems/` loads, `Samples` returns only samples while hidden tests exist, unknown slug and missing dir); `server/problems_test.go` (list, empty list is `[]`, detail has samples and no `test_set`, 404, DB error is a generic 500 without the host text); `store/problems_test.go` plus `store/testdb_test.go` (real Postgres).
5. Test database design: each store test creates a throwaway schema `t_<random>` on Neon, applies the real `api/migrations/*.sql` Up sections into it, and drops it afterwards, so it also tests the migration. It uses the direct endpoint (the `-pooler` text removed from the URL, or `LEETFORCE_MIGRATE_DATABASE_URL`) and skips without a database.
6. Mistake and fix: the first version set `search_path` as a connection startup parameter and it was ignored, so the test's migration ran against the real `public` schema and failed with `relation "problems" already exists` (the first statement failed, so nothing was changed; `make migrate-status` afterwards still showed only version 1). Fix: `SET search_path` in `AfterConnect`, plus a guard that checks `current_schema()` equals the throwaway schema and aborts the test otherwise, so a test can never touch the real tables.
7. Verified on the host: `make fmt lint` 0 issues in all four modules; store and catalog tests pass with the real Neon database; `bin/api` against the real Neon and Upstash: `GET /problems` listed `sample-sum`, `GET /problems/sample-sum` returned two samples (tests 01 and 02) and not the three hidden ones, an unknown slug gave `404`. The real `problems` table now has the `sample-sum` row, written by the startup sync.

### Unit 4: `feat/4-submissions` (2026-10-02)
1. Claude (repo): `api/internal/store/submissions.go`: `InsertSubmission` is a single `INSERT ... SELECT ... FROM problems`, so the test-set version stamped on the submission is the one in force at that instant (returns `ErrNotFound` for an unknown problem); `DeleteSubmission`; `GetSubmission` joins the verdict if one exists (`s.id::text = $1`, so a malformed id is a clean "not found" instead of a Postgres cast error). `Submission` and `VerdictView` carry no source, test data or stderr, and the test-set version is `json:"-"`.
2. `api/internal/server/submissions.go`: `POST /submissions` (`{problem, language, source}`): 400 not JSON, 422 missing problem or source or an unsupported language, 413 source over 64 KiB (`MaxSourceBytes` mirrors `judge/engine.MaxSourceBytes`; the API does not import the engine because it pulls in Linux-only sandbox code) or body over about 260 KiB (`http.MaxBytesReader`), 404 unknown problem, then the row is inserted with a new UUID and the job is enqueued; response `202 {"id", "status":"queued"}`. If the enqueue fails the row is deleted (using `context.WithoutCancel`) and the client gets 503, so no row waits for a job that was never queued. `GET /submissions/:id` returns status and, once judged, the verdict, runtime, memory and passed/total counts. Internal errors are logged and returned as a generic 500.
3. Dependency: `github.com/google/uuid` for ids.
4. Tests: `server/submissions_test.go` (accepted stores and queues the right job; every rejection stores and queues nothing; queue failure removes the row and does not leak the error text; DB failure is a generic 500; GET never contains the test-set version, source, stderr, expected or input), `store/submissions_test.go` (real Postgres: unknown problem, version stamped at accept time survives a later problem version change, malformed id, duplicate id, language outside the four rejected by the schema, delete).
5. Verified on the host: `make fmt lint` 0 issues in all four modules; both store tests pass against Neon in throwaway schemas; live run of `bin/api` (Neon and Upstash, queue prefix `p4smoke`): `POST /submissions` for `sample-sum` returned 202, `GET` returned `status: queued` with no verdict, an unknown problem and a malformed id both gave 404. `bin/lfq destroy` removed the `p4smoke` keys.
6. Left behind: that live run inserted one real `submissions` row (id `31b379ac-3721-468b-84b1-d2b39bc098a7`, status `queued`, no verdict, its job was destroyed with the throwaway queue). It is harmless dev data; there is no cleanup script yet.
7. Known gap (deferred): a crash between the insert and the enqueue leaves a `queued` row with no job; a reaper for stale queued rows belongs with the live-status work in Phase 5.

## File and path index
- `docs/phases/phase-4-log.md`: this log
- `api/migrations/00001_init.sql`: schema (problems, submissions, verdicts)
- `Makefile`: `migrate-up`, `migrate-down`, `migrate-status`
- `scripts/setup-dev-host.sh`: goose and Trivy install steps
- `scripts/scan-staged.sh`: Trivy secret scan of the staged tree via the dev host
- `api/go.mod`, `api/go.sum`: the API module (gin, pgx)
- `api/cmd/api/main.go`: API entry point
- `api/internal/store/store.go`: pgx pool for Neon
- `api/internal/server/server.go`, `server_test.go`: router, health endpoints, test
- `api/internal/catalog/catalog.go`, `catalog_test.go`: problems directory loader and samples-only view
- `api/internal/store/problems.go`, `problems_test.go`, `testdb_test.go`: problem queries and the throwaway-schema test helper
- `api/internal/server/problems.go`, `problems_test.go`: problem endpoints
- `api/internal/store/submissions.go`, `submissions_test.go`: submission queries
- `api/internal/server/submissions.go`, `submissions_test.go`: submission endpoints
- `go.work`: `./api` added; `Makefile`: `api` in `GO_MODULES`, `build-api`
- `.env.example`: added `DATABASE_URL`, `LEETFORCE_MIGRATE_DATABASE_URL`

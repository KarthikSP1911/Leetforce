# Phase 4: API and database

**Branch:** `phase/4-api-database`
**Range:** `phase-4-start..phase-4-done`
**Dates:** 2026-10-02 to 2026-10-02
**Milestone:** none
**Working log:** [phase-4-log.md](phase-4-log.md) (every command, result and mistake)

## Summary
A Gin API now accepts submissions, stores them in Neon Postgres stamped with the test-set version, queues them for the runners, and stores the verdicts that come back. Verdict writes are idempotent: one SQL statement inserts the verdict with `ON CONFLICT DO NOTHING` and marks the submission judged only if the insert happened, so a duplicate verdict cannot change any state. Jobs that the runners gave up on (dead letters) become `IE` verdicts, which closes the Phase 3 gap. Phase 5 builds live status on top of these rows and streams.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Duplicate verdict posts do not change state | ✅ | Unit level: `go test ./api/internal/store` → `TestRecordVerdictIsIdempotent` PASS against real Neon (after AC, a repeat, a conflicting WA and an IE leave the verdict, status, runner id, test-set version and created time unchanged; exactly one row). End to end: `make test-api-e2e` → "PASS: AC stored through API, runner and ingest; a duplicate verdict changed nothing; a dead-lettered job became IE" (about 51 s, Neon + Upstash + real sandbox). Mutation check: changing `DO NOTHING` to `DO UPDATE` made both fail (store test "duplicate AC record = true, want false"; e2e verdict flipped AC to WA), then restored |
| `make migrate-up` works from empty | ✅ | On the host against the empty Neon database: `make migrate-status` showed `Pending 00001_init.sql`, `make migrate-up` applied it (about 340 ms, "successfully migrated database to version: 1"), `make migrate-down` and `make migrate-up` again both worked |
| Build item: `api/` (Gin) | ✅ | `api/cmd/api`, `make build-api`; `/healthz`, `/readyz` live against Neon and Upstash (`database: ok, redis: ok`) |
| Build item: Neon Postgres via pgx | ✅ | `api/internal/store`; pool tuned for Neon (ADR 0010) |
| Build item: migrations | ✅ | `api/migrations/00001_init.sql` (goose) |
| Build item: problems and submissions endpoints | ✅ | `GET /problems`, `GET /problems/:slug`, `POST /submissions`, `GET /submissions/:id`; handler tests and a live run on the host |
| Build item: test-set version recorded | ✅ | `submissions.test_set_version` stamped in the insert statement; `TestSubmissions` shows it stays at the accept-time value after the problem's version changes |
| Carried over from Phase 3: dead-lettered jobs need an `IE` verdict | ✅ | `ingest.HandleDead`; e2e step 3 (injected dead letter becomes `IE`); `TestDeadLetterReachesTheAPI` |
| Rule: runners never connect to the database | ✅ | `go test ./runner/` → `TestRunnerHasNoDatabaseDependency` still PASS; the database code is only in `api/` |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/4-migrations` | Initial schema, `make migrate-*`, goose and Trivy install, `scan-staged.sh` | 3 (1 feat, 1 build, 1 docs) |
| `feat/4-api-skeleton` | `api/` module: Gin router, health endpoints, pgx pool, config | 2 (1 feat, 1 docs) |
| `feat/4-problems-endpoints` | Problems loader, sync to Postgres, problem endpoints, throwaway-schema test helper | 2 (1 feat, 1 docs) |
| `feat/4-submissions` | Submission endpoints, test-set version stamped at accept time | 2 (1 feat, 1 docs) |
| `feat/4-verdict-ingest` | Queue consumers for the API, idempotent verdict write, ingest loop, dead letters to IE | 3 (2 feat, 1 docs) |
| `test/4-idempotency` | End-to-end test, dependency upgrade for 16 HIGH CVEs, full Trivy mode | 2 (1 test, 1 docs) |

Also on the phase branch directly: `61fb7cd` (open the phase log and PROGRESS), the ADR and flow commit `26e3779`, plus this report, the summary and the review record.

## File-by-file changes

Generated with `git diff --name-status phase-4-start..26e3779` (the commit before this report); the report, the summary and the review-record commit are listed at the end of each table.

### Added
| File | Purpose |
|---|---|
| `api/go.mod`, `api/go.sum` | Module `leetforce/api`: gin, pgx v5, google/uuid; `replace` to `../queue` and `../judge`; x/crypto, x/net and x/text upgraded to clear 16 HIGH CVEs (go.sum is generated) |
| `api/cmd/api/main.go` | API process: env config, loads and syncs problems, opens Postgres and Redis, starts the ingest loop, serves HTTP, graceful shutdown |
| `api/internal/server/server.go` | Router, request log, `/healthz` (no I/O), `/readyz` (pings dependencies; 503 hides error text) |
| `api/internal/server/server_test.go` | Health and readiness tests |
| `api/internal/server/problems.go` | `GET /problems`, `GET /problems/:slug` (samples only), generic 500 helper |
| `api/internal/server/problems_test.go` | List, empty list, detail without test-set version, 404, DB error not leaked |
| `api/internal/server/submissions.go` | `POST /submissions` (validation, size limits, store then enqueue, rollback on queue failure), `GET /submissions/:id` |
| `api/internal/server/submissions_test.go` | Accepted path, seven rejection cases, queue failure removes the row, DB failure generic, GET never leaks |
| `api/internal/store/store.go` | pgx pool tuned for Neon (30 s idle close, cache_describe), ping, close |
| `api/internal/store/problems.go` | `UpsertProblem`, `ListProblems`, `GetProblem`, `ErrNotFound` |
| `api/internal/store/problems_test.go` | Real-Postgres test of the problem queries and the schema's difficulty check |
| `api/internal/store/submissions.go` | `InsertSubmission` (version stamped in one statement), `DeleteSubmission`, `GetSubmission` with the joined verdict |
| `api/internal/store/submissions_test.go` | Real-Postgres: unknown problem, version at accept time, malformed id, duplicate id, language check, delete |
| `api/internal/store/verdicts.go` | `RecordVerdict` (the idempotent statement), `IsPermanent` |
| `api/internal/store/verdicts_test.go` | The exit-criterion test and edge cases (unknown submission, bad uuid, bad verdict, IE version fallback) |
| `api/internal/store/testdb_test.go` | Throwaway-schema helper that applies the real migrations and refuses to run on the real schema |
| `api/internal/catalog/catalog.go` | Loads the problems directory with the judge's loader; `Samples` returns only visible tests |
| `api/internal/catalog/catalog_test.go` | The repo's problems load; only samples come out while hidden tests exist |
| `api/internal/ingest/ingest.go` | Results and dead-letter loop with the acknowledge-or-retry failure policy |
| `api/internal/ingest/ingest_test.go` | Failure-class table tests, field mapping, dead letter to IE, `Run` on both streams |
| `api/migrations/00001_init.sql` | Tables `problems`, `submissions`, `verdicts` with CHECK constraints (goose Up and Down) |
| `queue/ingest.go` | The API's consumer group on the results and dead-letter streams (`SetupAPI`, `ReceiveResult`, `AckResult`, `ReceiveDead`, `AckDead`) |
| `queue/ingest_test.go` | 5 tests against real Redis: delivered once and kept, reclaim of an unacknowledged entry, malformed entry, dead letter reaches the API, undecodable dead letter |
| `scripts/scan-staged.sh` | Trivy on the staged tree through the dev host: secret scan, or `full` (vuln, secret, misconfig) |
| `scripts/test-api-e2e.sh` | The end-to-end exit test |
| `docs/adr/0009-idempotent-verdict-ingest.md` | ADR: results-stream consumer and the one-statement idempotent write |
| `docs/adr/0010-neon-access-migrations-and-test-schemas.md` | ADR: goose, pool settings, test schemas |
| `docs/phases/phase-4-log.md` | Working log |
| `docs/phases/phase-4.md` | This report |
| `docs/phases/phase-4-summary.md` | Plain-language summary and review record |

### Modified
| File | What changed | Why |
|---|---|---|
| `.env.example` | Added `DATABASE_URL` (with a note to quote it) and `LEETFORCE_MIGRATE_DATABASE_URL` | The URL contains `&`, which a shell treats as a control operator when `.env` is sourced; the first run on the host failed for exactly this reason |
| `Makefile` | `api` in `GO_MODULES`; `build-api`, `test-api-e2e`, `migrate-up`, `migrate-down`, `migrate-status` | New module and its gates; migrations read the URL from `.env` |
| `go.work` | `./api` added | Workspace includes the new module (ADR 0002) |
| `scripts/setup-dev-host.sh` | Installs goose and Trivy (official apt repository), prints the Trivy version | Host changes recorded as repeatable steps |
| `CLAUDE.md` | Current state, new commands, Trivy is now installed on the dev host | Keep the guide true (the working-style rule asks for this) |
| `docs/FLOW.md` | Phase 4 ticked; as-built flow with file paths; "flow after" column | Documentation rule |
| `docs/PROGRESS.md` | Phase 4 row and resume points; in review at the end | Phase tracker |

### Deleted
| File | Reason |
|---|---|
| none | |

### Renamed / moved
| From | To | Reason |
|---|---|---|
| none | | |

## Key code changes
1. **One statement for an idempotent verdict** (`api/internal/store/verdicts.go`). Both the "first wins" rule and the status flip are one atomic statement, so there is no state where a verdict exists but the submission still says `queued`, and two API instances racing on the same entry cannot both win:
```sql
WITH ins AS (
  INSERT INTO verdicts (submission_id, verdict, ...)
  SELECT s.id, $2, ... FROM submissions s WHERE s.id = $1::uuid
  ON CONFLICT (submission_id) DO NOTHING
  RETURNING submission_id
)
UPDATE submissions SET status = 'judged', updated_at = now()
WHERE id IN (SELECT submission_id FROM ins)
```
2. **Version stamped at accept time** (`store.InsertSubmission`): `INSERT ... SELECT $1, slug, $3, $4, test_set_version FROM problems WHERE slug = $2`. The version is read and written in the same statement, and zero affected rows means the problem does not exist.
3. **The API's own consumer group** (`queue/ingest.go`): `XAUTOCLAIM` for entries a crashed API instance left, then `XREADGROUP`; only `XACK` (no delete), so `lfq results` still works.
4. **Failure policy** (`api/internal/ingest/ingest.go`): duplicate or unknown submission → acknowledge; transient database error → leave pending; Postgres class 22 or 23 errors, undecodable entries and non-UUID ids → log and acknowledge so one bad entry cannot block the stream. Dead letters → `IE` with runner `dead-letter`.
5. **Tests cannot touch real tables** (`store/testdb_test.go`): after a first version silently ran against the `public` schema (and failed harmlessly), the helper sets `search_path` in `AfterConnect` and aborts unless `current_schema()` equals the throwaway schema.

## Decisions
- [ADR 0009](../adr/0009-idempotent-verdict-ingest.md): verdicts reach Postgres through a results-stream consumer and are written idempotently in one SQL statement; dead letters become `IE`.
- [ADR 0010](../adr/0010-neon-access-migrations-and-test-schemas.md): goose migrations, a pooler-safe pgx pool, and tests in throwaway schemas.
- Phase 2 decision B (compile errors on Submit) kept at its default: Submit stores and returns only the `CE` label.

## Tests
New this phase (top-level tests; Phase 3's are unchanged): `api` 14 (server 4, store 4, ingest 4, catalog 2; several have sub-cases), `queue` 5 added (17 in total), run on the host with the database enabled: all PASS, none skipped.
- Store tests need `DATABASE_URL` (they skip without it); queue tests need a Redis (the host has one); the end-to-end test needs both plus the sandbox.
- Commands: `make fmt lint test` (0 issues; all ok), `make test-api-e2e` (about 51 s), and `scripts/scan-staged.sh full`.
- Trivy: `scripts/scan-staged.sh full` = `trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1` on the staged tree, Trivy 0.75.0. First scan found 16 HIGH CVEs in `api/go.mod` (x/crypto, x/net, x/text); fixed by upgrading; rescan clean; no `.trivyignore`. Secret scans on every commit were clean. `trivy config` and `trivy image` do not apply yet (no Dockerfile, Terraform or Kubernetes files).

## Known issues and deferred work
- A crash between inserting a submission and enqueueing it leaves a `queued` row with no job: a reaper for stale queued rows moves to Phase 5.
- The `results` stream is never trimmed (entries are acknowledged, not deleted): a retention policy moves to Phase 11 (observability).
- Upstash command cost of the API's polling (about 35,000 commands a day while idle, estimated from the poll interval) is not measured against the bill: Phase 13 with the fleet.
- No authentication: anyone who can reach the API can read a submission by its random UUID (responses carry no source, test data or stderr). Phase 9 adds users and per-user rate limits.
- The real Neon database holds test leftovers from the live runs: 1 problem, 6 submissions, 5 verdicts (no cleanup command yet).
- Problems are loaded from the local folder on the API host; object storage comes in Phase 5.
- Commit `60ccd05` has the footer `Refs: phase-3` by mistake (Phase 4 work); it was already pushed, so it is recorded here and left as it is.
- The owner pasted the Neon connection string and, in Phase 3, the Upstash token in chat. Rotating both is advisable. Neon plan and limits were not checked (ADR 0010).
- Carried over unchanged: runner privilege model (Phase 6); leftover job folders after a killed runner (Phase 6); Phase 2 decisions A and C; Docker is still unverified.

## Stats
Measured with `git diff phase-4-start..26e3779` and `git log phase-4-start..26e3779` (before this report, the summary and the review-record commit):
- Commits: 16 (excluding merges), plus 6 merge commits
- Files: 29 added, 7 modified, 0 deleted (36 changed)
- Lines: +2530 / -12

# Phase 5: Live status and storage

**Branch:** `phase/5-live-status-storage`
**Range:** `phase-5-start..phase-5-done`
**Dates:** 2026-10-02 to 2026-10-02
**Milestone:** M2 (end to end on one machine)
**Working log:** [phase-5-log.md](phase-5-log.md) (every command, result and mistake)

## Summary
A submission now runs end to end on one machine with live status: you `POST` it, follow `GET /submissions/:id/events` with `curl -N`, and see `queued`, then `judging`, then the verdict. The hidden tests are no longer read from a directory next to the runner: the API publishes each problem as a bundle to an S3-compatible bucket (RustFS locally) under its test-set version, and the runner fetches exactly the version the submission was stamped with and refuses a mismatch. Responses for Submit carry only state and the verdict view, checked end to end with hostile programs that echo the hidden input. A reaper closes the Phase 4 gap of rows stored but never queued.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Submit via curl, watch Queued → Judging → verdict over SSE | ✅ | `make test-live-e2e` step 1: submit with no runner up, `curl -N .../events`, start a runner; stream `status,status,verdict` with `{"status":"queued"}`, `{"status":"judging"}` and AC 5/5. First run PASS (44 s); after the determinism fix 2 runs seen passing in a row; a manual run showed `16:42:55 queued`, `16:43:00 judging`, `16:43:01 verdict AC`; see log units 4 and 6 |
| Submit never leaks hidden data | ✅ | `make test-live-e2e` step 2: RE (echoes input and marker to stdout and stderr), CE (`#error <marker>` in the compiler output) and WA submissions; every POST, GET, SSE stream and `/problems` response searched for the marker, pieces of every hidden test (raw and JSON-escaped), a test-set version and the word `source`: none found (detector self-test passes). `go test ./api/internal/server` → `TestStreamCarriesNoHiddenData`. Mutation check: exposing `TestSetVersion` in the JSON makes the e2e fail ("a test-set version reached a response") |
| Build item: SSE status stream | ✅ | `api/internal/server/events.go`; 9 tests in `events_test.go` (flow, no duplicate events, late client, 404, transient and repeated errors, time limit, keep-alive, stream cap and release) |
| Build item: MinIO/S3 for test data | ✅ (with RustFS) | `storage/` module; `judge/problem/bundle.go`; `runner/internal/problems`; `storage` `TestRoundTrip` against the real RustFS; the e2e runner has `LEETFORCE_PROBLEMS_DIR=/nonexistent` and no database URL. MinIO itself could not be used: it no longer publishes container images (ADR 0011) |
| Build item: hidden-test redaction for Submit | ✅ | as the second criterion; no Run path exists yet (Phase 8) |
| Carried over from Phase 4: reaper for submissions stored but never queued | ✅ | `api/internal/reaper`, `store.ReapUnqueued`; `TestReapUnqueued`, `TestReapUnqueuedRespectsLimit` against Neon; e2e step 3: orphan row re-queued after an API restart and judged AC. Mutation check: removing `enqueued_at IS NULL` fails both store tests |
| Rule: runners never connect to the database | ✅ (guard narrowed) | `go test ./runner/` → `TestRunnerHasNoDatabaseDependency` PASS. The guard now allows exactly `database/sql/driver` and `database/sql/internal` (interface-only packages pulled in by `google/uuid` and `rs/xid` through the S3 client); `database/sql`, pgx, lib/pq, gorm, sqlx and Gin stay forbidden. Called out for the review (ADR 0011) |
| Regressions (earlier exit tests) | ✅ | `make test-crash` PASS (Phase 3); `make test-api-e2e` PASS (Phase 4) after fixing its missing executable bit. Sandbox and engine code did not change, so `make test-adversarial` and `make test-sandbox` were not rerun |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/5-dev-host-docker` | Docker on the dev host; local S3 (RustFS) in Compose instead of MinIO | 1 (1 build) |
| `feat/5-test-storage` | Bundles, `storage` module, runner problem sources, version in the job, API publish | 9 (4 feat, 1 ci, 1 fix, 2 test, 1 docs) |
| `feat/5-judging-state` | `judging` status: migration, status stream, runner event, API watcher | 5 (4 feat, 1 docs) |
| `feat/5-sse-status` | `GET /submissions/:id/events` | 2 (1 feat, 1 docs) |
| `feat/5-reaper` | `enqueued_at`, `ReapUnqueued`, reaper loop | 3 (2 feat, 1 docs) |
| `test/5-e2e-redaction` | `make test-live-e2e`, determinism fix, script modes | 4 (2 test, 1 fix, 1 docs) |
| `docs/5-adr-report` | ADRs 0011 and 0012, FLOW.md, CLAUDE.md, this report and the summary | see Stats |

The phase branch also holds one direct commit, the phase start (`docs(docs): start phase 5 log and progress`).

## File-by-file changes
Generated with `git diff --name-status phase-5-start..<last commit>`; the three phase documents of this branch (`phase-5.md`, `phase-5-summary.md`, `PROGRESS.md`) are included.

### Added
| File | Purpose |
|---|---|
| `judge/problem/bundle.go` | `Pack` (deterministic tar.gz of `problem.yaml` and `tests/`, no solutions) and `Unpack` (only those paths; no links, dirs or path escapes; 64 MiB cap) |
| `judge/problem/bundle_test.go` | Round trip keeps the test-set version, `solutions/` excluded, six hostile tar entries rejected, garbage rejected |
| `storage/go.mod`, `storage/go.sum` | New module `leetforce/storage` (minio-go v7.3.0 and its dependencies; lockfile, generated) |
| `storage/storage.go` | S3 bundle store: `ConfigFromEnv`, `EnsureBucket`, `Ping`, `PutBundle` (no-op when the SHA-256 matches), `Stat`, `GetBundle`, `Hash` |
| `storage/storage_test.go` | Key validation, env config, and a round trip against real S3 (skipped without `LEETFORCE_S3_ENDPOINT`) |
| `runner/internal/problems/problems.go` | `Source` interface; `Dir` (old directory mode) and `S3` (fetch, verify version, cache by bundle hash, `ErrPermanent`) |
| `runner/internal/problems/problems_test.go` | Fetch once then cache, no-hash object, seven error cases (permanent vs retryable), refetch when the bundle changes under the same version, `Dir` slug checks |
| `queue/status.go` | `StatusEvent`, `PublishStatus` (capped XADD), `StatusTail`, `ReadStatus` (plain XREAD, no group) |
| `queue/status_test.go` | Publish and read, tail semantics, undecodable entries skipped, validation (against real Redis) |
| `api/internal/ingest/status.go` | `StatusWatcher`: reads the status stream from the tail and marks submissions judging; best effort |
| `api/internal/ingest/status_test.go` | Only valid judging events applied, database errors do not stop a batch, run loop starts at the tail and advances |
| `api/internal/server/events.go` | `GET /submissions/:id/events`: state-based SSE, keep-alive, time limit, error event, per-instance stream cap |
| `api/internal/server/events_test.go` | 9 tests incl. no hidden data in any event and no internal error text |
| `api/internal/reaper/reaper.go` | Sweep at startup then every 15 min: re-queue stored-but-never-queued submissions |
| `api/internal/reaper/reaper_test.go` | Re-queues with the stored version, survives queue failure, run loop cadence, defaults |
| `api/internal/store/reap.go` | `MarkEnqueued` and `ReapUnqueued` (one transaction, `FOR UPDATE SKIP LOCKED`) |
| `api/internal/store/reap_test.go` | Orphan, young, already-queued and judged rows; failing enqueue retried next sweep; limit respected (against Neon, throwaway schema) |
| `api/migrations/00002_judging_status.sql` | Allows `submissions.status = 'judging'` |
| `api/migrations/00003_enqueued_at.sql` | `submissions.enqueued_at`, backfill of old rows, partial index for the reaper |
| `scripts/test-live-e2e.sh` | The Phase 5 exit test (SSE order, redaction sweep with detector self-test, reaper recovery, cleanup of its own rows) |
| `docs/adr/0011-problem-tests-in-object-storage.md` | Bundles in a bucket, RustFS instead of MinIO, runner guard narrowed |
| `docs/adr/0012-live-status-sse-and-reaper.md` | Status stream, SSE by polling the database, reaper design and its Neon cost |
| `docs/phases/phase-5-log.md` | Working log of the session |
| `docs/phases/phase-5.md`, `docs/phases/phase-5-summary.md` | This report and the plain-language summary |

### Modified
| File | What changed | Why |
|---|---|---|
| `.env.example` | `LEETFORCE_MINIO_*` replaced by `LEETFORCE_S3_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET/USE_TLS` | the object store is no longer MinIO and the API and runner need a bucket |
| `docker-compose.yml` | Service `minio` replaced by `s3` (`rustfs/rustfs:1.0.0`, localhost-only ports, volume `s3-data`) | MinIO images are no longer published |
| `Makefile` | `storage` added to `GO_MODULES`; `test-live-e2e` target; `dev` comment | new module and exit test |
| `go.work` | `./storage` added | new module |
| `commitlint.config.mjs`, `CLAUDE.md` | `storage` commit scope; `make test-live-e2e`, RustFS replaces MinIO in the stack and `make dev` lines | the new component; conventions kept current |
| `scripts/setup-dev-host.sh` | Docker (official apt repo) and `postgresql-client` install blocks; version lines | repeatable host setup; psql is used by the e2e |
| `scripts/scan-staged.sh`, `scripts/test-api-e2e.sh` | File mode 100644 to 100755 (no content change) | `make test-api-e2e` failed with Permission denied on the Linux host |
| `queue/queue.go` | `Job.TestSetVersion` (`test_set_version`, omitempty); comment on where tests come from | the runner must fetch the exact version |
| `runner/cmd/runner/main.go` | Chooses `problems.S3` when `LEETFORCE_S3_ENDPOINT` is set, else `problems.Dir`; `LEETFORCE_PROBLEM_CACHE` | tests from the bucket; directory mode kept for development |
| `runner/internal/agent/agent.go` | `Config.Problems` (defaults to `ProblemsDir`); loads via the source with the job's version; `JobQueue.PublishStatus`; publishes `judging` before judging (failure only logged) | use storage; live status |
| `runner/internal/agent/agent_test.go` | Two tests: judging reported before the verdict; a failing status publish does not stop the job | cover the new behaviour |
| `runner/nodb_test.go` | Skips exactly `database/sql/driver` and `database/sql/internal` | S3 client dependencies import the `Valuer` interface; the guard stays strict otherwise |
| `api/cmd/api/main.go` | S3 config and publish at startup, `storage` readiness, status watcher and reaper goroutines, `LEETFORCE_REAPER_*` | wire the new parts |
| `api/internal/catalog/catalog.go`, `catalog_test.go` | `Publish` (pack and store every problem); `TestPublish` | publish tests under their version |
| `api/internal/server/server.go` | `Events` config, stream slots, route `/submissions/:id/events` | SSE |
| `api/internal/server/submissions.go`, `submissions_test.go` | `InsertSubmission` returns the version, the job carries it, `MarkEnqueued` after enqueue; fakes and a test for the failure path | version to the runner; reaper bookkeeping |
| `api/internal/store/submissions.go`, `submissions_test.go`, `verdicts_test.go` | `InsertSubmission` `RETURNING test_set_version`; `StatusJudging`; `MarkJudging`; `TestMarkJudging` | judging state and the version in the job |
| `docs/FLOW.md`, `docs/PROGRESS.md` | Phase 5 ticked and its as-built flow; progress tracker | phase documentation |

### Deleted
| File | Reason |
|---|---|
| none | |

### Renamed / moved
| From | To | Reason |
|---|---|---|
| none | | |

## Key code changes
1. **Fetch the exact test set, verify it** (`runner/internal/problems/problems.go`): the S3 source stats the bundle, uses the cache directory for that SHA-256 if present, otherwise downloads, unpacks into a temp directory, loads the problem, **recomputes** the version and compares it with the job's before renaming into place:
   ```go
   if p.TestSetVer != version {
       return nil, fmt.Errorf("%w: bundle %s %s holds test set %s", ErrPermanent, slug, version, p.TestSetVer)
   }
   ```
2. **Judging can never undo a verdict** (`api/internal/store/submissions.go`): `UPDATE submissions SET status = 'judging' ... WHERE id = $1::uuid AND status = 'queued'`. Removing the guard makes `TestMarkJudging` fail.
3. **State-based SSE** (`api/internal/server/events.go`): the first event is the current state; later ones only on change; the stream ends after the verdict. A client that connects late sees the verdict at once; a judgement shorter than one poll can skip `judging`.
4. **Reaper in one transaction** (`api/internal/store/reap.go`): `SELECT ... WHERE status = 'queued' AND enqueued_at IS NULL AND created_at < now() - make_interval(secs => $1) ... FOR UPDATE SKIP LOCKED`, enqueue each row, mark the ones that succeeded, commit. Two instances never take the same row; a crash rolls the marks back.
5. **Deterministic bundles** (`judge/problem/bundle.go`): sorted entries, zero timestamps, fixed modes; `Unpack` accepts only `problem.yaml` and `tests/<name>` regular files.

## Decisions
- [ADR 0011](../adr/0011-problem-tests-in-object-storage.md): tests as bundles in a bucket keyed by test-set version, cached by bundle hash; RustFS 1.0.0 replaces MinIO locally; the runner dependency guard narrowed to allow two interface-only `database/sql` packages.
- [ADR 0012](../adr/0012-live-status-sse-and-reaper.md): best-effort status stream read with plain `XREAD`; SSE by polling Postgres (state, not history); reaper with `enqueued_at`, `SKIP LOCKED`, startup sweep plus a 15 minute interval.

## Tests
- New: 36 top-level tests (`bundle_test` 3, `storage_test` 3, `problems_test` 5, `queue/status_test` 3, `ingest/status_test` 3, `events_test` 9, `reaper_test` 4, `store/reap_test` 2, plus `TestPublish`, `TestMarkJudging` and 2 agent tests), one new subtest (accept even if marking enqueued fails), and the end-to-end `make test-live-e2e`.
- Commands (dev host, `set -a; . ./.env; set +a` first, otherwise the database and storage tests skip):
  ```bash
  make fmt lint test        # 0 issues in 5 modules, all ok
  make test-live-e2e        # PASS in about 45 s
  make test-api-e2e         # Phase 4 regression, PASS
  make test-crash           # Phase 3 regression, PASS
  ```
- Mutation checks run (each failed as intended, then restored): `MarkJudging` without `status = 'queued'`; reaper query without `enqueued_at IS NULL`; version exposed in the JSON.
- Security scans: `trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1` (Trivy 0.75.0) on every unit and on the final tree: 0 findings (api, judge, queue, runner, storage `go.mod`, `web/package-lock.json`). `trivy image rustfs/rustfs` (same digest as the pinned `1.0.0`): 0 HIGH/CRITICAL. `trivy config docker-compose.yml`: not scanned (no Compose checker). The secret scan ran on every commit.

## Known issues and deferred work
- **No Run path yet**: custom and sample input with failing-case details is Phase 8; today only Submit exists and it is verified to leak nothing.
- **One S3 credential pair** is shared by the API and the runner; read-only runner credentials come with infrastructure as code (Phase 12).
- **Old bundles and the results stream are never trimmed** (Phases 10 and 11).
- **Cost and quota checks still open**: the Neon plan and limits (the reaper wakes the database about every 15 minutes) and the Upstash command count (about 17,000 extra commands a day from the status loop) are not measured against any bill.
- **Test leftovers in Neon**: 1 problem, 31 submissions, 30 verdicts from earlier manual runs and from `make test-api-e2e`, which does not clean up (the new e2e does). One `print(1)` row from Phase 4 stays `queued` and is ignored by the reaper.
- **`judging` can be skipped** for very fast judgements (by design, ADR 0012).
- **Flaky-test lesson**: the first end-to-end version assumed `judging` would always be visible; it was not for a 300 ms judgement. Fixed in the test, not hidden.
- **Host state**: the host's Docker group membership takes effect at next login; a host-level Redis already listens on 6379, so the Compose `redis` service is not used there.
- **Untracked**: `web/AGENTS.md` and `web/CLAUDE.md` stay untracked (carried over).

## Stats
Counted for `phase-5-start..<sha of the commit that added the summary>`; the later review-Q&A and merge commits only touch documents already listed.
- Commits: STATS_COMMITS (excluding merges)
- Files: STATS_FILES
- Lines: STATS_LINES

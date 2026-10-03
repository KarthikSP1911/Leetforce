# Phase 10: Problem pipeline

**Branch:** `phase/10-problem-pipeline`
**Range:** `phase-10-start..phase-10-done`
**Dates:** 2026-10-03 (one session shared with Phase 9; see [phase-10-log.md](phase-10-log.md))
**Milestone:** none

## Summary
Problems can now be validated and fixed safely. `judge validate` checks a problem's files and runs its reference solutions in the sandbox; when a test set changes, the API notices the new test-set version at start-up, re-queues every submission judged against the old version, and the new verdict replaces the old one exactly once. Finding this required fixing a latent bug: the "already judged" marker in Redis ignored the test-set version, so a rejudge result would have been thrown away.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Fixing a test set triggers a rejudge of affected submissions | ✅ | `make test-rejudge-e2e` on the dev host: `PASS: a fixed test set re-queued the submission, the new verdict replaced the old one once, and repeating changed nothing` (WA at `ts-57b35a54a3fc496e`, was AC at `ts-fab2745cd5a1d86e`; `bin/rejudge -dry-run` then found 0 stale) |
| Problem import/validation | ✅ | `judge/validate` unit tests (`make test`); `make validate-problems`: `5 of 5 problems valid` |
| Reference-solution check | ✅ | `make validate-problems` judged every `solutions/<lang>/<verdict>.<ext>` in the sandbox on the dev host |
| Verdict writes stay idempotent | ✅ | `TestRecordVerdictIsIdempotent`, `TestRecordVerdictEdgeCases`, `TestRejudgeIEKeepsPreviousVerdict`, `TestRejudgeInFlightSubmission` (Postgres, no skips); `make test-api-e2e` PASS (duplicate verdict changed nothing) |
| Nothing else regressed | ✅ | `make test`, `make test-sandbox`, `make test-api-e2e`, `make test-crash`, `make test-live-e2e`, `make test-auth-e2e` all PASS; `make lint` 0 issues; Trivy `fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL` 0 findings (v0.75.0) |
| Adversarial suite | not run | No sandbox code changed (`git diff --stat main..HEAD -- judge/sandbox judge/engine` is empty) |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/10-problem-pipeline` | Validate, version-change detection, rejudge, verdict rule, marker fix, e2e script, ADR 0018 (written by a subagent) | 6 |
| `phase/9-auth-limits` | Phase 9 docs merged in so this branch carries them to `main` | 1 merge |

## File-by-file changes
`phase-10-start` was tagged on the Phase 9 branch before Phase 9's documents and four gate-found fixes were committed, so this list also shows those Phase 9 items (marked "P9"). Generated with `git diff --name-status phase-10-start..HEAD`.

### Added
| File | Purpose |
|---|---|
| `judge/validate/validate.go` | Structural checks: slug equals directory, title, difficulty, limits, NAME.in/NAME.out pairs, sizes (4 MiB per file, 32 MiB total, 3-200 tests), samples, at least one hidden test, warnings for a missing statement or starters |
| `judge/validate/reference.go` | Judges every `solutions/<lang>/<verdict>.<ext>` through the engine (behind a `Judger` interface) and requires that verdict; needs root and nsjail |
| `judge/validate/validate_test.go` | About 20 structural cases, directory discovery, solution naming, the reference check with a fake judge |
| `judge/cmd/judge/validate.go` | The `judge validate [-structure-only] [-strict]` subcommand and exit codes (0 valid, 1 invalid, 2 usage or host error) |
| `api/internal/store/rejudge.go` | `SyncProblem` (detects a version change under `FOR UPDATE`), `BeginRejudge` (one batch, `SKIP LOCKED`), `PendingRejudge` |
| `api/internal/store/rejudge_test.go` | Postgres tests: transitions, batching, idempotence, verdict replacement, IE keeps the old verdict, in-flight submission |
| `api/internal/rejudge/rejudge.go`, `rejudge_test.go` | `Sync`, `Rejudger.Run`, `RunChanged`: enqueue batches of 100 with the new version and mark them enqueued |
| `api/cmd/rejudge/main.go` | `rejudge [-dry-run] [-batch N] <slug>`: sync one problem and rejudge on demand |
| `queue/version_test.go` | The verdict marker is scoped to the test-set version (Redis) |
| `scripts/phase10-e2e.sh` | Phase 10 exit test (`make test-rejudge-e2e`) |
| `docs/adr/0018-problem-pipeline.md` | Decision record |
| `docs/phases/phase-10-log.md` | Running log (this phase) |
| P9: `queue/limit_test.go`, `scripts/test-auth-e2e.sh` | Limiter tests on real Redis; Phase 9 exit test script |
| P9: `docs/phases/phase-9.md`, `phase-9-log.md`, `phase-9-summary.md` | Phase 9 documents |

### Modified
| File | What changed | Why |
|---|---|---|
| `api/internal/store/verdicts.go` | A verdict replaces the stored one only if its version equals the submission's current version and differs from the stored one; stale versions are dropped; IE never replaces a real verdict | Rejudge needs replacement; idempotence and late deliveries must stay safe |
| `queue/queue.go`, `queue_test.go` | `Publish`/`Published` scope the marker key to the test-set version (`Published(ctx, id, version)`) | A rejudge job was otherwise acknowledged as already judged and its result dropped |
| `runner/internal/agent/agent.go` | Passes the job's version to `Published`; IE results carry the job's version | Follow the queue change |
| `api/cmd/api/main.go` | Problem sync goes through `rejudge.Sync`; `RunChanged` runs after the queue is open | Fixing a test set rejudges automatically at start-up |
| `judge/cmd/judge/main.go` | Registers `validate` | New subcommand |
| `Makefile` | Targets `validate-problems`, `test-rejudge-e2e`, `test-auth-e2e`; `.PHONY` line repaired (two names had been joined) | New gates |
| P9: `api/internal/server/auth.go`, `auth_test.go` | `//nolint:gosec` with reasons on the cookie | The Secure flag follows the request scheme on purpose |
| P9: `scripts/phase9-e2e.py` | Host and port for the SSE check come from `LEETFORCE_API` | Run on any port |
| P9: `docs/FLOW.md`, `docs/PROGRESS.md` | Phase 9 flow and status | Documentation rule |

### Deleted
| File | Reason |
|---|---|
| (none) | |

### Renamed / moved
| From | To | Reason |
|---|---|---|
| (none) | | |

The list was generated before the closing documents (this report, `phase-10-summary.md`, the Phase 10 parts of `docs/FLOW.md`, `docs/PROGRESS.md`, `CLAUDE.md`, `.env.example`) were committed; they are in the final range.

## Key code changes
1. **Rejudge in one statement per batch.** `BeginRejudge` locks stale rows with `FOR UPDATE SKIP LOCKED`, moves them to the problem's current version, status `queued`, `enqueued_at` NULL, and returns id, language and source. Two API instances cannot take the same row.
2. **A failed enqueue is retried by the existing reaper.** The rejudge routine calls `MarkEnqueued` only after the job is on the queue; otherwise `enqueued_at` stays NULL and the Phase 5 reaper re-queues it with the new version.
3. **The verdict rule.** In `RecordVerdict` the new verdict wins only when `incoming.version == submission.version && incoming.version != stored.version`. This keeps duplicates, crash recovery and late old-version results harmless.
4. **The marker bug.** `<prefix>:verdict:<id>` is now `<prefix>:verdict:<id>:<version>` when a version is given; without a version the key is unchanged, so old callers (for example `lfq`) behave as before.

## Decisions
- [ADR 0018](../adr/0018-problem-pipeline.md): validate as a judge subcommand in two steps, version change detected on API start with automatic rejudge plus an on-demand command, verdict replacement rule, old bundles kept.

## Tests
- New: `judge/validate/validate_test.go`, `api/internal/rejudge/rejudge_test.go`, `api/internal/store/rejudge_test.go`, `queue/version_test.go`, `queue/limit_test.go`; end to end `scripts/phase10-e2e.sh`.
- Commands (dev host, `.env` loaded): `make test`, `make validate-problems`, `make test-rejudge-e2e`. The queue tests need `LEETFORCE_TEST_REDIS_URL` (set it to `LEETFORCE_REDIS_URL`), otherwise they skip; they were run with it set.

## Known issues and deferred work
- While a rejudge is pending the submission has status `queued` and still has its old verdict row; the API and the web UI do not special-case this (the Submissions tab shows it as queued until the new verdict arrives).
- The rejudge runs before the API starts listening, so a very large rejudge delays readiness; batching and a background worker are a later refinement.
- A job enqueued without a version (`lfq`) and a result with a computed version use different marker keys, so such a job can be judged twice; `Publish` still records one result.
- Old test bundles are never deleted from object storage (storage cost grows with each test change).
- `make test-adversarial` was not run (no sandbox changes); Trivy ran once on the final tree, with 0 findings.
- The new pages from Phase 9 are still not looked at in a browser.

## Stats
Reproduce with:
```bash
git diff --name-status phase-10-start..phase-10-done
git diff --stat phase-10-start..phase-10-done
git log --oneline --no-merges phase-10-start..phase-10-done
```
Before this correction commit: 34 files changed, 2605 insertions, 43 deletions; 14 commits excluding merges; 2 merge commits. The range includes the Phase 9 documents and four Phase 9 fixes (see "File-by-file changes").

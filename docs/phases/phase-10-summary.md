# Phase 10 summary: Problem pipeline

## TL;DR
- A new command, `judge validate`, checks that a problem is well formed and that its reference solutions get the verdicts they claim, before the problem is published.
- When a problem's tests change, the API notices at startup (the test-set version, a content hash of the checker and all tests, changes) and automatically re-queues every affected submission. The new verdict replaces the old one exactly once.
- Along the way I found and fixed a bug that would have silently thrown away every rejudge result: the Redis "already judged" marker was keyed by submission id only.

## Where this phase fits
```
 browser --> API (P4, P9 auth + limits) --> Redis Streams queue (P3) --> runner (P3) --> sandbox (P1, P2, P6)
 [P7-8 UI]      |                                ^                          |
                |                                |                          v
                |   [P10 NEW: rejudge on test-set version change] --> results stream --> API ingest (P4)
                |   [P10 NEW: verdict replace rule]                                          |
                v                                                                            v
 [P10 NEW: judge validate, before publishing]                                          Postgres (verdicts)
                                                                                             |
 browser <-- SSE / poll (P5, P8) <-----------------------------------------------------------+

 Still to come: metrics and dashboards (P11), infra as code (P12), cloud (P13), contests (P14-15)
```
- Phase 10 depends on Phase 4 (problem sync, verdict writes), Phase 3 (queue, reaper) and Phase 9 (submissions owned by users).
- It unblocks Phase 11 (a rejudge is one more flow to watch) and the contest phases (a contest problem fix must be able to rejudge).

## What I built and why
All three units were written by one subagent on `feat/10-problem-pipeline` and merged with `--no-ff`; I added the follow-up fixes listed at the end.

### Validate a problem (`judge/validate`, `judge/cmd/judge/validate.go`)
- **What:** `judge validate [-structure-only] [-strict] <problem-dir|problems>`.
- **Why:** nothing checked a problem before publishing. A missing output file or a "reference" solution that does not actually pass would only show up when users submitted.
- **How it works:** step one is structural and needs no sandbox: slug equals directory name, limits in range, `NAME.in`/`NAME.out` pairs with no orphans, size limits, samples and at least one hidden test, and statement and starters (warnings, or failures with `-strict`). Step two (`judge/validate/reference.go`) judges every `solutions/<lang>/<verdict>.<ext>` through the real engine and requires exactly that verdict, plus an `ac` solution per language. Exit code 0 is valid, 1 invalid, 2 usage or host error. `make validate-problems [DIR=...]` runs it.
- **Alternatives considered:** running the check in the API or a runner (it is an authoring-time step, so a CLI). See [ADR 0018](../adr/0018-problem-pipeline.md).

### Detecting a test-set version change (`api/internal/store/rejudge.go`)
- **What:** `store.SyncProblem` upserts a problem and returns the old and new version.
- **Why:** to rejudge you must know the tests changed. Before, the upsert just overwrote the version.
- **How it works:** one transaction reads the stored version with `SELECT ... FOR UPDATE`, then upserts. Old test bundles stay in object storage because their keys include the version.

### Rejudge (`api/internal/rejudge`, `store.BeginRejudge`, `api/cmd/rejudge`)
- **What:** stale submissions are moved to the current version and re-queued.
- **Why:** without it a fixed test leaves wrong verdicts forever.
- **How it works:** `BeginRejudge` picks, in batches of 100 with `FOR UPDATE SKIP LOCKED`, judged rows whose verdict is for another version, plus queued or judging rows on an older version. In one statement it sets their version to the current one, status `queued` and `enqueued_at` NULL. Only then does the routine enqueue a job per row and call `MarkEnqueued`. It runs at API start for each problem whose version changed (`api/cmd/api/main.go`), and on demand with `go run ./api/cmd/rejudge [-dry-run] [-batch N] <slug>`.
- **Alternatives considered:** enqueue first then update rows (a failed commit would leave jobs with a version the row lacks); a history table (deferred). See the ADR.

### Verdict write rule (`api/internal/store/verdicts.go`)
- **What:** a verdict is stored only for the submission's current version, and may replace a stored one only when the versions differ.
- **Why:** the old rule was "first write wins", which would have kept the stale verdict.
- **How it works:** late verdicts from old-version jobs, duplicates and same-version conflicts change nothing. An `IE` (internal error) verdict never replaces a real verdict; it only moves a re-queued row back to `judged`, so a failed rejudge keeps the old result.

### Redis marker scoped to the version (`queue/queue.go`, `runner/internal/agent/agent.go`)
- **What:** `Publish` and `Published(ctx, id, version)` key the marker by submission id and test-set version.
- **Why:** the marker `<prefix>:verdict:<id>` (kept 7 days) meant "this submission was judged". A rejudge job would have been acknowledged as already done and its result dropped.
- **How it works:** the runner passes the job's version; its internal-error result carries the version too.

### Follow-ups from the test run
- `fix(api)`: gosec finding on the session cookie (`Secure` follows the request scheme), suppressed with the reason.
- `fix(judge)`: the validator's per-file limit was 1 MiB, but the existing `max-subarray-sum` has a 1,100,007-byte test; now 4 MiB (the 32 MiB total stays).
- `fix(ci)`: the agent's Makefile edit had glued two names together in `.PHONY`.

## How it works now, step by step
1. An author fixes a test file in `problems/<slug>/tests/` and runs `make validate-problems DIR=problems/<slug>` on the dev host; reference solutions must still get their stated verdicts.
2. The API starts. For each problem it publishes the bundle to object storage and calls `SyncProblem`. The content hash differs, so the problem is reported as changed.
3. For that problem `BeginRejudge` moves stale submissions to the new version and `queued`, with `enqueued_at` NULL, and returns their source.
4. The routine enqueues one job per row carrying the new version and marks the rows enqueued.
5. A runner takes the job, checks the Redis marker for that id and version (absent), fetches the new bundle, judges, and publishes the result.
6. The ingest writes the verdict: the version matches the submission's, differs from the stored one, so the old verdict is replaced once. The SSE stream and submission list show the new verdict.
7. Restarting the API again finds nothing stale; `bin/rejudge -dry-run` reports 0.

## Key concepts
- **Test-set version:** a hash of the checker and all tests. It changes exactly when the answer key changes, so it says which verdicts are out of date.
- **Reference solution:** a known solution stored with the problem that must get a known verdict. It proves the tests and limits are sane.
- **Rejudge:** judging an already judged submission again against new tests.
- **Idempotent:** doing it twice has the same effect as once. Needed because the API may restart mid-rejudge.
- **`FOR UPDATE` / `SKIP LOCKED`:** Postgres row locks; the first stops two writers racing on one row, the second lets a second worker skip rows another already holds.
- **Reaper:** the existing background task that re-queues `queued` rows whose `enqueued_at` is NULL. The rejudge reuses it as its safety net.
- **Dead letter / IE:** a job that failed too often is parked and reported as an internal error, not as the user's fault.

## Try it yourself
```bash
# on the dev host, repo root, .env supplies DATABASE_URL and LEETFORCE_REDIS_URL
make validate-problems                 # expect 5 of 5 problems valid (sample-sum warns: no statement or starters)
make test-rejudge-e2e                  # scripts/phase10-e2e.sh: copy a problem, judge AC, change a test, restart the API
sudo -n bin/judge validate -structure-only problems   # no sandbox needed
```
Expect the e2e test to end in PASS: the same submission becomes WA at the new version, one verdict row, repeating changes nothing, and `bin/rejudge -dry-run` finds 0 stale. It uses its own problem slug and Redis prefix and deletes the rows it creates.

## Trade-offs and risks
- While a rejudge is pending a submission is `queued` but still has its old verdict row. A client that shows any existing verdict shows the old one until the new lands; acceptance briefly counts old verdicts.
- The rejudge runs at API start, before it listens. A large rejudge delays startup; I did not measure throughput.
- Every change to a problem's tests costs one judging job per affected submission, including changes that are reverted.
- If a rejudge fails with an IE, the old verdict stays and the row is retried by the next startup or `rejudge <slug>`.
- An `lfq` tool job with no version and a computed-version result use different marker keys, so a redelivered tool job may be judged twice. Real submissions always have a version.
- Not run: `make test-adversarial`, because no sandbox code changed (`git diff --stat main..HEAD -- judge/sandbox judge/engine` was empty).

## Review questions
Skipped by the owner (Phases 9 and 10 in one session, no stops). The questions I would have asked:

Understanding:
1. Why was the Redis verdict marker scoped to the test-set version, and what would have happened to a rejudge result without that?
2. Why can an `IE` verdict never replace a real verdict?
3. Why does a failed enqueue leave `enqueued_at` NULL instead of rolling anything back?
4. What happens if the test set changes while a submission is judging?
5. Why does `judge validate` need root for the reference-solution check but not for the structural checks?

Decisions for the owner:
- A. Should a rejudge run automatically at API start whenever a version changes, or only on an explicit command?
- B. Should old test bundles be kept in object storage forever, or pruned once no submission uses them?

## Review Q&A
Skipped by the owner's instruction. Nothing was answered on the owner's behalf. Decisions stay on Claude's defaults: A, rejudge runs automatically at API start (the command exists for retries); B, old bundles are kept forever.

## Open decisions
- A and B above (revisit B when storage cost matters, Phase 13).
- Whether a verdict history table is wanted (deferred, ADR 0018).
- Carried over: earlier review answers listed in `docs/PROGRESS.md`; `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Handoff
- **State:** branch `phase/10-problem-pipeline`, tags `phase-10-start` and `phase-10-done`; M3 was tagged at Phase 9. All gates passed on the dev host: `make test`, `test-sandbox`, `test-api-e2e`, `test-crash`, `test-rejudge-e2e`, `test-live-e2e`, `test-auth-e2e`, and `validate-problems` (5 of 5 valid after raising the per-file limit to 4 MiB). Trivy found 0 HIGH/CRITICAL. The adversarial suite was not run (no sandbox code changed). Migration 00005 is applied to Neon. The dev host is still up and billable.
- **Next phase:** 11 - Observability: Prometheus metrics with the `leetforce_` prefix, Grafana, Loki, and queue-depth and runner-health alerts. Exit: dashboards show a live submission flow.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-10-summary.md, then start Phase 11 (Observability). Ask me the recap question and show me the session plan before writing any code.
  ```

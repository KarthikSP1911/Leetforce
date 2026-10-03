# Phase 8 summary: Web: run, submit, results

## TL;DR
- The workspace now works end to end in the browser: **Run** (on the sample tests or on your own input) and **Submit** (against all tests, including hidden ones) both give a verdict in a result panel.
- Run is a new, separate path: it goes through the same queue and runners but its result lives in a Redis key that expires after 10 minutes and never reaches Postgres, so it cannot change acceptance or show up as a submission.
- Submit shows live status (Queued, Judging, verdict) over Server-Sent Events with a polling fallback, and a Submissions tab lists your recent attempts. Hidden test data never appears in any Submit response.

## Where this phase fits
```
 SUBMIT (hidden tests, verdict only)
 browser --> API --------> Postgres --> queue --> runner --> sandbox --> verdict
   |  [P8 NEW: UI,     [built P4]     [built P3] [built P3]  [built P1,2,6]  |
   |   client id]                                                            v
   |                                        results stream --> API ingest --> Postgres (idempotent, P4)
   |<------------- SSE: queued -> judging -> verdict (P5 server, P8 client) <-+
   |               fallback: poll GET /submissions/:id every 1 s (P8 NEW)

 RUN (samples or custom input, details allowed)
 browser --> POST /runs --> Redis key run:<id> (queued) --> same queue --> same runner
   [P8 NEW]  [P8 NEW]                                                        |
                                                                             v
                                                       sample tests only, or RunCustom
                                                                             |
 browser <-- poll GET /runs/:id every 500 ms <-- Redis key run:<id> (done, 10 min TTL) <-+
   (never the results stream, never Postgres)

 Still to come: login and rate limits (P9), problem pipeline (P10), metrics (P11), cloud (P12-13)
```
- Phase 8 depends on Phase 7 (the workspace and the Monaco editor), Phase 5 (the SSE stream and the verdict view) and Phase 3 (queue and runner).
- It unblocks Phase 9: once real users exist, the anonymous browser id used here is replaced by a user id, and `POST /runs` gets limits.

## What I built and why
### Run endpoint (`feat/8-api-run-endpoint`)
- **What:** `POST /runs` and `GET /runs/:id` on the API, a Run job kind in the queue, a runner handler, and a custom-input mode in the judge engine.
- **Why:** Run needs to show more than Submit may (input, expected output, actual output, stderr), but untrusted code may only run in the sandbox, so it cannot run in the API process. Without a separate path, run details could leak hidden data or run attempts could count towards acceptance.
- **How it works:**
  - `api/internal/server/runs.go` validates the request, writes state `queued` to a Redis key, and enqueues a job with id `run-<uuid>` and kind run.
  - `queue/run.go` stores the state under `<prefix>:run:<id>` with a 10 minute TTL (`SetRun`, `GetRun`).
  - `runner/internal/agent/run.go` (`processRun`) keeps only sample tests, then calls `engine.Judge` with `ContinueOnFail` and `Detail`; for custom input it calls `engine.RunCustom` (`judge/engine/engine.go`), which compiles and runs the program once under the language limits.
  - A dead-lettered run job is ended with an `IE` result in its key by `handleDeadRun` (`api/internal/ingest/ingest.go`).
- **Alternatives considered:** running in the API (breaks the sandbox rule), a flag on Submit, a second stream, storing runs in Postgres. See [ADR 0016](../adr/0016-run-and-submit-paths.md).

### List submissions (`feat/8-api-list-submissions`)
- **What:** `GET /problems/:slug/submissions` returns the newest 50 submissions of one browser for one problem.
- **Why:** the Submissions tab needs history, and there are no user accounts yet.
- **How it works:** migration `api/migrations/00004_submission_client.sql` adds a nullable `client_id` column. `POST /submissions` stores the `X-LeetForce-Client` header after the insert; `listSubmissions` in `api/internal/server/submissions.go` returns the verdict view only (never source, test-set version or stderr). A missing or malformed id gives an empty list with no database query.
- **Alternatives considered:** waiting for login (would pull Phase 9 forward). See ADR 0016.

### SSE client (`feat/8-web-sse-client`)
- **What:** typed API calls and two watchers in `web/src/lib/api/client.ts` and `web/src/lib/api/watch.ts`.
- **Why:** the browser must follow a submission live, and still work when a proxy or network breaks the stream.
- **How it works:** `watchSubmission` opens an `EventSource` on `/api/submissions/:id/events` and handles `status` and `verdict` events. On a stream error, a `timeout` event, a bad payload, or no `EventSource` support, it polls `GET /submissions/:id` every second, giving up after 2 minutes. `watchRun` polls `GET /runs/:id` every 500 ms and stops on a 404.
- **Alternatives considered:** polling only (slower to react, more requests) and SSE only (fails behind buffering proxies).

### Run, Submit and result panel (`feat/8-web-run-submit`, covering the planned result-panel unit)
- **What:** Run and Submit buttons are enabled; the console switches to a Result tab; the Testcase tab chooses Samples or Custom input; `ResultPanel.tsx` shows the outcome.
- **Why:** this is the point of the phase: a user can write code and see a verdict.
- **How it works:** `web/src/hooks/useJudge.ts` owns the request lifecycle. Ctrl+Enter runs and Ctrl+Shift+Enter submits, registered with Monaco's `addCommand` because the editor swallows Ctrl+Enter. The verdict is the largest text (24px bold) and always carries its label, then runtime, memory and "passed / total". Submit never shows input, expected output or stderr; Run shows them only for failing samples. Colours come only from token aliases; "Judging" uses a sky dot beside normal text.
- **Alternatives considered:** one button with a mode switch (hides the security difference between the paths).

### Submissions tab (`feat/8-web-submissions-tab`)
- **What:** `web/src/components/workspace/SubmissionsTab.tsx` lists this browser's attempts.
- **Why:** users expect to see past results and re-check a pending one.
- **How it works:** it refetches when `useJudge` reports a created or judged submission, and re-reads every 2 seconds while any row is queued or judging.

### End-to-end check (`test/8-e2e`)
- **What:** `scripts/phase8-e2e.py` plus `store.TestTestSetVersion`, and a browser check in both themes.
- **Why:** the exit criterion is that the whole flow works and that no hidden data leaks.
- **How it works:** the script drives the real API and a runner on the dev host: Run on samples, a failing Run (details for the failing samples only), custom input, a compile error, Submit AC and WA with a check that no response contains `expected`, `input`, `stderr`, `actual`, `source` or `test_set_version` or any test file text, the SSE sequence, and the Submissions list for the right and wrong client id.

## How it works now step by step
**One Run (samples):**
1. You press Ctrl+Enter. `useJudge` calls `POST /api/runs` with problem, language and source.
2. The API validates, looks up the test-set version, writes `queued` to the Redis key `run:run-<uuid>` and enqueues a job of kind run. It answers 202 with the id.
3. A runner takes the job, sets `judging`, loads the problem bundle and keeps only sample tests.
4. The runner judges them in the sandbox with detail on, then writes `done` plus the result to the same key (10 minute TTL).
5. The browser, polling every 500 ms, sees `done`, and `ResultPanel` shows the verdict, runtime, memory and, for failing samples, input, expected, actual and stderr.

**One Submit:**
1. You press Ctrl+Shift+Enter. `POST /api/submissions` carries the `X-LeetForce-Client` header.
2. The API inserts the row (tagging `client_id`), enqueues the job and answers 202.
3. The runner judges against all tests with detail off and publishes the verdict to the results stream; the API ingests it idempotently into Postgres.
4. The browser, listening on the SSE stream, sees `queued`, `judging`, then `verdict`; if the stream breaks it polls instead.
5. `ResultPanel` shows the verdict, runtime, memory and passed/total, and the Submissions tab refreshes. No hidden input, expected output or stderr is sent.

## Key concepts
- **Server-Sent Events (SSE):** a one-way, long-lived HTTP response in which the server pushes small named events to the browser; LeetForce uses it so the verdict appears the moment it exists, without repeated asking.
- **Polling fallback:** asking the server for the state on a timer; used when the SSE stream cannot work (a proxy that buffers, a dropped connection) so the user still gets the verdict.
- **TTL (time to live):** a lifetime after which Redis deletes a key by itself; Run results use 10 minutes because they are throwaway.
- **Job kind:** a field on a queued job saying what to do with it; empty means Submit, `run` means Run. It lets both share one queue and one set of runners.
- **Anonymous client id:** a random id the browser keeps in `localStorage` (`lf-client`) and sends as a header; it groups one browser's submissions until login exists. It is not a credential: anyone who knows it can list that browser's verdict views.
- **Idempotency of verdicts:** storing a verdict twice for the same submission id changes nothing; it is what makes crash recovery and duplicate deliveries safe. Run results are not part of it because they never reach the database.
- **Hidden vs sample tests:** sample tests are shown in the statement and may be revealed in Run details; hidden tests decide the verdict on Submit and their input, expected output and the program's stderr are never returned.

## Try it yourself
```bash
# terminal 1 (repo root; .env supplies DATABASE_URL and LEETFORCE_REDIS_URL)
set -a; . ./.env; set +a
LEETFORCE_S3_ENDPOINT= LEETFORCE_PROBLEMS_DIR="$PWD/problems" go -C api run ./cmd/api
# terminal 2
cd web && npm run dev
# terminal 3, on a Linux host with nsjail (the dev host), repo checked out, same .env
make build-runner
sudo -E env LEETFORCE_S3_ENDPOINT= LEETFORCE_PROBLEMS_DIR="$PWD/problems" LEETFORCE_RUNNER_ID=try-p8 bin/runner
# terminal 4, repo root, API running and a runner active
python scripts/phase8-e2e.py
```
The runner must run on a Linux host with nsjail and root (or the Phase 6 service setup); it cannot run on Windows. Migration 00004 must be applied first (`make migrate-up`; already applied to Neon in this session).

Expect: in the browser at http://localhost:3000/problems/sample-sum, Ctrl+Enter shows Accepted with Sample 01 and 02; Submit shows Accepted and "5 / 5 test cases passed"; the Submissions tab lists the attempt; Custom input `4 10 20 30 40` shows "Ran Successfully" with output 100. The script prints a PASS line per check (Run samples, failing Run details, custom input, compile error, Submit AC and WA with no hidden data, SSE order, Submissions list) and finishes without a FAIL. It writes 3 submissions to the real database.

## Trade-offs and risks
- Run and Submit share runners, so heavy Run traffic can slow Submits. There is no limit on `POST /runs` until Phase 9.
- The client id is not a credential and is lost when `localStorage` is cleared; old rows are not migrated to users in Phase 9.
- A Run result vanishes after 10 minutes; a browser that polls past that gets a 404 and shows "The run expired. Run again."
- Not checked in a browser: a failing Run in the UI (covered by the API check), Ctrl+Shift+Enter, narrow screens, the SSE-to-polling fallback, and the queued and judging states visually. The saved e2e script was edited after its last full run (one check was corrected and re-verified inline).
- Migration 00004 touched the real Neon database; it is additive and reversible with `make migrate-down`.

## Review questions
Asked in chat at the end of the phase. Answers are recorded after the review.

Understanding:
1. Why is Run a separate job kind with its own Redis key instead of a flag on Submit that writes a row marked "run"?
2. What happens if the Redis run key expires while the browser is still polling, and what does the user see?
3. `wa.py` fails Submit with WA but passes Run. Why, and is that a bug?
4. Why is the anonymous client id not a credential, and what could someone do who learns another browser's id?
5. A corporate proxy blocks the SSE stream. What does the web app do, and what does the user notice?

Decisions for the owner:
- A. Keep the anonymous client id design until Phase 9, or replace it earlier?
- B. Add a rate limit to `POST /runs` now, or leave it to Phase 9 (limits per user and per IP)?

## Review Q&A
Filled in after the review.

## Open decisions
- Light-mode difficulty text contrast (Easy is 3.30:1, under AA 4.5:1) because the `--lf-success` token is fixed: keep it, or add a darker light-mode shade.
- Monaco from the CDN or bundled (Phase 12 at the latest).
- Carried over from `docs/PROGRESS.md`: Phase 7 review skipped (decisions A, B, C on Claude's defaults); Phase 6 ADR 0013 and 0014 confirmations; Phase 5 and Phase 4 review answers; Neon plan and limits unchecked; Phase 2 decisions A and C on defaults; `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Handoff
- **State:** branch `phase/8-web-run-submit`, tag `phase-8-start`; nothing running locally (API, web dev server and runner stopped). The EC2 dev host `leetforce-dev` is still up and billable.
- **Next phase:** 9 - Auth and limits (M3): real users and abuse protection. Build: sign-up and login, sessions, rate limiting per user and per IP, solved status. Exit: limits enforced and tested, a usable product on one machine. Think beforehand about how login replaces the client id and which limits `POST /runs` and `POST /submissions` need.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-8-summary.md, then start Phase 9 (Auth and limits). Ask me the recap question and show me the session plan before writing any code.
  ```

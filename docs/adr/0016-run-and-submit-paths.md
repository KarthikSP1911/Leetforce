# 0016. Run is a separate job kind through the same queue and runner; Submit is unchanged

**Status:** Accepted (Phase 8). The owner answered the session plan with "yes", so option A of the plan (a new `POST /runs` endpoint through the queue and runner, results kept in Redis only) and option B (an anonymous browser id for the Submissions tab until Phase 9) are the recorded defaults.

## Context
Phase 8 puts Run and Submit in the browser. They have different security needs. Submit judges against hidden tests and may only return the verdict, runtime and memory (CLAUDE.md security rules). Run is a convenience: it may show input, expected output, actual output and stderr, but only for sample tests or for the user's own input. Untrusted code must run only in the sandbox, so Run cannot be executed in the API process. Run results must also not count towards acceptance or appear in the submission history.

## Decision
- **Run is a separate job kind.** `queue.Job` gets `Kind` (`""` for Submit, `queue.KindRun` for Run), `Custom` and `Input` (`queue/queue.go`). Run jobs go onto the same jobs stream and are taken by the same runners, with the same heartbeat, `XAUTOCLAIM` reclaim and dead-letter behaviour. `Process` in `runner/internal/agent/agent.go` hands `KindRun` jobs to `processRun` (`runner/internal/agent/run.go`).
- **API:** `POST /runs` and `GET /runs/:id` (`api/internal/server/runs.go`). Body: `problem`, `language`, `source`, optional `input`. No `input` means "run the sample tests"; an `input` (up to 8 KiB, mirroring `engine.MaxInputBytes`) means "run once on this input". The run id is `run-` plus a UUID.
- **Result storage:** the run's state (`queued`, `judging`, `done` plus the result) lives in one Redis key, `<prefix>:run:<id>`, with a 10 minute TTL (`queue/run.go`, `SetRun` and `GetRun`). It is never published to the results stream and never written to Postgres, so a run cannot become a verdict or change acceptance. `GET /runs/:id` returns 404 once the key has expired.
- **Why the `run-` prefix:** a submission id is a UUID, and the ingest path rejects non-UUID ids, so a stray run result or dead letter cannot be stored as a verdict. A dead-lettered run job is ended with an `IE` result in its Redis key by `handleDeadRun` in `api/internal/ingest/ingest.go`, so the browser stops waiting; it never touches the database.
- **Sample runs:** the runner copies the problem, keeps only tests marked `Sample`, and calls `engine.Judge` with `Options{ContinueOnFail: true, Detail: true}`. Hidden tests are never in the copy, so `Detail` cannot expose them. A problem with no sample tests is a permanent error (the API answers 422 unless an input is supplied).
- **Custom input:** `engine.RunCustom` (`judge/engine/engine.go`) compiles the source and runs it once on the input under the language's limits. Its verdict is a judge verdict or `OK` (the program ran cleanly; there is nothing to compare against). Stdout and stderr are returned; compiler output only for CE.
- **Submit is unchanged:** same endpoint, same stream, same ingest. Responses and SSE events still carry only the verdict view; hidden input, expected output and raw stderr are never returned.
- **Anonymous client id for the Submissions tab.** The browser keeps a random id in `localStorage` (`lf-client`, `web/src/lib/api/client.ts`) and sends it as `X-LeetForce-Client`. `POST /submissions` stores it in the new nullable `submissions.client_id` column (`api/migrations/00004_submission_client.sql`) after the insert (best effort). `GET /problems/:slug/submissions` returns that client's newest 50 submissions, verdict view only. A missing or malformed id (must match `^[A-Za-z0-9-]{8,64}$`) returns an empty list without querying.

## Alternatives
- **Run in the API process:** simplest, no queue round trip, but it runs untrusted code outside the sandbox, which CLAUDE.md forbids.
- **A flag on Submit (`run: true`):** one endpoint, but run rows would enter Postgres and acceptance unless every query filtered them, and one missed branch could return hidden data on the Submit path. Keeping the two paths apart makes the redaction rule easy to audit.
- **A separate stream or consumer group for runs:** isolates run load from submissions, but needs a second set of setup, reclaim and dead-letter code and a second runner loop. One queue is enough at the current scale; revisit if run traffic starves submissions.
- **Storing runs in Postgres with a cleanup job:** durable but pointless for throwaway data, and risks counting them.
- **Login for the Submissions tab now:** Phase 9 builds auth; doing it here would pull that phase forward.

## Consequences
- A run costs one queue round trip and one runner slot, so Run and Submit compete for the same runners. There is no per-user or per-IP limit on `POST /runs` until Phase 9.
- Run results disappear after 10 minutes. The web client polls `GET /runs/:id` every 500 ms and gives up after 2 minutes (`web/src/lib/api/watch.ts`).
- The client id is **not a credential**. Anyone who learns an id can list that browser's submissions for a problem, and clearing `localStorage` loses the history. The list shows only verdict views, never source, test data or stderr, so the exposure is limited. Phase 9 replaces it with a user id; old rows stay tagged with the client id and are not migrated.
- Migration 00004 is additive and reversible (`make migrate-down`); at the time of writing it is not yet applied to Neon.
- Verification status (runner and sandbox-backed tests on the dev host, end-to-end browser check) is recorded in `docs/phases/phase-8-log.md`; no measurements are claimed here.

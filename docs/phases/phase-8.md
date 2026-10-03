# Phase 8: Web: run, submit, results

**Branch:** `phase/8-web-run-submit`
**Range:** `phase-8-start..phase-8-done`
**Dates:** 2026-10-03 (first commit 10:28, last 18:16, +0530)
**Milestone:** none (M3 is Phase 9 in `docs/PLAN.md`)
**Log:** [phase-8-log.md](phase-8-log.md)
**ADR:** [0016](../adr/0016-run-and-submit-paths.md)

## Summary
The workspace can now judge code from the browser. Submit queues a submission, follows it over SSE and shows the verdict, runtime and memory. Run goes through a new `POST /runs` path (queue, runner, sandbox) that keeps its results in Redis only, can run samples or a custom input, and is the only path that shows failing-case details. A Submissions tab lists the current browser's own submissions using an anonymous client id until accounts arrive in Phase 9.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Submit from the browser and see verdict, runtime and memory | ✅ | Chrome on `localhost:3000/problems/sample-sum`: Submit gave Accepted, 45 ms, 10.4 MB, 5 / 5 passed (log, "Unit 7"). `scripts/phase8-e2e.py`: Submit AC (5/5) and WA (2/5), SSE sent `queued`, `judging`, `verdict` |
| Failing-case details only for Run | ✅ | `scripts/phase8-e2e.py`: Run on a program failing the samples returned input, expected, actual, stderr for the two failing samples only; Submit AC and WA responses contained none of `expected`, `input`, `stderr`, `actual`, `source`, `test_set_version` nor any test file text. `TestProcessRunSamplesOnly` pins that hidden tests cannot reach a Run result |
| Run on custom input | ✅ | e2e: custom input returns `OK` with stdout; browser: input `4 10 20 30 40` gave "Ran Successfully", output 100; `TestRunCustom` (echo, crash, compile error) and `TestRunCustomErrors` in the sandbox |
| Submissions tab | ✅ | e2e: list returned this client's 3 submissions newest first, empty for another or no client id; browser (light mode) showed the new row |
| Both themes | ✅ | Run and Submit exercised in dark mode, Submissions tab and custom input in light mode |
| Sandbox and judge unchanged in behaviour (gate for engine change) | ✅ | Dev host: `make test-sandbox` all packages ok; `make test-adversarial` EXIT=0, 49 top-level PASS, no FAIL |
| Lint and security | ✅ | `make fmt lint` 0 issues in 5 modules; `npm run lint`, `tsc --noEmit`, `prettier --check`, `npm run build` clean; Trivy 0.75.0 `fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1 --skip-dirs node_modules,bin .`: 0 findings (vulnerability DB date not captured; no `.trivyignore`) |

## Branches merged
Generated from `git log --merges --oneline phase-8-start..HEAD` and `git log --no-merges` (17 commits).

| Branch | Purpose | Commits |
|---|---|---|
| `feat/8-api-list-submissions` | migration 00004, `GET /problems/:slug/submissions` | 1 |
| `feat/8-web-sse-client` | typed client, SSE watcher with polling fallback | 1 |
| `feat/8-web-run-submit` | result panel, `useJudge`, enabled Run and Submit, shortcuts (units 4 and 5) | 2 |
| `feat/8-web-submissions-tab` | Submissions tab | 1 |
| `feat/8-api-run-endpoint` | `RunCustom`, Run job kind, `POST /runs`, runner support, tests, ADR, FLOW, e2e script share this and `test/8-e2e` | 3 feature commits plus test and docs commits (phase branch was merged into it once, a8463bf, to resolve doc conflicts) |
| `test/8-e2e` | Phase 8 end-to-end script and version test | 1 |
| docs commits on the phase branch | log, progress, units 2 to 6 log | 3 |

## File-by-file changes
Generated from `git diff --name-status phase-8-start..HEAD` (35 files: 14 added, 21 modified, 0 deleted) before this report was added.

### Added
| File | Purpose |
|---|---|
| `api/internal/server/runs.go` | `POST /runs` (validates, takes the problem's current test-set version, enqueues a Run job) and `GET /runs/:id` (reads throwaway state from Redis) |
| `api/internal/server/runs_test.go` | handler tests: bad input, unknown problem, enqueue, state read, no submission row created |
| `api/migrations/00004_submission_client.sql` | nullable `submissions.client_id` plus a partial index for "my submissions"; reversible |
| `docs/adr/0016-run-and-submit-paths.md` | ADR for this phase |
| `docs/phases/phase-8-log.md` | running log |
| `docs/phases/phase-8.md` | this report |
| `queue/run.go` | `RunState`, `RunResult`, `SetRun` and `GetRun` with a 10 minute TTL; separate from the results stream |
| `runner/internal/agent/run.go` | `processRun`: filters the problem to samples, sets `Detail`, calls `RunCustom` for custom input, writes the Run state; never reports to the results stream |
| `runner/internal/agent/run_test.go` | tests incl. `TestProcessRunSamplesOnly` (hidden tests cannot appear in a Run result) |
| `scripts/phase8-e2e.py` | scripted API check: Run cases, hidden-data scan of Submit responses, SSE sequence, Submissions list isolation (writes 3 rows to the real DB) |
| `web/src/components/workspace/ResultPanel.tsx` | verdict as largest text with label, runtime, memory, passed/total; failing-case detail only for Run; token colours only |
| `web/src/components/workspace/SubmissionsTab.tsx` | lists this browser's submissions, refreshes after judging, re-reads every 2 s while a row is queued or judging |
| `web/src/hooks/useJudge.ts` | state machine for Run and Submit (create, watch, result) used by the workspace |
| `web/src/lib/api/watch.ts` | `watchSubmission` (EventSource, polls `GET /submissions/:id` on error or timeout) and `watchRun` (polls every 500 ms, gives up after 2 min) |
| `web/src/types/submission.ts` | TypeScript types for submissions, runs and results |

### Modified
| File | What changed | Why |
|---|---|---|
| `api/cmd/api/main.go` | `ing.SetRuns(q)`; `Versions: db, Runs: q` in `server.Deps` | wire the Run path and dead-run handling |
| `api/internal/ingest/ingest.go` | `RunSetter`, `SetRuns`, `handleDeadRun`: a dead-lettered Run job ends with an IE run result | a dead Run must not be stored as a submission IE and the browser must stop waiting |
| `api/internal/ingest/ingest_test.go` | `TestHandleDeadRun` (IE, no DB write; a failed write leaves it pending) | pins both behaviours |
| `api/internal/server/server.go` | `Versions`, `Runs` deps; routes for `/runs`, `/runs/:id`, `/problems/:slug/submissions` | expose the new endpoints |
| `api/internal/server/submissions.go` | tags a new row from `X-LeetForce-Client` after insert (best effort); `listSubmissions` returns `VerdictView` only; empty list for a missing or malformed id | per-browser history without leaking other users' rows or source |
| `api/internal/server/submissions_test.go` | tests list and tagging; assert no source, test-set version or stderr | security rule check |
| `api/internal/store/problems.go` | `TestSetVersion(slug)` | Run jobs carry the version so the runner fetches the same bundle |
| `api/internal/store/problems_test.go` | `TestTestSetVersion` | run against the real DB |
| `api/internal/store/submissions.go` | set client id, list by client and problem (newest 50) | Submissions tab data |
| `api/internal/store/submissions_test.go` | `TestListSubmissions` on a throwaway schema built from the migrations | exercises migration 00004 |
| `docs/FLOW.md` | Phase 8 ticked and "as built" section | CLAUDE.md rule |
| `docs/PROGRESS.md` | phase status and resume point | session state |
| `judge/engine/engine.go` | `RunCustom`, `CustomReport`, `MaxInputBytes` (8 KiB), `ErrInputTooLarge`; `runProgram` extracted from `runTest` | custom-input runs share the sandbox setup with judging, one code path for limits |
| `judge/engine/engine_test.go` | tests for `RunCustom` (echo, crash, compile error) and its errors | sandbox change gate |
| `queue/queue.go` | `Job.Kind`, `Job.Custom`, `Job.Input`, `KindRun` | Run jobs reuse the stream and consumer group |
| `runner/internal/agent/agent.go` | dispatches `KindRun` jobs to `processRun` | keep Run off the verdict path |
| `runner/internal/agent/agent_test.go` | adapted fakes to the new job fields | existing tests still pass |
| `web/src/components/workspace/CodeEditor.tsx` | `editor.addCommand` for Ctrl+Enter (Run) and Ctrl+Shift+Enter (Submit) | Monaco swallows Ctrl+Enter otherwise |
| `web/src/components/workspace/Tabs.tsx` | can be controlled | the console switches to Result after Run or Submit |
| `web/src/components/workspace/Workspace.tsx` | Run and Submit enabled; Testcase tab offers Samples or Custom input; Submissions tab added | the phase's UI |
| `web/src/lib/api/client.ts` | `createSubmission`, `getSubmission`, `listSubmissions`, `createRun`, `getRun`, `clientId()` (localStorage `lf-client`) | typed calls for the new endpoints |

### Deleted
None.

### Renamed / moved
None.

## Key code changes
- **Separate Run path** (`queue/run.go`, `runner/internal/agent/run.go`, `api/internal/server/runs.go`): run ids are `run-<uuid>` so they never parse as submission ids; results live in a Redis key with a 10 minute TTL and never reach the results stream or Postgres.
- **Hidden data cannot leak into Run** (`runner/internal/agent/run.go`): the runner filters the problem to sample tests before judging and sets `Detail`, so a Run result only ever contains sample details.
- **`RunCustom`** (`judge/engine/engine.go`): compiles, runs once on user input under the problem's limits, returns stdout, stderr (truncated to 4 KiB) and a verdict that is `Completed` when it ran cleanly. `runProgram` is shared with `runTest`.
- **Client id history** (`api/migrations/00004_submission_client.sql`, `submissions.go`): an anonymous id (`^[A-Za-z0-9-]{8,64}$`) lists only verdict-level fields; not a credential, replaced in Phase 9.
- **SSE with fallback** (`web/src/lib/api/watch.ts`): EventSource first, one-second polling on any stream problem.

## Decisions
- [ADR 0016](../adr/0016-run-and-submit-paths.md): Run through the queue and runner with Redis-only results; anonymous client id for the Submissions tab. Owner accepted the recommended defaults ("yes"): keep `--lf-success` and Monaco on the CDN; the dev host stays up.

## Tests
- Go: `go vet ./...` and `go test -count=1 ./...` in `api/` (with `.env` loaded for DB tests) pass, including `TestListSubmissions`, `TestTestSetVersion`, `TestHandleDeadRun`, handler tests; `make test` on the dev host passes.
- Sandbox: `make test-sandbox` (incl. `TestRunCustom`, `TestRunCustomErrors`) and `make test-adversarial` (EXIT=0, 49 PASS) on the dev host.
- End to end: `python scripts/phase8-e2e.py` against API, Redis, runner and Neon; browser check in Chrome in both themes.
- Web: `npm run lint`, `npx tsc --noEmit`, `npx prettier --check`, `npm run build` in `web/`. No automated browser tests.
- `make fmt lint`: 0 issues in all 5 modules. Trivy as above: 0 findings.

## Known issues and deferred work
- Narrow screens were not checked in a browser.
- Ctrl+Shift+Enter (Submit shortcut) was not tested in a browser; Ctrl+Enter was.
- The SSE to polling fallback was not exercised in a browser; the browser used the SSE path.
- A failing Run was verified through the API script, not visually in the UI; queued and judging states were not seen visually (the SSE stream carried them).
- `scripts/phase8-e2e.py` was edited after its last full run (the failing-sample Run case was re-verified inline, not by re-running the whole saved script).
- No rate limit on `POST /runs` (or submissions) until Phase 9; the client id is not authentication.
- Migration 00004 was applied to the real Neon database by `make migrate-up` (additive; `make migrate-down` reverses it). The e2e script wrote 3 submission rows there.
- Light-mode Easy text contrast (`--lf-success`) is 3.30:1, below AA; token kept as the owner chose the default.
- Monaco still loads from the CDN (default kept); bundling is a Phase 12 option.
- Trivy vulnerability DB date was not recorded.
- The dev host is still running and billable.
- The end-of-phase review with the owner has not happened; the recap question was not answered, and earlier phases' review answers are still outstanding (see PROGRESS.md).
- Solved status and users: Phase 9.

## Stats
Measured with `git log --no-merges phase-8-start..HEAD` and `git diff --stat phase-8-start..HEAD` before this report was added.
- Commits: 17 (excluding merges); 7 merge commits
- Files: 14 added, 21 modified, 0 deleted (35)
- Lines: +2232 / -70; this report and the summary add more

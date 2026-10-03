# Phase 8 log: Web: run, submit, results

Running log (CLAUDE.md "Documenting every step"). Entries: who, command, result, mistakes.

## Session 1

### Start of session
- Claude read CLAUDE.md, PROGRESS.md, phase-7-summary.md; repo state matched (main at 19f57a9, tags phase-0..7 present, untracked: .claude/, web/AGENTS.md, web/CLAUDE.md).
- Owner answered the plan with "yes": Claude's recommended defaults apply: (A) new `POST /runs` endpoint through the queue and runner, results kept in Redis only, never in Postgres; (B) anonymous browser id in localStorage for the Submissions tab until Phase 9; (C) keep the `--lf-success` token and Monaco on the CDN; (D) dev host stays up. The recap question was not answered.
- Claude ran `git checkout -b phase/8-web-run-submit` and `git tag phase-8-start`.

### Units (checklist)
- [ ] 1 feat/8-api-run-endpoint
- [ ] 2 feat/8-api-list-submissions
- [ ] 3 feat/8-web-sse-client
- [ ] 4 feat/8-web-run-submit
- [ ] 5 feat/8-web-result-panel
- [ ] 6 feat/8-web-submissions-tab
- [ ] 7 test/8-e2e

### Design notes (Claude, from reading the code before unit 1)
- `judge/engine` already has `Options.Detail` (input, expected, actual, stderr for failing sample tests only); it has no custom-input mode, so unit 1 adds one.
- The runner publishes verdicts to the results stream, which the API ingests into Postgres. Run jobs must not take that path, so run results go to a separate Redis key with a TTL.
- A dead-lettered run job reaches the API's `HandleDead`; it must be dropped, not recorded as a submission IE.

### Unit 2: list submissions (`feat/8-api-list-submissions`, merged 34a5f0e)
- Claude, a10c946. Migration `api/migrations/00004_submission_client.sql` adds `submissions.client_id` (nullable) and a partial index. NOT yet applied to the real Neon database (`make migrate-up` is still to run; it is additive and reversible with `make migrate-down`).
- `POST /submissions` tags the row from the `X-LeetForce-Client` header after the insert (best effort, logged on failure). `GET /problems/:slug/submissions` returns that client's newest 50 with `VerdictView` only. A missing or malformed id (must match `^[A-Za-z0-9-]{8,64}$`) returns an empty list and runs no query, so the endpoint cannot list other people's history. The id is not a credential; Phase 9 replaces it with a user id.
- Verified on Windows with `.env` loaded: `go vet ./...` and `go test -count=1 ./...` in `api/` pass, including `store.TestListSubmissions` (throwaway schema built from the migrations, so it exercises migration 00004) and handler tests that assert the response never contains source, test-set version or stderr.

### Unit 3: web client (`feat/8-web-sse-client`, merged 721cadc)
- Claude, 72f152a. `web/src/types/submission.ts`, `web/src/lib/api/client.ts` (`createSubmission`, `getSubmission`, `listSubmissions`, `createRun`, `getRun`, `clientId()` in localStorage key `lf-client`), `web/src/lib/api/watch.ts` (`watchSubmission`: EventSource on `/api/submissions/:id/events`, falls back to polling `GET /submissions/:id` every second on stream error, timeout or no EventSource; `watchRun`: polls every 500 ms, stops on 404, gives up after 2 min).
- Mistake: my first multi-file shell command failed to parse (nothing ran); files were recreated with the file tools.

### Units 4 and 5 combined: Run, Submit and result panel (`feat/8-web-run-submit`, merged 2dda231)
- Claude, fcc8f5f (`ResultPanel.tsx`) and c958537 (`useJudge.ts`, `Workspace.tsx`, `CodeEditor.tsx`, `Tabs.tsx`). Run and Submit are enabled; the console switches to Result; Testcase tab offers Samples or Custom input; Ctrl+Enter runs and Ctrl+Shift+Enter submits, registered with `editor.addCommand` because Monaco swallows Ctrl+Enter; `Tabs` can now be controlled.
- Result panel: verdict is the largest text (24px bold) with its label, then runtime and memory, then "passed / total". Submit never shows input, expected output or stderr; Run shows them only for failing samples. Colors use only the token aliases; "Judging" uses an `accent` (sky) dot beside foreground text, not sky text.
- Verified: `npm run lint`/`tsc --noEmit`/`prettier`/`npm run build` pass. NOT verified in a browser (needs the API, runner and a sandbox host).

### Unit 6: Submissions tab (`feat/8-web-submissions-tab`, merged 2afe568)
- Claude, cbc2ebb. `SubmissionsTab.tsx` lists the browser's submissions, refreshes when `useJudge` reports a created or judged submission, and re-reads every 2 s while a row is queued or judging. lint, typecheck and build pass; not seen in a browser.

### State at this point
- Merged into `phase/8-web-run-submit`: units 2 to 6. Unit 1 (`feat/8-api-run-endpoint`, 3 commits) is committed but NOT merged: it needs the dev host for the runner and judge tests, `make fmt lint` and `make test-adversarial`. Until it merges, the web Run button calls `/runs`, which the phase branch does not yet have.
- Still to do: unit 1 verification and merge, apply migration 00004, unit 7 (real-browser end-to-end in both themes, hidden-data check on every Submit response, lint/test/Trivy), ADR, FLOW.md, report, summary, review.

### Unit 1: Run endpoint (`feat/8-api-run-endpoint`, merged)
- Claude. Commits: engine `RunCustom` with a shared `runProgram` and `MaxInputBytes` 8 KiB (c41a633); queue `Job.Kind/Custom/Input` and `queue/run.go` (`SetRun/GetRun`, 10 min TTL) (bf14d7e); API `POST /runs`, `GET /runs/:id`, `store.TestSetVersion`, ingest turns a dead-lettered run into an IE run result, runner `processRun` (a1112fd); lint fix for `noctx` in new tests; `RunCustom` tests; ADR 0016 and the FLOW.md Phase 8 section (written by a subagent, checked by Claude against the code).
- Design: run ids are `run-<uuid>` so they never parse as submission ids; a run publishes to its own Redis key, never the results stream; the runner filters the problem to sample tests before judging and sets `Detail`, so hidden tests cannot reach a Run result (test `TestProcessRunSamplesOnly`).
- Dev host: it was unreachable at first (ssh timed out), then reachable again. Synced with `git bundle create ... feat/8-api-run-endpoint main`, `scp` to `~/p8.bundle`, `git checkout --detach`, `git fetch ~/p8.bundle feat/8-api-run-endpoint:refs/heads/feat/8-api-run-endpoint -f`, `git checkout feat/8-api-run-endpoint` (host HEAD eec5f44). `golangci-lint` is not on the non-login PATH: use `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin`.
- Merging the phase branch into the unit branch conflicted in `docs/PROGRESS.md` and this log (both edited on both branches); resolved by taking the phase branch versions and re-adding this entry.
- Verified on the host: `make fmt lint` found `noctx` in 3 new tests (the first run), fixed, then 0 issues in all 5 modules with the tree unchanged; `make test` all ok (DB-backed tests skip without `DATABASE_URL`; they pass on Windows with `.env`); `make test-sandbox` all packages ok in the real sandbox including `TestRunCustom` (echo, crash, compile error) and `TestRunCustomErrors`; `make test-adversarial` EXIT=0 with 49 top-level PASS lines and no FAIL; `trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1 --skip-dirs node_modules,bin .` with Trivy 0.75.0: 0 vulnerabilities in the 5 go.mod files and web/package-lock.json, no secrets or misconfigurations; the vulnerability DB date was not captured; no `.trivyignore`.
- Mistakes: a long foreground `make test-sandbox` over ssh hit the tool timeout and the connection dropped (exit 255); long host jobs now run detached with `setsid nohup` and a log file under `~/`, polled afterwards.

### Unit 7: end-to-end check (`test/8-e2e`)
- Migration: Claude ran `make migrate-up` on the dev host (host `.env`); `make migrate-status` before showed 00004 pending, after showed `00004_submission_client.sql` applied (goose version 4). This wrote to the real Neon database; it is additive and `make migrate-down` reverses it.
- Stack: Claude synced the phase branch to the host (bundle, as in unit 1), `make build-runner`, started `bin/runner` there as root (`sudo -E`, host `.env` loaded, `LEETFORCE_S3_ENDPOINT` blank, `LEETFORCE_PROBLEMS_DIR=~/Leetforce/problems`, `LEETFORCE_RUNNER_ID=e2e-p8`, detached with `setsid nohup`, log `~/runner.log`). Locally: `go -C api run ./cmd/api` with `.env` loaded and the same blank S3 endpoint, and `npm run dev` in `web/`. API `/readyz` showed database and redis ok.
- Scripted check (`scripts/phase8-e2e.py`, writes 3 submissions to the real DB): Run on samples AC with 2 cases; Run with a program that gets the samples wrong returns input, expected, actual and stderr for the two failing samples only; Run on custom input returns `OK` with stdout; Run with a C++ compile error returns compiler output; Submit AC (5/5) and WA (2/5) responses contain none of `expected`, `input`, `stderr`, `actual`, `source`, `test_set_version` and none of the test files' text; the SSE stream sent `queued`, `judging`, `verdict`; the Submissions list returned this client's 3 submissions newest first and an empty list for another client id or none. All PASS after one of my own test errors: the first run reported "run WA has detail" failing because `wa.py` passes both samples and only fails hidden test 03, so Run correctly said AC; the check was changed to a program that fails the samples (re-verified by an inline run; the saved script was edited afterwards and not re-run in full).
- Browser (Chrome extension, `localhost:3000/problems/sample-sum`, 1249 px window): in dark mode Ctrl+Enter ran the samples (Accepted, 23 ms, 4.5 MB, Sample 01 and 02 Accepted); the Submit button gave Accepted, 45 ms, 10.4 MB, 5 / 5 test cases passed; in light mode the Submissions tab showed the new row, and Custom input `4 10 20 30 40` ran to "Ran Successfully" with output 100. Verdict text is the largest text and always carries its label.
- Not checked in a browser: a failing Run in the UI (covered by the API check), keyboard Ctrl+Shift+Enter, narrow screens, the SSE-to-polling fallback path, and the queued and judging states visually (the SSE stream carried them).
- Cleanup: tab closed; API and web dev server stopped (ports 8080 and 3000 freed); the host runner stopped gracefully (`runner stopped` in the log). The dev host is still up and billable.
- Extra test added: `store.TestTestSetVersion` (`api/internal/store/problems_test.go`), run against the real database with `.env` loaded: PASS, with `TestListSubmissions`.
- Final checks on the merged tree: `npm run lint`, `tsc --noEmit`, `prettier --check` clean; `go vet` and `go test` in `api/` pass.

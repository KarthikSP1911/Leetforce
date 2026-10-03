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

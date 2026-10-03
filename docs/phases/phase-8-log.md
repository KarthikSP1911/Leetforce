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

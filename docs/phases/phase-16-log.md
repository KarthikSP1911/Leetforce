# Phase 16 log

Running log for Phase 16 (launch readiness). Entries are appended per unit of work.

## Load test

Branch `feat/16-load-test` (off `phase/16-launch-readiness`). Done by Claude, in a worktree on the Windows machine; the real stack was not run (needs the dev host).

### What was added
- `tools/loadtest/` (`main.go`, `load.go`, `report.go`, `load_test.go`, `go.mod`): stdlib-only Go load tester. See `docs/adr/0023-load-test-tool.md`.
- `go.work`: added `./tools/loadtest`. `Makefile`: `tools/loadtest` added to `GO_MODULES`, new `loadtest` target.
- `docs/adr/0023-load-test-tool.md`.

### Routes used (verified in `api/internal/server/server.go`)
`POST /auth/signup` (201), `GET /problems`, `POST /runs` (202) and `GET /runs/:id` (`status` becomes `done`), `POST /submissions` (202), `GET /submissions/:id/events` (SSE, `event: verdict` ends it). API listens on `:8080` by default (`LEETFORCE_API_ADDR`). Contest endpoint: no contest routes were committed on `phase/14-contests` when checked, so `POST /contests/<slug>/submissions` is an assumption; override with `-contest-path`. A 404 aborts with an error.

### How to run
```bash
make loadtest ARGS="-users 20 -duration 2m -ramp 20s -json out.json"
LEETFORCE_LOADTEST_BASE_URL=https://host make loadtest ARGS="-mode contest -contest <slug> -users 50 -iterations 3"
```
Flags: `-users`, `-iterations`, `-duration`, `-ramp`, `-mode mixed|contest`, `-contest`, `-contest-path`, `-problem`, `-language`, `-source-file`, `-password`, `-verdict-timeout`, `-json`, `-base-url`.
Sign-up is rate limited per IP (20 per 10 min, ADR 0017): set `LEETFORCE_LIMIT_*` higher on the test deployment for more than 20 users per run from one IP.

### Cleanup of test users (do not run against the real DB without checking the count first)
Accounts are `lfload_<runid>_<n>` with email `...@loadtest.invalid`. Check, then delete (adjust FK handling to the schema in `api/migrations`; delete dependent rows first if no `ON DELETE CASCADE`):
```sql
SELECT count(*) FROM users WHERE email LIKE '%@loadtest.invalid';
-- submissions by test users (only if user_id has no cascade):
DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@loadtest.invalid');
DELETE FROM users WHERE email LIKE '%@loadtest.invalid';  -- sessions cascade or delete them first
```
Not executed anywhere.

### Commands and results
- `cd tools/loadtest && go vet ./... && go test -count=1 ./...` -> `ok leetforce/tools/loadtest` (table-driven tests against an httptest fake: mixed flow, 429 counted apart from errors, contest mode, contest 404 abort, config errors, percentiles, JSON/text report).
- `golangci-lint fmt ./...` and `golangci-lint run ./...` in `tools/loadtest` -> 0 issues. Two findings fixed on the way: gosec G304 in the test (`filepath.Clean`) and staticcheck QF1002 (tagged switch).
- Mistake: first attempt to write the files with one long shell command was refused by the sandbox; files were written individually instead (no effect on the result).
- Full `make fmt lint` was not run across the other modules on this machine; only the new module was linted.

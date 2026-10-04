# Phase 14 log: Contests

Running log. Secrets and public IPs are never written here.

Owner overrides for this session (parallel with Phases 15 and 16): skip the recap question, do not wait for plan approval, skip the review Q&A, do not run test suites or the sandbox, e2e or adversarial gates (they run once in the Phase 16 session). Only gofmt, go build, go vet, tsc and lint ran. Nothing billable ran.

## File and path index
| Path | What |
|---|---|
| `api/migrations/00006_contests.sql` | contests, contest_problems, contest_participants, submissions.contest_id |
| `api/internal/contest/` | model, scoring, Postgres store, handlers, tests |
| `api/internal/server/contests.go` | `ContestService` interface and the hidden-problem and submit gates |
| `api/cmd/contestscore/` | prints standings JSON for one contest |
| `web/src/app/contest/`, `web/src/components/contest/`, `web/src/types/contest.ts` | contest pages |
| `scripts/seed-mock-contest.sh`, `scripts/run-mock-contest.sh`, `scripts/mock-contest-e2e.py` | mock contest (written, not run) |
| `docs/phases/phase-14-contract.md` | contract for Phase 15, with the `Event.Elapsed` deviation |
| `docs/adr/0023-contest-model-and-scoring.md` | ADR |
| `C:\Users\karth\Downloads\Leetforce-p14` | git worktree on `phase/14-contests` (created by Claude) |

## Session 1

1. **Claude** found the session started on `main` in the main checkout, with no `phase/14-contests` branch. Ran `git worktree add -b phase/14-contests ../Leetforce-p14 main` and `git tag phase-14-start`. Result: worktree at `62e6926`.
2. **Claude** spawned one subagent for the web pages (only `web/`), wrote the migration, contest package, server wiring, scripts and docs itself.
3. Migration number: the next free goose number was `00006` (`00005_users_sessions.sql` was the last).
4. Mistakes and fixes:
   - A first bulk shell heredoc failed on a quoting error and wrote nothing; the files were rewritten with the file tool. The `Store.Pool()` append was lost with it, which `go build` caught (`db.Pool undefined`); fixed.
   - The worktree had no `node_modules`, so the husky commit hooks (`commitlint`, `lint-staged`) failed. Claude made two directory junctions to the main checkout's installs: `node_modules` and `web/node_modules` (git-ignored; remove with `rmdir`, which removes only the link). Hooks were not bypassed.
   - commitlint rejected body lines over 72 characters and a 74-character header; messages were shortened and recommitted. The earlier failed attempts produced no commits.
5. Subagent reported `npx tsc --noEmit` has one error, `layout.tsx: Cannot find name 'LayoutProps'` (a type Next generates at build time; not touched, not verified). `npm run lint` passed.
6. Ran: `gofmt -l`, `go build ./...`, `go vet ./...` in `api/` (clean) and `go test ./internal/contest/` only (scoring and handler tests, pure, pass). Not run: `make fmt lint` (golangci-lint), the other test suites, `make migrate-up`, Trivy (`scripts/scan-staged.sh` needs the dev host), any e2e. **Migration 00006 has not been applied to any database.**
7. Commits: units merged with `--no-ff` into the phase branch: `feat/14-contest-api`, `feat/14-contest-web`, `test/14-mock-contest`.
8. Not done this phase: the event stream and `GET /submissions/:id` do not check contest visibility (the id is a random uuid); a standings API is Phase 15.

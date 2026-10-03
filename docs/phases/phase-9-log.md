# Phase 9 log: Auth and limits

Running log (CLAUDE.md "Documenting every step"). Entries: who, command, result, mistakes.

## Session 1 (shared with Phase 10)

### Start of session
- Claude read CLAUDE.md, `docs/PROGRESS.md`, `docs/phases/phase-8-summary.md` and the Phase 9 and 10 sections of `docs/PLAN.md`. Repo matched: on `main` at c5dd1c1, tags `phase-0..8-*` present, untracked only `.claude/`, `web/AGENTS.md`, `web/CLAUDE.md`. One doc mismatch: the PROGRESS table row for Phase 8 still said "in review" while the header said done (fixed in the closing docs commit).
- The owner asked for Phase 9 **and** Phase 10 in one session, faster, with all tests run once at the end after both phases, and then asked for subagents. This overrides two CLAUDE.md rules (one phase per session, test before every commit); see "Deviations" below. The recap question was asked and not answered; the plan defaults were accepted with "go": email + password, bcrypt, opaque session cookie stored in Postgres; limits Submit 10/min per user and 30/min per IP, Run 20/min per user and 60/min per IP; review Q&A skipped; dev host left running.
- `git checkout -b phase/9-auth-limits`, `git tag phase-9-start`.

### Deviations from CLAUDE.md (owner's explicit instructions, recorded so the review has no surprises)
- Two phases in one session. The review Q&A for both phases was skipped.
- Per-commit tests were replaced by compile checks (`go build`, `go vet`, `tsc`) while writing, and one full test run on the dev host at the end of Phase 10. A commit in between may therefore not have passed its tests when it was made.
- Units were merged into three per phase and done directly on the phase branch (the Go backend) or on one feature branch per agent (`feat/9-web-auth`, `feat/10-problem-pipeline`), not one branch per small unit.
- Two subagents worked in git worktrees under `.claude/worktrees/` (git-ignored by the owner's untracked `.claude/` entry): one wrote the Phase 9 web side, one the whole of Phase 10. Their branches were merged with `--no-ff` and the worktrees removed.

### Backend (Claude, directly on `phase/9-auth-limits`)
- `api/migrations/00005_users_sessions.sql`: `users` (unique `lower(email)`, `lower(username)`), `sessions` (`token_hash bytea` primary key, expiry), `submissions.user_id` (+ partial index). NOT applied by hand: applied by the gate script on the dev host (see "Test run").
- `queue/limit.go`: `Queue.Allow`, a fixed-window counter in Redis (`INCR`, then `PEXPIRE` when the key is new or has no TTL), keys `<prefix>:rl:<scope>:<key>`.
- `api/internal/store/users.go`: `CreateUser` (unique violation 23505 becomes `ErrConflict`), `UserByLogin`, session create/lookup/delete, `DeleteExpiredSessions`, `SolvedProblems`. `store/submissions.go`: `SetSubmissionClient`/`ListSubmissions` replaced by `SetSubmissionUser`/`ListUserSubmissions`.
- `api/internal/server/auth.go`: signup, login, logout, `GET /me`, cookie `lf_session` (HttpOnly, SameSite=Lax, Secure on HTTPS), bcrypt cost 12, 8-72 byte passwords, dummy-hash compare for unknown accounts. `limits.go`: `Limiter` interface, `Limits` with defaults, 429 + `Retry-After`, fail closed (503) if Redis errors. `submissions.go`, `runs.go`: login required, per-user and per-IP limits run before the body is read. `problems.go`: list items carry `solved`. `server.go`: routes, `TrustedProxies` (gin `SetTrustedProxies`; default none so `X-Forwarded-For` cannot be forged).
- `api/cmd/api/main.go`: `LEETFORCE_TRUSTED_PROXIES`, `LEETFORCE_LIMIT_*` env vars (documented in the file header), daily `sweepSessions` goroutine.
- Dependency: `golang.org/x/crypto v0.57.0` added to `api/go.mod` (`go get`; it was already in the module cache).
- Tests: `api/internal/server/auth_test.go` (signup/login/logout, validation table, identical 401s, auth and submit/run limits, forged `X-Forwarded-For`, solved list, 401 without login), `store/users_test.go`, `store/submissions_test.go` (`TestListUserSubmissions` now also checks `SolvedProblems`). Existing server test helpers sign in through a `fakeAccounts` with a session cookie.
- Mistakes: my first batch edit of the server tests failed to parse in the shell (nested heredocs); nothing had been applied, so I redid it with the file tools and a script file. A `//nolint` comment I inserted by script landed in the middle of an `if` condition and broke the syntax; fixed by splitting out a variable (caught by `gofmt -l`).
- Commits: afb6a0a (users, sessions, `queue.Allow`), c73962e (store by user), 054c9c4 (accounts, sessions, limits in the API), 4403c1b (e2e scripts + ADR 0017), 951f601 (lint explanation, on the Phase 10 branch).
- Existing e2e scripts needed a login: `scripts/test-api-e2e.sh` and `scripts/test-live-e2e.sh` now sign up a throwaway account (`ensure_login`, cookie jar `$WORK/cookies`) and delete the account and its submissions on exit; `scripts/phase8-e2e.py` was renamed (`git mv`) to `scripts/phase9-e2e.py` and extended with the account, 401, limit and solved checks.

### Web (subagent, branch `feat/9-web-auth`, merged as 9b15f67)
- Commits 172da68 (client calls + `AuthProvider`), 3712085 (login/signup pages + `UserMenu`), 3166c95 (solved column, sign-in prompts). The anonymous client id and header were removed from `web/src/lib/api/client.ts`; `ApiError` carries `retryAfter`; the Problems page forwards only the `lf_session` cookie on its server-side fetches; `?next=` redirects accept only same-origin paths; `ResultPanel` shows "Sign in to run or submit code." on 401. The subagent ran `npm ci`, `npm run lint`, `typecheck` and `build` in its worktree.
- Claude re-ran `npm run lint && npm run typecheck && npm run build` in `web/` on the merged tree on Windows: pass.
- NOT verified in a browser (login form, user menu, solved check mark, light and dark themes, narrow screens).

### Host and file index
- Files created or changed outside the repo: none yet in this phase except the dev-host steps in the Phase 10 log's "Test run".
- Paths: `docs/adr/0017-accounts-sessions-and-limits.md`, `docs/phases/phase-9-log.md` (this file), `scripts/phase9-e2e.py`.

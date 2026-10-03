# Phase 9: Auth and limits

**Branch:** `phase/9-auth-limits`
**Range:** `phase-9-start..phase-9-done`
**Dates:** 2026-10-03 (one session shared with Phase 10; see [phase-9-log.md](phase-9-log.md))
**Milestone:** M3 (a usable product on one machine)

## Summary
LeetForce now has real users. People sign up and log in with an email or username and a password, a session cookie identifies them, Run and Submit need a login, and both are rate limited per user and per IP with counters in Redis. The problem list shows which problems a user has solved, and the Submissions tab lists the user's own attempts instead of an anonymous browser's. This is the abuse protection that the earlier phases left open (`POST /runs` had no limit).

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Limits enforced and tested | ✅ | Unit: `TestSubmitRunLimits`, `TestAuthRateLimits`, `TestForwardedForIsIgnoredUnlessTrusted` (`api/internal/server/auth_test.go`) and `TestAllowCountsPerKeyAndWindow`, `TestAllowRepairsCounterWithoutExpiry` (`queue/limit_test.go`, real Redis). End to end on the dev host: `make test-auth-e2e` printed `PASS run limit sends Retry-After 29`, `PASS run limit trips (default 20/min per user)` (17th request 429) and `PASS repeated failed logins get 429 after real 401s` |
| A usable product on one machine | ✅ for the backend and API flow; ⚠️ the new pages are not checked in a browser | `make test-auth-e2e`: signup, logout ends the session, login by username, Run and Submit as a signed-in user, SSE to a verdict, the user's own submission list, `sample-sum` solved for that user only. `npm run lint`, `typecheck`, `build` pass in `web/`. Login and signup pages, user menu and the solved mark were not looked at in a browser |
| Anonymous Run and Submit refused | ✅ | `TestSubmitAndRunRequireLogin`; e2e `PASS anonymous run is refused 401`, `PASS anonymous submit is refused 401` |
| No hidden data in any response (carried over) | ✅ | e2e `PASS submit AC response has no test data`, `PASS submit WA response has no test data`; `make test-live-e2e` PASS |

All tests were run once at the end of Phase 10 on the final branch tip, as the owner asked; see [phase-10-log.md](phase-10-log.md) "Test run".

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/9-web-auth` | Login and signup pages, auth client, user menu, solved column, sign-in prompts (written by a subagent) | 3 |

The backend (migration, store, API, limiter, e2e scripts, ADR) was committed directly on `phase/9-auth-limits` by Claude. Four small commits that belong to Phase 9 (found by the end-of-session gates) landed on `phase/10-problem-pipeline`: 951f601 (lint explanation for the cookie), 1548dc5 (`queue/limit_test.go`, `scripts/test-auth-e2e.sh`, `make test-auth-e2e`), 3273eaf (execute bit on the new script), 3a51b95 (`.PHONY` repair). They appear in the Phase 10 report's file list.

## File-by-file changes
Generated with `git diff --name-status phase-9-start..HEAD` on `phase/9-auth-limits` (documents added after the last code commit are listed at the end).

### Added
| File | Purpose |
|---|---|
| `api/migrations/00005_users_sessions.sql` | `users` (unique `lower(email)` and `lower(username)`), `sessions` (hashed token, expiry), `submissions.user_id` and a partial index |
| `queue/limit.go` | `Queue.Allow`: fixed-window counter in Redis (`INCR` + `PEXPIRE`), shared by all API instances |
| `api/internal/store/users.go` | Create user (unique violation becomes `ErrConflict`), find by email or username, create/lookup/delete sessions, sweep expired, `SolvedProblems` |
| `api/internal/store/users_test.go` | Case-insensitive uniqueness, login lookup, session lifecycle and expiry |
| `api/internal/server/auth.go` | Signup, login, logout, `GET /me`; bcrypt; cookie `lf_session`; dummy-hash compare |
| `api/internal/server/auth_test.go` | Account flow, validation table, identical 401s, auth and submit/run limits, forged `X-Forwarded-For`, solved list, login required |
| `api/internal/server/limits.go` | `Limiter` interface, `Limits` with defaults, 429 + `Retry-After`, fail closed on limiter error |
| `docs/adr/0017-accounts-sessions-and-limits.md` | Decision record for accounts, sessions and limits |
| `scripts/phase9-e2e.py` | Phase 9 exit checks against a live API and runner (renamed from `phase8-e2e.py`, extended) |
| `web/src/types/auth.ts` | `User` type |
| `web/src/components/auth/AuthProvider.tsx` | Client context with the current user, `refresh`, `signOut`, `useUser` |
| `web/src/components/auth/AuthForm.tsx` | Shared login and signup form |
| `web/src/app/login/page.tsx`, `web/src/app/signup/page.tsx` | The two pages |
| `web/src/components/layout/UserMenu.tsx` | Sign in link or user menu with Sign out (Escape and outside click close it) |
| `docs/phases/phase-9.md`, `phase-9-log.md`, `phase-9-summary.md` | This report, the running log, the explainer |

### Modified
| File | What changed | Why |
|---|---|---|
| `api/cmd/api/main.go` | `LEETFORCE_TRUSTED_PROXIES`, `LEETFORCE_LIMIT_*`, `Accounts`/`Limiter`/`Limits` passed to the router, daily `sweepSessions` | Configure and wire auth and limits |
| `api/internal/server/server.go` | `Accounts`, `Limiter`, `Limits`, `TrustedProxies` in `Deps`; four auth routes; `SetTrustedProxies` | `X-Forwarded-For` is believed only from configured proxies |
| `api/internal/server/submissions.go`, `runs.go` | Login required, limits before the body is read; tag the submission with `user_id`; list by user; client id removed | Abuse protection and per-user history |
| `api/internal/server/problems.go` | List items are `problemItem` with `solved` | Solved status |
| `api/internal/server/submissions_test.go`, `runs_test.go` | Fakes use users and a session cookie; list and tag tests rewritten | Follow the new rules |
| `api/internal/store/submissions.go`, `submissions_test.go` | `SetSubmissionUser`, `ListUserSubmissions` replace the client-id versions; test also checks `SolvedProblems` | Per-user history |
| `scripts/test-api-e2e.sh`, `scripts/test-live-e2e.sh` | Sign up a throwaway account (`ensure_login`, cookie jar) and delete it on exit | Submit now needs a login |
| `web/src/lib/api/client.ts` | Auth calls; client id removed; `ApiError.retryAfter`; optional cookie for server-side fetches; 204 handling | Browser and server-side calls use the session |
| `web/src/types/problem.ts` | `solved?: boolean` | Solved status |
| `web/src/app/layout.tsx` | Mounts `AuthProvider` | Auth state for all pages |
| `web/src/app/problems/page.tsx` | Forwards only the `lf_session` cookie to the API | Server-rendered list can show solved marks |
| `web/src/components/layout/Navbar.tsx` | Static Sign in link replaced by `UserMenu` (Navbar stays a server component) | Show the user |
| `web/src/components/problems/ProblemTable.tsx` | Check mark with label "Solved" in the Status column | Solved status |
| `web/src/hooks/useJudge.ts`, `components/workspace/ResultPanel.tsx` | 401 shows "Sign in to run or submit code." with a link; 429 shows retry seconds | Clear errors for the new rules |
| `web/src/components/workspace/SubmissionsTab.tsx` | Sign-in prompt when anonymous; refetch when the user changes | Per-user history |

### Deleted
| File | Reason |
|---|---|
| `scripts/phase8-e2e.py` | Renamed to `scripts/phase9-e2e.py` (git shows delete + add) |

### Renamed / moved
| From | To | Reason |
|---|---|---|
| `scripts/phase8-e2e.py` | `scripts/phase9-e2e.py` | The Phase 8 script cannot run any more (Submit needs a login); the new one covers its checks plus Phase 9's |

## Key code changes
1. **Sessions store only a hash.** `newToken` makes 32 random bytes; the cookie holds the token and `sessions.token_hash` holds its SHA-256 (`api/internal/server/auth.go`, `store/users.go`). A leaked database does not leak working sessions.
2. **Limits run first and are shared.** `Deps.limit` calls `Queue.Allow` before the body is parsed, so a flood of bad requests costs one Redis round trip each. Keys are `<scope>-user:<id>` and `<scope>-ip:<ip>`; over the limit the answer is 429 with `Retry-After`.
   ```go
   if !d.limitUserAndIP(c, "submit", user.ID, lim.SubmitUser, lim.SubmitIP, lim.SubmitWindow) { return }
   ```
3. **The client IP cannot be forged.** `gin.SetTrustedProxies(d.TrustedProxies)` with an empty default means `X-Forwarded-For` is ignored unless the peer is a listed proxy (`TestForwardedForIsIgnoredUnlessTrusted`).
4. **No account enumeration.** A login for an unknown account still runs a bcrypt compare against a dummy hash and returns the same 401 body as a wrong password (`TestLoginRejects`).

## Decisions
- [ADR 0017](../adr/0017-accounts-sessions-and-limits.md): email/password accounts, opaque server-side sessions (SHA-256 of the token stored), bcrypt cost 12, Redis fixed-window limits, trusted-proxy rule for client IPs.

## Tests
- New: `api/internal/server/auth_test.go`, `api/internal/store/users_test.go`, `TestListUserSubmissions`/`SolvedProblems` in `store/submissions_test.go`, `queue/limit_test.go` (on the Phase 10 branch), `scripts/phase9-e2e.py` via `make test-auth-e2e`.
- Run: `make test` (needs `DATABASE_URL` for the store tests; Redis tests need `LEETFORCE_TEST_REDIS_URL`), `make test-auth-e2e` (dev host, needs psql).

## Known issues and deferred work
- The login and signup pages, user menu, solved mark and sign-in prompts were not looked at in a browser, in either theme, or at narrow widths.
- Trivy was not run in this session (the owner asked for speed): `scripts/scan-staged.sh` and the pre-merge scan are still to do. A new dependency, `golang.org/x/crypto v0.57.0`, was added to `api/go.mod`.
- No email verification, password reset or OAuth (ADR 0017); deferred, no phase assigned.
- Old submissions keep their `client_id` and have no user; they are not migrated.
- Behind the local Next.js proxy, set `LEETFORCE_TRUSTED_PROXIES=127.0.0.1` or every browser shares one IP for the per-IP limits. The web app does not set this for the owner.
- `POST /runs` and `POST /submissions` share a runner pool; limits bound abuse but there is still no queue priority (Phase 13 scaling).
- Migration 00005 was applied to the real Neon database by `make migrate-up` on the dev host (reversible with `make migrate-down`).

## Stats
Generated at the end of the session; see the final lines of [phase-10.md](phase-10.md) for the combined numbers. For `phase-9-start..phase/9-auth-limits` before these documents: 36 files changed, 1889 insertions, 266 deletions; commits excluding merges: 7.

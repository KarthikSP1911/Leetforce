# Phase 9 summary: Auth and limits

## TL;DR
- LeetForce now has accounts: you sign up, log in, and a cookie tells the API who you are. Run and Submit refuse anonymous callers.
- Every Run, Submit, sign-up and login is counted in Redis and refused with "429 Too Many Requests" when someone goes over the limit, per user and per IP address, so nobody can flood the runners.
- The problem list shows a check mark on problems you solved, and the Submissions tab shows your own attempts. This completes milestone M3: a usable product on one machine.

## Where this phase fits
```
 browser --> [P9 NEW: login, session cookie] --> API --> [P9 NEW: limits in Redis] --> Postgres + queue
 [P7-8 UI]   [P9 NEW: users, sessions tables]    [P4]                                  [P4, P3]
                                                                                          |
 browser <-- SSE / poll (P5, P8) <-- API ingest (P4) <-- results stream <-- runner <-- sandbox (P1, P2, P6)

 Still to come: problem pipeline (P10), metrics (P11), infra as code (P12), cloud (P13), contests (P14-15)
```
- Phase 9 depends on Phase 8 (the UI that submits) and Phase 4 (Postgres, Redis through the queue module).
- It unblocks Phase 10 (rejudging needs submissions owned by users) and Phase 14 (contests need users).

## What I built and why
### Accounts and sessions (backend, on `phase/9-auth-limits`)
- **What:** `POST /auth/signup`, `/auth/login`, `/auth/logout` and `GET /me`; tables `users` and `sessions`.
- **Why:** limits per user need a user, and "my submissions" and "solved" need to belong to someone. Before this, a browser id in `localStorage` was the only identity and it was not a credential.
- **How it works:** the password is hashed with bcrypt (`api/internal/server/auth.go`). On login the server creates a random token, sends it in an HttpOnly cookie and stores only its SHA-256 in `sessions`. Each request looks the hash up (`store/users.go`). Logout deletes the row.
- **Alternatives considered:** JWT (harder to revoke), argon2id (stronger but more tuning), email verification and password reset (no mail service yet). See [ADR 0017](../adr/0017-accounts-sessions-and-limits.md).

### Rate limits (`api/internal/server/limits.go`, `queue/limit.go`)
- **What:** Submit 10/min per user and 30/min per IP; Run 20/min and 60/min; sign-up and login 20 per 10 minutes per IP, and 10 login attempts per 10 minutes per account name.
- **Why:** each judged program costs a sandboxed process; without limits one person can starve everyone else, and a password can be guessed by trying many times.
- **How it works:** `Queue.Allow` does `INCR` on a Redis key and sets an expiry the first time; if the count is over the limit the API answers 429 with a `Retry-After` header. The check runs before the request body is read. If Redis is unreachable the request is refused, since a submission needs Redis anyway.
- **Alternatives considered:** sliding window or token bucket (smoother, more Redis work); limiting in a reverse proxy (not built yet). A burst across a window edge can reach twice the limit; accepted.

### Trusted proxies
- **What:** `X-Forwarded-For` is ignored unless the request comes from an address in `LEETFORCE_TRUSTED_PROXIES`.
- **Why:** otherwise anyone could send a fake header and get a fresh per-IP limit each time.
- **How it works:** gin's `SetTrustedProxies`. Behind the local Next.js dev proxy, set it to `127.0.0.1`, or all users appear to share one IP.

### Solved status and the web UI (`feat/9-web-auth`, written by a subagent)
- **What:** `/login` and `/signup` pages, a user menu in the navbar, a check mark in the problem list, and clear messages for "sign in" (401) and "wait N seconds" (429).
- **Why:** the backend rules would otherwise just produce error toasts.
- **How it works:** `AuthProvider` asks `/me` once and shares the user; the server-rendered Problems page forwards only the `lf_session` cookie so the list can include `solved`. After login the page honours `?next=` only for same-site paths.
- **Not checked:** I did not open these pages in a browser.

## How it works now, step by step
1. You open `/signup`, enter email, username, password. The browser posts to `/api/auth/signup` (the Next.js proxy forwards it to the Go API).
2. The API checks the input, hashes the password, inserts the user and sets the `lf_session` cookie.
3. You press Submit. The browser sends the cookie. The API finds your session, then asks Redis whether you are under 10 submits per minute (and your IP under 30). If not, it answers 429.
4. If allowed, the submission is stored with your `user_id`, queued, judged and shown exactly as in Phase 8.
5. When the verdict is AC, the problem list shows a check mark for you (and nobody else).

## Key concepts
- **Session cookie:** a small value the browser sends with every request so the server knows who you are. `HttpOnly` keeps JavaScript from reading it; `SameSite=Lax` stops other sites from making your browser submit for you.
- **bcrypt:** a deliberately slow password hash; slow means guessing is expensive. Limited to 72 bytes, so longer passwords are refused.
- **Rate limit (fixed window):** count requests per key per time window; over the count means 429. LeetForce needs it so judging capacity is shared fairly.
- **Trusted proxy:** a server whose forwarded-IP header we believe. Needed so IP limits cannot be dodged by forging a header.
- **Account enumeration:** learning which accounts exist from different error messages or timings. Avoided by identical 401 answers and a dummy hash comparison.

## Try it yourself
```bash
# repo root, .env supplies DATABASE_URL and LEETFORCE_REDIS_URL; on the dev host
make migrate-up          # applies migration 00005 (already applied to Neon in this session)
make test-auth-e2e       # starts the API and a runner, runs scripts/phase9-e2e.py, deletes its test account
```
Expect lines `PASS anonymous run is refused 401`, `PASS signup`, `PASS logout ends the session`, `PASS run limit sends Retry-After`, `PASS repeated failed logins get 429 after real 401s`, then `ALL PASS`. For the web UI: run the API with `LEETFORCE_TRUSTED_PROXIES=127.0.0.1`, `cd web && npm run dev`, open http://localhost:3000/signup.

## Trade-offs and risks
- Accounts are email and password only: no reset, no verification. A forgotten password cannot be recovered.
- The limiter depends on Redis; if it fails, submissions are refused (503) rather than unlimited.
- Many users behind one NAT share a per-IP limit; raise `LEETFORCE_LIMIT_AUTH_IP` if that hurts.
- bcrypt at cost 12 takes about a quarter of a second of CPU per sign-up/login; the per-IP auth limit bounds that load.
- The new web pages are unverified in a browser.

## Review questions
The owner asked for Phases 9 and 10 in one session without stops; the review was skipped. The questions I would have asked are below, unanswered.

Understanding:
1. Why does the database store the SHA-256 of the session token and not the token itself?
2. Why does the login for a non-existent account still run a bcrypt comparison?
3. What would happen to the per-IP limits if `X-Forwarded-For` were trusted from anyone?
4. Why does the limit check run before the request body is parsed?
5. What does a user see if Redis is down, and why was "refuse" chosen over "allow"?

Decisions for the owner:
- A. Keep the default limit numbers, or change them?
- B. Add email verification and password reset before any public use?

## Review Q&A
Skipped by the owner's instruction (two phases in one session, no stops). Nothing was answered on the owner's behalf. Decisions stay on Claude's defaults: A keep the numbers in ADR 0017; B no verification or reset until a mail service exists.

## Open decisions
- Limit numbers and whether sign-up needs email verification (before public launch, Phase 16 at the latest).
- Carried over from `docs/PROGRESS.md`: Phase 8, 7, 6, 5, 4 and 2 review answers and decisions; Neon plan and limits unchecked; `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Handoff
- **State:** branch `phase/9-auth-limits`, tags `phase-9-start`, `phase-9-done`, `M3` once merged to `main`; the dev host is still up and billable; migration 00005 is applied to Neon.
- **Next phase:** 10 - Problem pipeline (done in the same session, see [phase-10-summary.md](phase-10-summary.md)).
- **Next session prompt:** see the Phase 10 summary.

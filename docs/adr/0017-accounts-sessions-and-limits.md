# 0017. Email/password accounts, opaque server-side sessions, Redis rate limits

**Status:** Accepted (Phase 9). The owner accepted the session plan defaults ("go"): bcrypt, opaque session cookie stored in Postgres, and the limit numbers below.

## Context
Until Phase 8 anyone could Submit and Run, and "my submissions" was keyed by an anonymous browser id that is not a credential (ADR 0016). Judging costs a sandboxed process, so an anonymous caller can burn runner capacity. Phase 9 needs real users, abuse protection per user and per IP, and a "solved" mark per user.

## Decision
- **Accounts:** `users` (email, username, bcrypt hash; both unique case-insensitively through `lower()` indexes). Sign-up needs a valid email, a 3-32 character username (`[A-Za-z0-9_-]`) and a password of 8-72 bytes (bcrypt ignores input past 72 bytes, so longer ones are refused, not truncated). Cost factor 12.
- **Login by email or username.** A login for an unknown account compares against a dummy hash so it costs the same as a wrong password, and both answer the same 401 text, so accounts cannot be enumerated by timing or message.
- **Sessions:** a 32-byte random token in the `lf_session` cookie (HttpOnly, SameSite=Lax, Secure when the request is HTTPS or `X-Forwarded-Proto: https`, 30 days). Only the SHA-256 of the token is stored (`sessions.token_hash`), so a database leak does not yield usable sessions. Logout deletes the row. Expired rows are ignored at lookup and swept daily (`sweepSessions` in `api/cmd/api/main.go`).
- **CSRF:** all state-changing endpoints take JSON bodies and the cookie is SameSite=Lax, so a cross-site form post does not carry the session. No separate CSRF token for now.
- **Login required** for `POST /submissions` and `POST /runs` (401 otherwise). Reads of problems, a submission by id (an unguessable UUID) and its event stream stay public, as before. `submissions.user_id` replaces the client id; old rows keep their `client_id` and are not migrated.
- **Rate limits** (`api/internal/server/limits.go`, counters in Redis through `queue.Allow`, a fixed window shared by every API instance):

  | Action | Per user | Per IP | Window |
  |---|---|---|---|
  | Submit | 10 | 30 | 1 min |
  | Run | 20 | 60 | 1 min |
  | Sign-up and login | 10 failed-or-not attempts per account name (login only) | 20 | 10 min |

  Over the limit the API answers 429 with `Retry-After`. All values are overridable with `LEETFORCE_LIMIT_*` (negative turns one off). The limiter runs before the body is parsed. If Redis fails the request is refused (503) rather than allowed, since a submission needs Redis anyway.
- **Client IP:** `X-Forwarded-For` is believed only from addresses in `LEETFORCE_TRUSTED_PROXIES` (default none), so the header cannot be forged to dodge the per-IP limit. Behind the local Next.js proxy set it to `127.0.0.1`; without it every browser shares the proxy's IP and the per-IP limits apply to all users together.
- **Solved status:** `GET /problems` adds `solved` per item for the signed-in user (an `AC` verdict on any of their submissions, `store.SolvedProblems`); false for anonymous callers.

## Alternatives
- **JWT:** stateless, but logout and revocation need a deny list, which is a session table again. Opaque tokens give instant revocation for one extra query per request.
- **argon2id:** stronger against GPUs; bcrypt was chosen because it is already in `golang.org/x/crypto` with no tuning of memory parameters on a small host. Revisit if the host grows.
- **Sliding-window or token-bucket limits:** smoother but more Redis work; a fixed window is enough to stop abuse and is two commands. A burst across a window boundary can reach twice the limit; accepted.
- **Email verification, password reset, OAuth:** not needed for a single-machine product; deferred (no mail service exists yet).
- **Fail-open limiter:** keeps serving when Redis is down, but there is nothing to serve then.

## Consequences
- Signing up costs about 250 ms of CPU for bcrypt at cost 12; the per-IP auth limit bounds how much of that an attacker can force.
- Sessions live in Neon, so each authenticated request does one indexed read; a suspended Neon compute adds its wake-up delay to the first request.
- Passwords cannot be recovered or reset in this phase.
- The 20-per-10-minute IP limit on sign-up and login is shared by everyone behind one NAT; raise `LEETFORCE_LIMIT_AUTH_IP` for shared networks.
- Migration 00005 touches the real Neon database; it is additive and reversible with `make migrate-down`.

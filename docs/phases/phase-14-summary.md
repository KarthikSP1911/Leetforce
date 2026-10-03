# Phase 14 summary: Contests

## TL;DR
- A contest is now a timed window over a few problems. Users register, submit during the window, and an ICPC-style score can be computed from their verdicts.
- Contest problems stay hidden until the contest starts, and during it from anyone who has not registered.
- Nothing has been run yet: migration, API, scripts and pages are untested. The gate is the Phase 16 session.

## Where this phase fits
```
browser -> API -> Redis queue -> runner -> sandbox -> verdict -> API -> browser
 built      built    built       built      built      built
 + Phase 14: /contest pages (browser), /contests + contest_id + gates (API),
             contest tables in Postgres, Score() over verdicts
 Phase 15 (parallel): standings API and leaderboard UI, reads Store.Events + Score
 Phase 16: runs the gates for 14-16
```
- Depends on Phase 9 (users and sessions) and the submission flow of Phases 4 to 8; contest submissions reuse it unchanged.
- Unblocks Phase 15, which ranks contest participants with `Score`.

## What I built and why
### API and scoring (`feat/14-contest-api`)
- **What:** migration `00006`, package `api/internal/contest`, gates in `api/internal/server`.
- **Why:** a contest needs a window, a problem set, registered participants, hidden problems and a fair score.
- **How it works:** status is computed from the clock. `POST /submissions` with `contest_id` checks the window and registration, then tags the submission. `Score` takes judged events and returns ranked standings. See the [contract](phase-14-contract.md).
- **Alternatives:** stored status, a separate submissions table, points scoring ([ADR 0023](../adr/0023-contest-model-and-scoring.md)).

### Web (`feat/14-contest-web`)
- **What:** `/contest` list and `/contest/[slug]` page with countdown, register, problem tabs and a standings placeholder.
- **Why:** users need to find, join and play a contest. The placeholder lets Phase 15 plug in.
- **How:** the countdown uses the API's `server_time` to correct the browser clock; problems open the normal workspace with `?contest=<slug>` so Submit sends `contest_id`.

### Mock contest scripts (`test/14-mock-contest`)
- **What:** seed script, driver and `make test-mock-contest`. **Why:** the exit criterion is a full mock contest. **How:** three users, A and B problems, AC/WA/CE, visibility and 409 checks, and scores from the real `Score`.

## How it works now, step by step
1. A contest row (SQL or the seed script) names problems A, B, C.
2. A user signs in, opens `/contest/mock-contest`, clicks Register (`POST /contests/:slug/register`).
3. When the contest starts, `GET /contests/:slug/problems` returns the problems to registered users (404 to others).
4. The user opens a problem, Submits; the API checks the window, registration and that the problem is in the contest, tags the submission, queues it.
5. The runner judges it as usual; the verdict is stored.
6. `Store.Events` returns judged, in-window events; `Score` ranks them.

## Key concepts
- **ICPC scoring:** rank by problems solved, then total penalty; penalty is minutes to the first AC plus 20 per earlier rejected attempt.
- **Derived status:** upcoming, running or ended is calculated from the times, not stored.
- **Hidden problem:** answers 404 so its existence is not revealed.

## Try it yourself
Not possible yet (untested); Phase 16 runs it. After Phase 16: `make test-mock-contest`, expect `PASS: a full mock contest ran end to end`.

## Trade-offs and risks
- Contests are created by SQL only; no admin API.
- Standings are computed on read (Phase 15 adds caching).
- Rejudged verdicts change standings automatically.
- Every list or detail call adds one visibility query.
- All of it is unverified: SQL, wiring and pages have never run.

## Review questions
Skipped by owner.

## Review Q&A
Skipped by owner (session override: Phases 14 to 16 run in parallel, tested once in Phase 16).

## Open decisions
- Whether contests need an admin creation API (later phase).
- Whether `Event.Elapsed` (the one contract change) is acceptable to Phase 15.

## Handoff
- **State:** branch `phase/14-contests`, tag `phase-14-start`, worktree `Leetforce-p14`; not merged to `main`, no `phase-14-done` tag. Migration 00006 not applied.
- **Next phase:** 15 (Leaderboard, parallel) then 16 (Launch readiness), which merges 14 and 15.
- **What Phase 16 must run to verify Phase 14** (dev host, `.env` with `DATABASE_URL`, `LEETFORCE_REDIS_URL`):
  1. `make migrate-up` (applies `00006_contests.sql`; also `migrate-down` then `migrate-up` to check the Down).
  2. `make fmt lint` and `make test` (includes `go test ./internal/contest/`).
  3. `make test-mock-contest` (expect `PASS: a full mock contest ran end to end`; it deletes its contest and users).
  4. `cd web && npm run lint && npm run typecheck && npm run build`, then look at `/contest` and `/contest/mock-contest` in light and dark mode (seed with `scripts/seed-mock-contest.sh`, clean with `--clean`).
  5. `scripts/scan-staged.sh full` (Trivy) before merging.
  6. Because the submit path changed: `make test-live-e2e` and `make test-auth-e2e` still pass.
- **Next session prompt:** Phase 16's own prompt from the owner.

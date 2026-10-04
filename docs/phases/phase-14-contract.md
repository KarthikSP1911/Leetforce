# Phase 14 contract (for Phase 15)

Phase 15 codes against this. It matches the owner's contract with **one deviation**, marked below.

## Deviation: `Event.Elapsed`

The agreed `Score(events []Event) []Standing` has no contest start, so it cannot compute "whole minutes from contest start to the first AC". `Event` therefore has one extra field:

```go
type Event struct {
    UserID      string
    ProblemSlug string
    SubmittedAt time.Time
    Elapsed     time.Duration // SubmittedAt minus contest start; filled by the store
    Verdict     string
}
```

- Phase 15 should get events only from `Store.Events(ctx, contestID)`, which fills `Elapsed` and already drops attempts outside `[starts_at, ends_at)` and submissions without a verdict.
- Phase 15 tests that build `Event` literals must set `Elapsed`. `Score` ignores events with negative `Elapsed`; it cannot check the end of the window.
- Signature and the `Standing` fields in the contract are unchanged.

## Package `api/internal/contest`

- `Score(events []Event) []Standing`, pure and table-driven tested (`scoring_test.go`).
- `Standing{UserID, Rank, Solved, PenaltyMinutes, PerProblem []ProblemResult}`; `Rank` is competition ranking (1, 1, 3). `ProblemResult{ProblemSlug, Solved, Rejected, SolvedAtMinutes, PenaltyMinutes}`.
- Rules: rank by solved desc, then penalty asc (then user id for a stable order). Penalty per solved problem = whole minutes to the first AC + 20 per rejected attempt before it. CE, IE (internal error), unjudged attempts and attempts after the first AC do not count. A user with no counted event is absent.
- `Store` interface: `ListContests(ctx, userID)`, `GetContest(ctx, slug, userID)`, `Events(ctx, contestID)`, `IsRegistered(ctx, contestID, userID)`, plus `Register` and `Problems`. `contestID` is the contest's uuid as text (`Contest.ID`, not in JSON). `*contest.PG` (`contest.NewPG(store.Pool())`) implements it.
- Standings are keyed by `user_id` only; Phase 15 maps ids to usernames itself.

## Schema (migration `00006_contests.sql`)

`contests(id uuid, slug, title, starts_at, ends_at, created_at)`; status is derived from time. `contest_problems(contest_id, problem_slug, label, position, points)`. `contest_participants(contest_id, user_id, registered_at)`. `submissions.contest_id uuid NULL`. The `points` column is stored but not used by scoring (ICPC).

## HTTP

| Route | Behaviour |
|---|---|
| `GET /contests` | `{contests:[{slug,title,starts_at,ends_at,status,registered,problem_count}]}` |
| `GET /contests/:slug` | the same fields plus `server_time`; 404 if unknown |
| `POST /contests/:slug/register` | 200, idempotent; 401 anonymous; 409 after the end; 404 unknown |
| `GET /contests/:slug/problems` | `{problems:[{label,position,points,slug,title,difficulty}]}`; 404 before start and, during the contest, for unregistered or anonymous callers; public after the end |
| `POST /submissions` `contest_id` | the contest's slug (its id also works); 409 if the window is not open or the user is not registered; 422 if the problem is not in the contest; 404 unknown contest |

Contest problems are 404 on `GET /problems`, `GET /problems/:slug`, `GET /problems/:slug/submissions`, `POST /runs` and `POST /submissions` (without `contest_id`) while the contest has not started, and while it runs for anyone not registered. They are public once it has ended. They are also left out of `GET /problems`.

## Web

`/contest`, `/contest/[slug]`. `web/src/components/contest/standings-slot.tsx` exports `StandingsTable({ contestSlug })`, a placeholder Phase 15 replaces (keep the export name and prop).

## Scripts (written, not run)

`scripts/seed-mock-contest.sh`, `scripts/run-mock-contest.sh` (`make test-mock-contest`), `scripts/mock-contest-e2e.py`, and `api/cmd/contestscore` (standings JSON from real verdicts).

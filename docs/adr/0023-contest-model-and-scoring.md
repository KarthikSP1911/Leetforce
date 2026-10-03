# ADR 0023: Contest model and scoring

Status: accepted (Phase 14; untested until the Phase 16 gate)

## Context

Phase 14 adds timed contests. Phase 15 builds a leaderboard on top, in parallel, so the model and the scoring rules had to be fixed up front (owner's contract, `docs/phases/phase-14-contract.md`).

## Decision

- **Status is derived, not stored.** `contests` has `starts_at` and `ends_at`; upcoming, running and ended come from the clock (`contest.StatusAt`, window `[start, end)`). Nothing can drift or need a job to flip it.
- **Contest submissions are ordinary submissions** with a nullable `submissions.contest_id`. They use the same queue, runner, sandbox and hidden-data rules. Only submissions with `contest_id` count towards standings.
- **ICPC scoring** as a pure function `Score(events)`: solved desc, then penalty asc. Penalty per solved problem is whole minutes from the start to the first AC plus 20 per rejected attempt before it. CE and IE do not count (CE never ran; IE is our fault). Attempts after the first AC do not count.
- **Window is enforced twice:** at submit time (409 outside the window or when not registered) and when reading events (`Events` drops attempts outside the window). `Event` carries `Elapsed` so `Score` needs no contest handle (the one change to the agreed contract).
- **Visibility:** a contest's problems are hidden (404, never 403, so existence is not confirmed) until the start, and during the contest from anyone not registered. They become public after the end. The check is one SQL query (`HiddenProblems`) applied in the list, detail, submissions list, run and submit handlers. If it fails the request fails (no fail-open).
- **Contest is optional wiring:** `server.Deps.Contests == nil` turns contests off.

## Alternatives

- Store status in a column and flip it with a timer: rejected, more moving parts and drift.
- Pass the contest start to `Score`: cleaner signature, but would break Phase 15's compiled contract; an `Elapsed` field is additive.
- Points-based scoring (the `points` column): kept in the schema for later, not used.
- Separate contest submission table: rejected, would duplicate verdict, rejudge and reaper logic.

## Consequences

- A rejudge that changes a verdict (Phase 10) changes standings on the next read; there is no cached standing in Phase 14.
- A problem in several contests is hidden while any of them hides it.
- Hiding adds one query per list or detail request; negligible now, cache it in Phase 15/16 if the load test shows it matters.
- Not measured: nothing was run this phase (gate in Phase 16).

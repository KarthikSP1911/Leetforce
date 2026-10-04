# ADR 0025: Leaderboard ranking, caching and invalidation

Status: accepted (Phase 15; untested, the gate runs in Phase 16)

## Context
Phase 15 adds contest standings and a global ranking. The exit criterion is "rankings are correct under concurrent submissions". Verdicts arrive from many runners at once through the ingester, and a verdict write is already idempotent per submission (`RecordVerdict`). Phase 14 (contests) is built in parallel and provides `Score` (ICPC rules), the contest tables and `submissions.contest_id`; Phase 15 codes against that contract behind a stub (`api/internal/contest/contract_stub.go`, build tag `leaderboard_stub`).

## Decision

**Source of truth.** A ranking is a pure function of rows in Postgres (`submissions` + `verdicts` + Phase 14 tables). Nothing ranking-related is stored as state, so there is nothing to drift. Redis only caches the computed result.

**Contest standings.** `Score` (solved desc, penalty asc; penalty = minutes to first AC + 20 per rejected attempt before it; CE, attempts after the first AC and out-of-window attempts ignored) gives the totals. The window filter (`start <= t <= end`; a submission exactly at the end counts) and the tie-break are applied in `leaderboard.scoreEvents`: equal solved and penalty are ordered by the earlier time of the last first-AC, then username. Rows identical on all three share a rank (1, 2, 2, 4). IE verdicts (judge failures, not the user's fault) are ignored like CE. Only registered participants' submissions count.

**Global ranking.** Per user: distinct problems with at least one AC (any contest or not), weighted easy 1, medium 3, hard 5. Order: score desc, solved desc, earlier last first-AC, username. Same rank for identical triples. The full ranked list (capped at 10 000 rows) is one snapshot; `GET /leaderboard?page=&per_page=` slices it (default 50, max 100).

**Cache.** Redis keys (via `queue.KV*`): `lb:contest:<id>` and `lb:global` hold `{v, data}`; `lb:v:contest:<id>` and `lb:v:global` are version counters. A read takes the counter value FIRST, serves the snapshot only if its `v` equals it, otherwise recomputes from Postgres and stores the result tagged with the counter value read before the query. TTL (standings 15 s, global 60 s) is a backstop only.

**Invalidation.** The ingester calls `OnVerdict(submissionID)` only when `RecordVerdict` returned true (stored or replaced). That does INCR on the global counter and, if the submission belongs to a contest, on that contest's counter. INCR runs after the Postgres commit.

## Why this is correct under concurrency
- A reader that computes while a verdict commits either sees the new row (fresh) or the old state, in which case its snapshot is tagged with the old counter, and the writer's INCR (which happens after commit) makes that snapshot unservable. A stale snapshot can therefore never be served after the bump.
- A replayed or duplicate verdict returns `recorded=false`, so it triggers no invalidation; and even a spurious bump only causes a recompute that yields the identical ranking, because the ranking is a function of the database. So invalidation is idempotent in effect (CLAUDE.md rule).
- If Redis fails after the commit, the bump is lost and staleness is bounded by the TTL (15/60 s); Redis down on read falls back to computing from Postgres.

## Alternatives
- Redis sorted set updated incrementally per verdict: fast reads, but ICPC penalty is not a single additive score, rejudges can lower a score, and replays/races make incremental updates easy to get wrong. Rejected for correctness over speed at this scale.
- Materialised table updated in the ingest transaction: strong consistency, but couples every verdict write to ranking recomputation and needs Phase 14 schema details. Revisit if recompute cost grows.
- Delete-key invalidation instead of a version counter: a reader that started before the delete can store its stale result after it. The counter closes that race.

## Consequences
- Every cache miss runs one aggregate query (global) or one events query (contest). Fine for this scale; thundering herd on a miss is not coalesced (singleflight deferred).
- Assumptions about Phase 14 columns (`contests.id/slug/title/starts_at/ends_at`, `contest_problems.contest_id/problem_slug/position`, `contest_participants.contest_id/user_id`, `submissions.contest_id`) are in `api/internal/store/leaderboard.go`; Phase 16 verifies them after the merge.
- Open assumption: Phase 14's `Score` has no start argument. Events are rebased to offsets from the Unix epoch before the call (`leaderboard.epoch`). If the real signature differs, the change is confined to `score_contract.go` and `scoreEvents`.
- Migration 00015 adds a partial index `verdicts_accepted_idx` for the global scan. Numbers: not measured (no load run in this phase).

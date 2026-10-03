# Phase 15: Leaderboard

**Branch:** `phase/15-leaderboard` (worktree `../Leetforce-p15`)
**Range:** `phase-15-start..phase-15-done` (merged to `main` together with Phase 16)
**Dates:** 2026-10-03 (one session, parallel with phases 14 and 16)
**Milestone:** none
**Status:** done; gate passed in Phase 16. Log: [phase-15-log.md](phase-15-log.md)

## Summary
Adds contest standings (`GET /contests/:slug/standings`, ICPC rules via the Phase 14 `Score` contract, stubbed here) and a global ranking (`GET /leaderboard`, weighted distinct solves), both cached in Redis with a version counter that the ingester bumps after every stored verdict. A concurrency test and a make target exist but were not run.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Rankings are correct under concurrent submissions | PASS | `make test-leaderboard-concurrent`: `TestLeaderboardConcurrentIngest` passes 3 of 3 runs with `-race` against the real schema (Phase 16 combined gate on the dev host, head `1a153e4`, log in [phase-16-log.md](phase-16-log.md)). Two defects in the test itself were found and fixed first: contest problems inserted without the required `label`, and an oracle that scored verdicts after the contest end |

## Branches merged
None. Units were committed directly on `phase/15-leaderboard` (owner override: work fast, parallel phases).

## File-by-file changes

### Added
| File | Purpose |
|---|---|
| `queue/kv.go` | Redis get/set(TTL)/incr/counter helpers used for snapshots and version counters |
| `api/internal/contest/contract_stub.go` | STUB of the Phase 14 contract (tag `leaderboard_stub`); Phase 16 deletes it |
| `api/internal/leaderboard/types.go` | Domain types, `Source`/`Cache` interfaces, JSON response types |
| `api/internal/leaderboard/rank.go` | `RankGlobal`, `scoreEvents` (window, tie-break, ranks, cells) |
| `api/internal/leaderboard/service.go` | Version-tagged snapshot cache, `Standings`, `Global`, `OnVerdict` |
| `api/internal/leaderboard/score_contract.go` | Adapter from `contest.Score` to `Scorer` (tag `leaderboard_stub`) |
| `api/internal/leaderboard/score_none.go` | Nil scorer without the tag (standings answer 503) |
| `api/internal/leaderboard/penalty_test.go` | Table-driven ICPC penalty tests: ties, CE, post-AC, end-time boundary (tag `leaderboard_stub`) |
| `api/internal/leaderboard/service_test.go` | Global ranking, cache hit, invalidation, stale snapshot, paging tests |
| `api/internal/server/leaderboard.go` | The two HTTP handlers |
| `api/internal/store/leaderboard.go` | SQL against contests, participants, submissions, verdicts |
| `api/internal/store/leaderboard_concurrent_test.go` | Concurrency test (tags `leaderboard_concurrent`, `leaderboard_stub`) |
| `api/migrations/00015_leaderboard_indexes.sql` | Partial index on accepted verdicts |
| `docs/adr/0025-leaderboard-ranking-and-cache.md` | Decision record |
| `docs/phases/phase-15-log.md`, `phase-15-summary.md`, `phase-15.md` | Log, summary, this report |
| `scripts/test-leaderboard-concurrent.sh` | Runs the concurrency test with `-race` |
| `web/src/app/leaderboard/page.tsx` | Global leaderboard page |
| `web/src/components/contest/standings-slot.tsx` | `StandingsTable({contestSlug})`, live standings |
| `web/src/types/leaderboard.ts` | TypeScript response types |

### Modified
| File | What changed | Why |
|---|---|---|
| `Makefile` | `test-leaderboard-concurrent` target | Phase 16 gate |
| `api/cmd/api/main.go` | Builds the ranking service, registers it with the ingester and server | Wiring |
| `api/internal/ingest/ingest.go` | `Invalidator` interface, `SetInvalidator`, call after a stored verdict | Cache invalidation, idempotent |
| `api/internal/server/server.go` | `Ranking` dependency and two routes | Serve the rankings |
| `docs/FLOW.md`, `docs/PROGRESS.md` | Phase 15 as built; status in review, untested | Repo rule |
| `web/src/lib/api/client.ts` | `getLeaderboard`, `getStandings` | Web client |

### Deleted / Renamed
None.

## Key code changes
- Cache: `cached()` in `service.go` reads the version counter before querying Postgres and tags the snapshot with it; `OnVerdict` increments after the commit.
- Ingest: invalidation only when `RecordVerdict` returns true, so replays change nothing.
- `scoreEvents`: window filter `[start, end]` inclusive, rebase to epoch offsets for `Score`, last-AC tie-break.

## Decisions
- [ADR 0025](../adr/0025-leaderboard-ranking-and-cache.md): ranking is a function of the database; Redis snapshot with version counter; global weights 1/3/5.

## Tests
- Written: `penalty_test.go`, `service_test.go`, `leaderboard_concurrent_test.go`.
- Run in this phase: only `go test ./internal/leaderboard/` (pure Go, no services), with and without `-tags leaderboard_stub`: pass. The concurrency test and every other suite were not run.
- Commands: `go test -tags leaderboard_stub ./internal/leaderboard/`; `make test-leaderboard-concurrent`.

## Known issues and deferred work
- Concurrency test unrun; SQL against Phase 14 tables unverified (assumed column names, see ADR).
- `Score` signature assumption (no start argument) may need a change in `score_contract.go` / `scoreEvents`.
- `npx tsc` reports `LayoutProps` not found in `src/app/layout.tsx` until `next build` generates types (pre-existing, untouched). `next build` and the pages in a browser not checked. Trivy not run.
- Singleflight on cache miss deferred.

## Stats
- Commits: 6 (excluding merges), plus this report
- Files: 22 added, 6 modified (counted before this report), 0 deleted
- Lines: +1834 / -13 before this report

## Integration (Phase 16)

The stub of the Phase 14 contract (`contract_stub.go`, `score_none.go`, the `leaderboard_stub` build tag) was deleted. The merge exposed one real defect: the adapter never set `contest.Event.Elapsed`, so every penalty scored 0; `penalty_test.go` fails without the fix. Its ADR is 0025. See [phase-16-log.md](phase-16-log.md).

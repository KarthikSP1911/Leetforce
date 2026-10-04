# Phase 15 summary: Leaderboard

## TL;DR
- The API now answers `GET /contests/:slug/standings` (ICPC scoreboard) and `GET /leaderboard` (global ranking), and the web has a `/leaderboard` page and a live standings table.
- A ranking is always computed from Postgres; Redis only caches it, with a version counter that every stored verdict bumps, so a stale ranking is never served.
- **Nothing was run against a database or Redis in this phase.** The concurrency test is written; Phase 16 runs it.

## Where this phase fits
```
browser -> API -> queue -> runner -> sandbox -> verdict -> API ingest -> Postgres -> SSE -> browser
                                                              |  (Phase 15, new)
                                                              +-> bump cache version (Redis)
browser /leaderboard, standings table -> API -> Redis snapshot (valid?) -> else Postgres -> rank -> store snapshot
```
- Built before: ingest with idempotent verdict writes (Phase 4), Redis (Phase 3).
- Depends on Phase 14 (contest tables, `Score`), stubbed here; Phase 16 merges both, removes the stub and tests.

## What I built and why
### Cache primitives (`queue/kv.go`)
- **What:** get, set with TTL, incr and read-counter on Redis.
- **Why:** the ranking cache needs a version counter; the API module has no direct Redis client.

### Ranking and cache (`api/internal/leaderboard`)
- **What:** `scoreEvents` (window filter, ICPC totals from `Score`, per-problem cells, last-AC tie-break, ranks), `RankGlobal` (weights easy 1, medium 3, hard 5), `Service` (snapshot cache, `OnVerdict`).
- **Why:** rankings must equal what the database says even when verdicts arrive concurrently. A snapshot is tagged with the counter value read before querying; a verdict commits, then increments the counter, so any snapshot computed around it becomes unservable.
- **Alternatives:** incremental sorted set, materialised table, delete-key invalidation. See [ADR 0025](../adr/0025-leaderboard-ranking-and-cache.md).

### API, ingest hook, SQL (`server/leaderboard.go`, `store/leaderboard.go`, `ingest.go`)
- **What:** two routes, a partial index (migration 00015), and an `Invalidator` the ingester calls only when `RecordVerdict` returned true.
- **Why:** replayed verdicts must change nothing (CLAUDE.md idempotency rule).

### Web (`web/src/app/leaderboard`, `components/contest/standings-slot.tsx`)
- **What:** server-rendered paginated table; client table polling every 10 s (paused when the tab is hidden) with a Refresh button, brand tokens only, verdict cells carry text labels.

## How it works now, step by step
1. A runner finishes a contest submission; the result reaches the ingester.
2. `RecordVerdict` writes the verdict once (a duplicate returns false).
3. If it returned true, `OnVerdict` bumps `lb:v:global` and `lb:v:contest:<id>`.
4. The next standings request reads the counter, finds the old snapshot tagged with an older number, recomputes from Postgres, stores the new snapshot.
5. The browser polls and shows the new order.

## Key concepts
- **Version-tagged cache:** a snapshot carries the counter value it was computed under; it is served only while the counter has not moved.
- **ICPC penalty:** minutes to first AC plus 20 per rejected attempt before it; CE and later attempts ignored.
- **Competition ranking:** equal results share a rank (1, 2, 2, 4).

## Try it yourself (Phase 16, after merging Phase 14)
```
make test-leaderboard-concurrent                       # race detector, needs DATABASE_URL
cd api && go test -tags leaderboard_stub ./internal/leaderboard/   # before the stub is removed
curl http://localhost:8080/leaderboard?page=1
curl http://localhost:8080/contests/<slug>/standings
```

## Trade-offs and risks
- Each cache miss is one aggregate query; no request coalescing yet.
- Phase 14 column names and the `Score` signature are assumptions; the stub may not match.
- A lost Redis INCR leaves staleness up to the TTL (15 s / 60 s).

## Review questions
Skipped by owner (standing override for the parallel phases 14 to 16).

## Review Q&A
Skipped by owner.

## Open decisions
- Global weights 1/3/5 and counting contest solves in the global ranking are Claude defaults; the owner may change them (Phase 16 or later).

## Handoff
- **State (updated in Phase 16):** merged into `main` with Phase 16, tags `phase-15-start` and `phase-15-done`; the concurrency gate passed. Earlier state: branch `phase/15-leaderboard`, in review, untested.
- **Next phase:** 16 Launch readiness. See the handoff in the chat and `docs/PROGRESS.md`.
- **Next session prompt:** Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-15-summary.md, then start Phase 16 (Launch readiness). Merge phases 14 and 15, replace the Phase 15 stub, and run every deferred gate.

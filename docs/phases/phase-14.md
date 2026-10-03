# Phase 14: Contests

**Branch:** `phase/14-contests`
**Range:** `phase-14-start..HEAD` (not merged to `main`; Phase 16 merges)
**Dates:** 2026-10-03 (one session, parallel with Phases 15 and 16)
**Milestone:** none
**Log:** [phase-14-log.md](phase-14-log.md) · **Contract for Phase 15:** [phase-14-contract.md](phase-14-contract.md)

## Summary
Contests exist as a timed window over a set of problems, with registration, contest-only problem visibility, window-checked submissions and ICPC scoring as a pure function. The web has a contest list and a contest page with a countdown, problem tabs and a standings slot for Phase 15. **Nothing was run against a database, runner or browser; the gate is in Phase 16.**

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| a full mock contest runs end to end | not yet verified | written: `make test-mock-contest` (`scripts/run-mock-contest.sh`); runs in Phase 16 |
| scoring rules correct | unit-tested only | `go test ./internal/contest/` (`TestScore`, `TestScorePerProblem`, `TestStatusAt`) pass |
| visibility and register rules | unit-tested only | `TestProblemsVisibility`, `TestRegister` pass (fake store); SQL gates untested |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/14-contest-api` | migration, contest package, server gating, contestscore | 1 |
| `feat/14-contest-web` | contest pages, API client, `?contest=` in the workspace | 1 |
| `test/14-mock-contest` | seed and driver scripts, Makefile target | 1 |

## File-by-file changes (from `git diff --name-status phase-14-start..HEAD`, code commits)

### Added
| File | Purpose |
|---|---|
| `api/migrations/00006_contests.sql` | contests, contest_problems, contest_participants, `submissions.contest_id` and indexes |
| `api/internal/contest/model.go` | `Contest`, `Problem`, statuses, `StatusAt`, errors |
| `api/internal/contest/scoring.go` | `Event`, `Standing`, `Score` (ICPC) |
| `api/internal/contest/scoring_test.go` | table-driven scoring and status tests |
| `api/internal/contest/store.go` | `Store` interface and Postgres `PG` (events, hidden problems, submit check) |
| `api/internal/contest/handlers.go` | `/contests` handlers |
| `api/internal/contest/handlers_test.go` | visibility and register tests with a fake store |
| `api/internal/server/contests.go` | `ContestService`, route wiring, hidden-problem and submit gates |
| `api/cmd/contestscore/main.go` | prints standings JSON from real verdicts |
| `scripts/seed-mock-contest.sh` | creates the mock contest and 3 users (`--clean` removes them) |
| `scripts/run-mock-contest.sh` | starts API and runner, seeds, drives, cleans up |
| `scripts/mock-contest-e2e.py` | the checks: register, AC/WA/CE, visibility, 409s, scores |
| `web/src/app/contest/page.tsx` | contest list |
| `web/src/app/contest/[slug]/page.tsx` | contest page |
| `web/src/components/contest/ContestView.tsx` | countdown, register, problem tabs, standings section |
| `web/src/components/contest/StatusBadge.tsx` | status badge with text label, time formatting |
| `web/src/components/contest/standings-slot.tsx` | `StandingsTable` placeholder for Phase 15 |
| `web/src/types/contest.ts` | contest API types |

### Modified
| File | What changed | Why |
|---|---|---|
| `Makefile` | `test-mock-contest` target | Phase 14 gate |
| `api/cmd/api/main.go` | passes `contest.NewPG(db.Pool())` in `Deps` | enable contests |
| `api/internal/store/store.go` | `Store.Pool()` | the contest package owns its queries |
| `api/internal/server/server.go` | `Deps.Contests`, `contestRoutes` | wiring |
| `api/internal/server/problems.go` | hide contest problems in list and detail | contest-only visibility |
| `api/internal/server/runs.go` | 404 for hidden problems | no Run on a hidden problem |
| `api/internal/server/submissions.go` | `contest_id`, window and registration checks, tag before enqueue, hidden check on submit and history | contest submissions |
| `web/src/lib/api/client.ts` | contest calls, `contest_id` on submit | client |
| `web/src/hooks/useJudge.ts` | optional contest argument sent as `contest_id` | Submit in a contest |
| `web/src/components/workspace/Workspace.tsx` | optional `contest` prop | pass it to `useJudge` |
| `web/src/app/problems/[slug]/page.tsx` | read `?contest=` | link from the contest page |

### Deleted / Renamed
None.

## Key code changes
- `contest.Score` (`scoring.go`): sorts a copy of the events by time, keeps per user and problem the rejected count until the first AC, ignores CE, IE, unjudged and later attempts, ranks 1, 1, 3.
- `PG.HiddenProblems` (`store.go`): one query for problems of contests that have not started or, while running, that the user is not registered for. Applied in four handlers; an error fails the request instead of leaking.
- `createSubmission` (`submissions.go`): `contest_id` is checked before the insert (404/409/422), the tag is written before the job is queued, and on a tag failure the row is deleted.

## Decisions
- [ADR 0023](../adr/0023-contest-model-and-scoring.md): derived status, contest submissions are ordinary submissions, ICPC scoring, `Event.Elapsed` deviation.

## Tests
- New: `scoring_test.go`, `handlers_test.go` (both pass, run alone). Not added: Postgres store tests and server tests for the gates (need a database; the mock-contest driver covers them in Phase 16).
- Phase 16 commands: see the handoff in [phase-14-summary.md](phase-14-summary.md).

## Known issues and deferred work
- Not run: migration, `make fmt lint`, full tests, Trivy, the mock contest, browser check in light and dark mode.
- `npx tsc --noEmit` reported `layout.tsx: Cannot find name 'LayoutProps'` (Next-generated type; unverified).
- No standings endpoint (Phase 15). `GET /submissions/:id` and the event stream do not check contest visibility.
- No admin API to create contests; contests are created by SQL (seed script).
- Review Q&A skipped by the owner.

## Stats
- Commits: 3 (excluding merges), plus docs commits
- Files: 18 added, 11 modified, 0 deleted (code commits)
- Lines: +1828 / -16 (code commits)

# Phase 16: Launch readiness

**Branch:** `phase/16-launch-readiness` (contains the merged Phases 14 and 15)
**Range:** `phase-16-start..phase-16-done`
**Dates:** 2026-10-03 to 2026-10-04 (one session, with Phases 14 and 15 built in parallel by other sessions)
**Milestone:** M5 (launch-ready, with the open items in [launch-checklist.md](../launch-checklist.md))
**Log:** [phase-16-log.md](phase-16-log.md)

## Summary
Phase 16 integrated the untested Phases 14 (contests) and 15 (leaderboard), ran the first combined test gate on the dev host, and delivered the launch-readiness work: a security review with 18 findings (7 fixed with tests, 11 accepted in writing), a load tester, a backup and restore drill with a runbook, a README with a high-level system design, a cost review and a launch checklist. The gates found and fixed nine defects, including a zero-penalty bug in the merged scoring and a leak of contest problem slugs through public standings. At the owner's request the web UI also got a landing page, professional fonts, custom dropdowns and scrollbars, aligned panes, SVG artwork and Framer Motion animations. The cloud was not applied: the Phase 13 exit criteria remain unproven.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Load test run and results recorded | PASS | `make test-loadtest-local`: 24 of 24 submissions reached a verdict (mixed and contest modes, 6 users x 2 iterations each), 0 errors, 0 rate limited; time to verdict p50 8.1 s and 6.6 s on a 1 vCPU host with one runner. Not a capacity claim. See the log |
| Review findings resolved or accepted in writing | PASS | [security-review.md](../security-review.md): SEC-01 to SEC-05, SEC-16, SEC-17 fixed with tests; SEC-06 to SEC-15 and SEC-18 accepted with reasons; Trivy full scan clean (0.75.0) |
| Backup and restore drill | PASS with limits | Scratch-database restore PASS (goose version 15, 8 tables equal); S3 drill PASS but vacuous (the AWS bucket has no objects yet); Neon branch mode not run. [runbook](../runbook-backup-restore.md) |
| Phase 14 exit: mock contest end to end | PASS | `make test-mock-contest`, 32 checks |
| Phase 15 exit: rankings correct under concurrency | PASS | `make test-leaderboard-concurrent`, 3 of 3 with `-race` |
| Combined gate green on the final code | PASS | 16 steps, head `1a153e4`, [log](phase-16-log.md) |

## Branches merged
| Branch | Purpose | Non-merge commits |
|---|---|---|
| `feat/16-backup-restore` | Neon and S3 backup scripts, restore drill, runbook | see `git log --merges` |
| `feat/16-docs-cost` | README, cost review, FLOW verification | see `git log --merges` |
| `feat/16-load-test` | load tester, ADR, make target | see `git log --merges` |
| `feat/14-contest-api, feat/14-contest-web, test/14-mock-contest (via phase/14-contests)` | Phase 14 as built by its own session | see `git log --merges` |
| `phase/15-leaderboard` | Phase 15 as built by its own session (no unit branches) | see `git log --merges` |
| `feat/16-security-review` | review, five fixes with tests, Trivy suppression | see `git log --merges` |
| `fix/16-lint-merge` | gofmt and noctx in merged code | see `git log --merges` |
| `fix/16-loadtest-contest-route` | contest load uses the real Phase 14 routes; local load script | see `git log --merges` |
| `fix/16-script-modes` | executable bits and `make check-scripts` | see `git log --merges` |
| `feat/16-web-polish` | landing page, auth pages, custom controls, fonts | see `git log --merges` |
| `fix/16-navbar-align` | navbar in the page column | see `git log --merges` |
| `fix/16-leaderboard-script` | drop the stub tag from the concurrency script | see `git log --merges` |
| `feat/16-home-art` | hero grid and pipeline SVG | see `git log --merges` |
| `fix/16-gate-scripts` | leaderboard test label; load script free port | see `git log --merges` |
| `feat/16-web-motion` | Framer Motion, sign-up fits the window | see `git log --merges` |
| `fix/16-leaderboard-oracle` | oracle applies the contest window | see `git log --merges` |
| `fix/16-backup-perms` | dump mode 0600 (SEC-17) | see `git log --merges` |
| `fix/16-standings-prestart` | no problem slugs in standings before the start (SEC-16) | see `git log --merges` |

21 merge commits in the range, 46 non-merge commits (34 of them unique to Phase 16; the rest came with Phases 14 and 15).

## File-by-file changes
Generated from `git diff --name-status phase-16-start` (the final run of this list is after the last commit).

### Added by Phase 16
| File | Purpose | Why |
|---|---|---|
| `api/internal/contest/handlers_test.go` | `NewRequestWithContext` in the test helper | golangci-lint noctx |
| `api/internal/leaderboard/penalty_test.go` | Removed the stub build tag | The contract is real now; the test is the regression check for the zero-penalty defect |
| `api/internal/leaderboard/score_contract.go` | Always built; sets `Elapsed` from the rebased time | Without `Elapsed` every contest penalty scored 0 |
| `api/internal/leaderboard/service.go` | No problem slugs in standings before the start | SEC-16 |
| `api/internal/leaderboard/service_test.go` | `TestStandingsHideProblemsBeforeStart` | Regression test for SEC-16 |
| `api/internal/server/events_ip_test.go` | Tests for the per-IP stream cap | SEC-01 regression |
| `api/internal/server/security_json_test.go` | Tests that non-JSON POSTs get 415 | SEC-03 regression |
| `api/internal/server/security_login_test.go` | Tests for the shared login budget | SEC-02 regression |
| `api/internal/store/leaderboard_concurrent_test.go` | Sets `label` on contest problems; oracle applies the contest window; no stub tag | The test failed on the real schema and compared against an oracle that counted verdicts after the contest end |
| `docs/adr/0024-load-test-tool.md` | Load test tool decision: stdlib Go program over k6 | Phase 16 ADR (renumbered from 0023 at integration) |
| `docs/cost-review.md` | Every recurring cost, what runs now, monthly estimate, shutdown list | Phase 16 cost review; dollar figures are UNVERIFIED estimates |
| `docs/launch-checklist.md` | Phase 13 leftovers, Phase 16 deliverables, what the owner must still decide | Single list for launch |
| `docs/phases/phase-14-summary.md` | State line updated | Phase 14 is merged |
| `docs/phases/phase-14.md` | Exit criteria evidence and integration note | Gate passed in Phase 16 |
| `docs/phases/phase-15-summary.md` | State line updated | Phase 15 is merged |
| `docs/phases/phase-15.md` | Exit criteria evidence and integration note | Gate passed in Phase 16 |
| `docs/phases/phase-16-log.md` | Running log of every Phase 16 step, fixes, mistakes, final gate | Required by CLAUDE.md; this report is built from it |
| `docs/phases/phase-16-summary.md` | Plain-language explainer for the owner | Required by CLAUDE.md section 7.2 |
| `docs/phases/phase-16.md` | This report | Required by CLAUDE.md section 7.1 |
| `docs/runbook-backup-restore.md` | Runbook, RPO and RTO (unverified), drill results table | Backup and restore drill deliverable |
| `docs/security-review.md` | Security review: 18 findings with status, evidence, tests | Phase 16 exit criterion |
| `scripts/backup-neon.sh` | Neon backup with `pg_dump`, counts and checksum; mode 0600 | Backup deliverable; the chmod is SEC-17 |
| `scripts/backup-s3-bundles.sh` | Backup and restore-verify of the test-bundle bucket | Backup deliverable |
| `scripts/mock-contest-e2e.py` | Executable bit | Consistency; it is run through `python3` |
| `scripts/restore-drill-neon.sh` | Restore drill into a scratch database or Neon branch, with checks and cleanup | Backup deliverable |
| `scripts/test-leaderboard-concurrent.sh` | Executable bit; dropped the stub tag logic | `make test-leaderboard-concurrent` failed with Permission denied |
| `scripts/test-loadtest-local.sh` | Local-stack load test: free port, mixed and contest modes, verdict check, cleanup | Phase 16 exit test |
| `tools/loadtest/go.mod` | Module for the load tester | stdlib only |
| `tools/loadtest/load.go` | Virtual users: sign-up, list, Run, Submit, SSE; contest mode registers and submits with `contest_id` | Load test deliverable |
| `tools/loadtest/load_test.go` | Table-driven tests against a fake server | Covers mixed, contest, rate limiting and abort paths |
| `tools/loadtest/main.go` | Flags and output | CLI |
| `tools/loadtest/report.go` | Latency percentiles, per-step counts, JSON output | Reporting |
| `web/src/components/home/HomeArt.tsx` | Hero grid and animated pipeline diagram (inline SVG) | Owner request for SVG artwork |
| `web/src/components/motion/Motion.tsx` | `MotionProvider`, `Reveal`, `Enter`, `ButtonLink` | Framer Motion helpers that honour reduced motion |
| `web/src/components/ui/Select.tsx` | Custom accessible listbox dropdown | Replaces native `<select>` |
| `web/src/lib/safe-next.test.ts` | Open-redirect test inputs | SEC-04 regression |
| `web/src/lib/safe-next.ts` | Rejects `/\host` and similar `?next=` values | SEC-04 |

### Modified by Phase 16 (some files also changed with Phases 14 and 15)
| File | What changed | Why |
|---|---|---|
| `.gitignore` | Ignore `backups/` | Dumps hold user data and must never be committed |
| `.trivyignore` | Suppress AWS-0132 until 2027-04-03 | The data bucket uses SSE-S3 on purpose (a customer key costs about 1 USD a month); reason and expiry are in the file and SEC-12 |
| `CLAUDE.md` | New make targets, font and layout rules, status line | Keeps the repo guide true: Inter and JetBrains Mono replace the system stack at the owner's request; `make dev` starts only Redis since Phase 13 |
| `Makefile` | Added `loadtest`, `test-loadtest-local`, `check-scripts`, `test-mock-contest`, `test-leaderboard-concurrent`; `lint` depends on `check-scripts`; `tools/loadtest` in `GO_MODULES` | Gates for Phase 14 to 16 and a check that scripts stay executable |
| `README.md` | Rewritten: architecture, high-level system design, quickstart, commands, cost table | The README described Phases 0 to 8 only |
| `api/cmd/api/main.go` | `IdleTimeout` 2 minutes; wires the ranking service and contests | SEC-05 (idle keep-alive connections were never reaped); Phase 14 and 15 wiring |
| `api/internal/server/auth.go` | Login attempts share one budget per account across email and username | SEC-02 |
| `api/internal/server/events.go` | Per-IP cap on open event streams | SEC-01 (one IP could take every stream slot) |
| `api/internal/server/server.go` | `requireJSON` on state-changing routes; leaderboard and standings routes | SEC-03; Phase 15 routes |
| `docs/FLOW.md` | Verified against the code; Phase 14 to 16 as-built flows | Several statements were stale (Phase 3 heartbeat, Phase 8, 10, 12) |
| `docs/PROGRESS.md` | Phase 16 done, rows for phases 14 to 16, open decisions | Resume point and tracker |
| `go.work` | Added `tools/loadtest` | The load tester is its own module |
| `scripts/k3s/push-ssm.sh` | Executable bit | `make check-scripts` |
| `scripts/k3s/sync-secrets.sh` | Executable bit | `make check-scripts` |
| `scripts/test-runner-loss.sh` | Executable bit | `make check-scripts` |
| `web/next.config.ts` | `allowedDevOrigins` from `LEETFORCE_DEV_ORIGINS` | Lets the dev server be opened from a LAN address |
| `web/package-lock.json` | Lockfile for `framer-motion` and the new test script | Generated |
| `web/package.json` | Added `framer-motion`, `npm test` script | Animations; test for SEC-04 |
| `web/src/app/globals.css` | Inter and JetBrains Mono, themed thin scrollbars | Owner request for professional fonts and controls |
| `web/src/app/layout.tsx` | Loads the fonts with `next/font`, motion provider | Self-hosted fonts, reduced-motion support |
| `web/src/app/page.tsx` | Real landing page replacing the redirect | Hero, features, how it works, call to action |
| `web/src/components/auth/AuthForm.tsx` | Two-column layout, show/hide password, hints as placeholders, `safeNext` | Professional auth pages that fit the window; SEC-04 |
| `web/src/components/layout/Navbar.tsx` | Client component; same column as the page; all-caps Inter wordmark; logo links home | Alignment fixes |
| `web/src/components/layout/UserMenu.tsx` | Avatar with the username initial, animated menu | Owner request |
| `web/src/components/problems/ProblemFilters.tsx` | Custom dropdowns with hidden inputs | Replace native selects, keep the plain GET form |
| `web/src/components/problems/ProblemTable.tsx` | Removed the Status column, added a `#` column | Owner request |
| `web/src/components/workspace/CodeEditor.tsx` | Editor uses the code font from the theme | Monaco cannot read CSS variables |
| `web/src/components/workspace/SplitPane.tsx` | Divider invisible until hover or focus | Cleaner panes |
| `web/src/components/workspace/Tabs.tsx` | 44px tab header | Aligned pane headers |
| `web/src/components/workspace/Workspace.tsx` | 44px toolbar, custom language dropdown, exact viewport height | No page scrollbar next to the workspace |
| `web/tsconfig.json` | `allowImportingTsExtensions` | Lets the node test import `.ts` files |

### Deleted
| File | Reason |
|---|---|
| `api/internal/contest/contract_stub.go` | Phase 15's stub of the Phase 14 contract; deleted in the merge (it never existed at `phase-16-start`, so it is not in the net diff) |
| `api/internal/leaderboard/score_none.go` | The no-scorer fallback for builds without the contract; same reason |

### Renamed / moved
| From | To | Reason |
|---|---|---|
| `docs/adr/0023-load-test-tool.md` | `docs/adr/0024-load-test-tool.md` | Phase 14 took 0023 and Phase 15 took 0025 (renamed before the net diff, so only the new name appears above) |

### Arrived with the merge of Phase 14 (described in [phase-14.md](phase-14.md))
| Status | File |
|---|---|
| A | `api/cmd/contestscore/main.go` |
| A | `api/internal/contest/handlers.go` |
| A | `api/internal/contest/model.go` |
| A | `api/internal/contest/scoring.go` |
| A | `api/internal/contest/scoring_test.go` |
| A | `api/internal/contest/store.go` |
| A | `api/internal/server/contests.go` |
| M | `api/internal/server/problems.go` |
| M | `api/internal/server/runs.go` |
| M | `api/internal/server/submissions.go` |
| M | `api/internal/store/store.go` |
| A | `api/migrations/00006_contests.sql` |
| A | `docs/adr/0023-contest-model-and-scoring.md` |
| A | `docs/phases/phase-14-contract.md` |
| A | `docs/phases/phase-14-log.md` |
| A | `scripts/run-mock-contest.sh` |
| A | `scripts/seed-mock-contest.sh` |
| A | `web/src/app/contest/[slug]/page.tsx` |
| A | `web/src/app/contest/page.tsx` |
| M | `web/src/app/problems/[slug]/page.tsx` |
| A | `web/src/components/contest/ContestView.tsx` |
| A | `web/src/components/contest/StatusBadge.tsx` |
| A | `web/src/components/contest/standings-slot.tsx` |
| M | `web/src/hooks/useJudge.ts` |
| M | `web/src/lib/api/client.ts` |
| A | `web/src/types/contest.ts` |

### Arrived with the merge of Phase 15 (described in [phase-15.md](phase-15.md))
| Status | File |
|---|---|
| M | `api/internal/ingest/ingest.go` |
| A | `api/internal/leaderboard/rank.go` |
| A | `api/internal/leaderboard/types.go` |
| A | `api/internal/server/leaderboard.go` |
| A | `api/internal/store/leaderboard.go` |
| A | `api/migrations/00015_leaderboard_indexes.sql` |
| A | `docs/adr/0025-leaderboard-ranking-and-cache.md` |
| A | `docs/phases/phase-15-log.md` |
| A | `queue/kv.go` |
| A | `web/src/app/leaderboard/page.tsx` |
| A | `web/src/types/leaderboard.ts` |

## Key code changes
1. **Penalty adapter (`api/internal/leaderboard/score_contract.go`).** `contest.Score` ranks by `Event.Elapsed`; Phase 15's adapter left it zero, so every penalty was 0. The adapter now sets `Elapsed: e.SubmittedAt.Sub(epoch)`, because `scoreEvents` already rebases times to an offset from the Unix epoch. `penalty_test.go` fails without this line (`penalty 0, want 11`).
2. **Standings visibility (`api/internal/leaderboard/service.go`).** `problems := c.Problems; if s.now().Before(c.StartsAt) { problems = []string{} }`. The public standings no longer reveal a contest's problem set before the start (SEC-16).
3. **Per-IP stream cap (`api/internal/server/events.go`).** One IP may hold at most 10 of the 200 SSE slots (SEC-01).
4. **Load tester (`tools/loadtest`).** A stdlib Go program: each virtual user signs up as `lfload_<run>_<n>`, then loops list, Run, Submit and follows the SSE stream to the verdict. 429 responses are counted apart from errors. Contest mode registers the user, takes the first contest problem and submits with `contest_id`. [ADR 0024](../adr/0024-load-test-tool.md).
5. **Backup scripts (`scripts/backup-neon.sh`, `restore-drill-neon.sh`, `backup-s3-bundles.sh`).** The restore drill restores a fresh dump into a scratch database (or Neon branch), compares row counts and the goose version, and always cleans up. Neon runs PostgreSQL 18, so the client tools must be version 18 too.

## Decisions
- [ADR 0024](../adr/0024-load-test-tool.md): a stdlib Go load tester instead of k6 (no extra install, shares the API's route knowledge, one binary).
- ADR 0023 (contests) and ADR 0025 (leaderboard) were written by the Phase 14 and 15 sessions and are unchanged.
- Owner decisions this session: use the dev host and the real Neon database for the gate; use a local scratch Postgres for the restore drill; fonts, layout and animation choices for the web UI.

## Tests
- New: `events_ip_test.go`, `security_login_test.go`, `security_json_test.go` (API), `safe-next.test.ts` (web), `TestStandingsHideProblemsBeforeStart`, `tools/loadtest/load_test.go`.
- Run: `make fmt lint test`, `make test-sandbox`, `make test-adversarial`, `make test-api-e2e test-live-e2e test-auth-e2e test-rejudge-e2e`, `make validate-problems`, `make test-mock-contest`, `make test-leaderboard-concurrent`, `make test-loadtest-local`; in `web/`: `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`.

## Known issues and deferred work
- Phase 13 cloud exit criteria unproven (no apply, AMI attempt 4 untried, runner-loss test not run); M4 is untagged. See the launch checklist.
- SEC-06 (no TLS in front of the API) and SEC-11 (branch protection) need the owner before any public launch.
- The AWS data bucket is empty, so the S3 drill proved only the mechanics. The Neon branch restore mode is untested. The Neon restore window and plan limits are unchecked.
- Cost figures in the README and cost review are estimates marked UNVERIFIED.
- A stale test stack (an API and a runner) from before this session is still running on the dev host and was not touched.
- The `framer-motion` animations were checked in a background browser tab only (frames were throttled); they should be looked at once in a normal tab.
- The load test used one runner on a 1 vCPU host: it shows the pipeline works, not how it scales.

## Stats
- Commits: 46 non-merge in the range (34 unique to Phase 16), 21 merges
- Files: 66 added, 39 modified, 0 deleted (net diff, including files that arrived with Phases 14 and 15)
- Lines: 104 files changed, 7655 insertions(+), 225 deletions(-) (this report excluded from the line counts)

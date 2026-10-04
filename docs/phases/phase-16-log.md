# Phase 16 log: launch readiness

Running log of what was done, by whom, and what happened. No secrets or addresses here.

## Backup and restore drill

Who: Claude (subagent), branch `feat/16-backup-restore` off `phase/16-launch-readiness`. Nothing was run against the real database, bucket or Neon API; only syntax checks and `--dry-run`.

### Files added or changed

- `scripts/backup-neon.sh`: `pg_dump --format=custom` of `LEETFORCE_MIGRATE_DATABASE_URL` (else `DATABASE_URL`) into `backups/neon/`, plus `.counts` (goose max applied version and per-table row counts) and `.sha256`. URL never printed. `--dry-run`, `--out`.
- `scripts/restore-drill-neon.sh`: restore into a scratch target, verify goose version, per-table counts and a sample query (`count(*)` on `problems`), clean up via an exit trap. Modes: `scratchdb` (temporary database on `LEETFORCE_SCRATCH_ADMIN_URL`) and `branch` (temporary Neon branch through the Neon REST API with `NEON_API_KEY`, `NEON_PROJECT_ID`; schema `public` is reset on the branch before restoring). `--dry-run`, `--dump`.
- `scripts/backup-s3-bundles.sh`: `aws s3 sync` of `problems/` down, count and byte comparison, sha256 list, restore into `restore-drill/<ts>/` in the same bucket, checksum comparison on a pull back, scratch prefix deleted by trap. Uses the aws cli (already used in `scripts/test-runner-loss.sh`); maps `LEETFORCE_S3_ACCESS_KEY/SECRET_KEY` to the cli only when set (empty means IAM role); `LEETFORCE_S3_ENDPOINT` only for offline RustFS. Never touches `tfstate/`.
- `docs/runbook-backup-restore.md`: scope, RPO/RTO (Neon PITR window marked UNVERIFIED), secrets handling, restore for total loss, bad migration (branch from a past timestamp) and bucket, drill procedure, results table (empty).
- `.gitignore`: added `backups/`.

How Neon is managed (checked first): `infra/neon/README.md` and `versions.tf` use `NEON_API_KEY` from the environment; the project is imported by Terraform; schema comes from goose (`make migrate-*`, directory `api/migrations`). `NEON_PROJECT_ID` is the project id from `infra/neon/terraform.tfvars` (git-ignored).

### Commands run and results

```
bash -n scripts/backup-neon.sh            # no output: syntax ok
bash -n scripts/restore-drill-neon.sh     # no output: syntax ok
bash -n scripts/backup-s3-bundles.sh      # no output: syntax ok
LEETFORCE_S3_BUCKET=dummy-bucket bash scripts/backup-s3-bundles.sh --dry-run
  # printed the 3-step plan, "dry-run: prerequisites ok, nothing read or written"
DATABASE_URL=postgres://u:p@h/db bash scripts/backup-neon.sh --dry-run
  # "backup-neon: pg_dump not found ..." (exit 1): prerequisite check works; this Windows host has no PostgreSQL client
DATABASE_URL=postgres://u:p@h/db bash scripts/restore-drill-neon.sh --dry-run
  # "restore-drill: pg_dump not found" (exit 1), same reason
```

shellcheck is not installed on this host, so only `bash -n` was used. The two Neon scripts could not reach their plan output here for lack of `pg_dump`/`psql`; run them with `--dry-run` on the dev host (Linux, Postgres client installed) first.

### Open points

- The real drill is not run. It needs owner approval (see the report to the integrator): reads production data, may create a Neon branch, and writes/deletes under `restore-drill/` in the real bucket.
- PITR window and RTO values in the runbook are UNVERIFIED until the owner checks the Neon console and the drill is timed.
- Not covered: scheduled backups (no cron or GitHub Actions job yet), encryption of dumps at rest beyond the local disk.
## File and path index
| Path | What |
|---|---|
| `README.md` | Rewritten: description, architecture, quickstart, commands table, cost table with UNVERIFIED figures |
| `docs/cost-review.md` | New: every recurring cost, running vs code-only, monthly estimate, shutdown list |
| `docs/FLOW.md` | Full pass against the code; corrections listed below |

## Docs and cost review

Who: Claude (docs subagent). Branch `feat/16-docs-cost` off `phase/16-launch-readiness`. Nothing billable was run and AWS was not queried.

### Steps and results
1. `git checkout -b feat/16-docs-cost`: ok. Read `CLAUDE.md`, `README.md`, `docs/FLOW.md`, `docs/PROGRESS.md`, `.env.example`, `Makefile`, `docker-compose.yml`, ADRs 0003, 0019, 0020, 0021, 0022, `infra/aws/*.tf`, `infra/bootstrap`, `infra/neon`, `packer/runner.pkr.hcl`, phase 12 and 13 logs and reports.
2. Cost facts found in the repo: instance types (`t3.small` control and runner, `t3.micro` dev), 15 GiB gp3 volumes (`root_volume_gib`), a public IP on every host (no NAT), S3 about $0.025 per GB-month (ADR 0021, the only dollar figure in the repo), 216k Upstash commands a month at the 60 s sampler (ADR 0019), Neon free-plan assumption (`infra/neon/variables.tf`, 6 h restore window). No hourly EC2, EBS or IPv4 price exists in the repo; the README and `docs/cost-review.md` mark every such figure UNVERIFIED and state the basis (my recollection of ap-south-1 list prices).
3. FLOW.md verification, each claim checked against code with `grep` and `ls`. Confirmed: reaper 15 min and 2 min grace (`api/internal/reaper/reaper.go`), SSE 500 ms poll, 15 s keep-alive, 10 min limit, 200 streams (`api/internal/server/events.go`), per-minute limits 10/30/20/60 and auth limits 20/10 (`api/cmd/api/main.go`), metrics ports 9102 and 9101, run TTL 10 min (`queue/run.go`), `MinIdle` 30 s, `MaxDeliveries` 3, verdict TTL 7 days, status stream cap 5000 (`queue/`), session cookie `lf_session` with 30-day TTL, enqueue failure deletes the row and returns 503, ten alert names in `observability/prometheus/alerts.yml`, `ci.tf` trusts `environment:production`, IMDS hop limit 2 on the control host and 1 on runners, 34 resources to add in the `infra/aws` plan (phase-13 log), five migrations. All function and file names cited in sections 1 to 13 were grepped (for example `RecordVerdict`, `ReapUnqueued`, `BeginRejudge`, `SyncProblem`, `processRun`, `RunCustom`, `limitUserAndIP`, `SolvedProblems`, `newCgroupJob`, `parseLog`, `seccompPolicy`); none were missing.
4. FLOW.md corrections made (discrepancies):
   - Intro said Phases 0 to 8 are built and 9 to 16 planned; now 0 to 13 built, 14 to 16 planned.
   - Phase 12 and 13 table rows were out of date (12 "AMI pending", 13 "in progress"); aligned with PROGRESS ("done (partial)", M4 not tagged).
   - Note under the table said Phases 6 to 16 are only a reading of the plan; now only 14 to 16.
   - Phase 3: heartbeat was "every MinIdle/3"; the code uses `HeartbeatEvery`, default 10 s (`runner/internal/agent/agent.go`), which is MinIdle/3 only at the defaults.
   - Phase 8: said unit 1 (run endpoint) was unmerged and the browser check pending; the phase-8 log shows it merged, migration 00004 applied and the Chrome check done.
   - Phase 10: referred to `bin/rejudge`; no Makefile target builds it, the real command is `go run ./cmd/rejudge` from `api/`.
   - Phase 12: Neon state was "infra/neon/terraform.tfstate" (local); since Phase 13 it is `tfstate/neon.tfstate` in S3. Runner hosts are now an Auto Scaling group; port 80 was added in Phase 13; the exit-check sentence said `terraform plan` was still pending although it ran in Phase 13.
   - Phases 14, 15, 16 had no section 3 entries; placeholders added and not ticked.
5. Other discrepancies noticed outside the files I edited (not changed, reported): `CLAUDE.md` says `make dev` starts "local Redis and S3 (RustFS...)", but since Phase 13 it starts only Redis (RustFS is commented out in `docker-compose.yml`). `CLAUDE.md` says "Planned, not defined yet: make scan", fine. `docs/PROGRESS.md` has no row for Phases 14 to 16 (not my task).
6. Logo: `web/public/brand/logo-mark.svg` is referenced as-is in the README header; no file under `web/public/brand` was touched.
7. Commit: `docs(docs): ...` with `Refs: phase-16` footer, on `feat/16-docs-cost`; not merged.

### UNVERIFIED items for the owner
- All dollar figures (EC2, EBS, IPv4, snapshot): list-price assumptions for ap-south-1.
- Neon plan and limits; Upstash plan and monthly command limit; GHCR and GitHub Actions allowances.
- That `leetforce-dev` is still the only running instance and that no builder, snapshot, Elastic IP or volume is left over (AWS was not queried).
- Whether the Neon password pasted into chat in Phase 13 was rotated.
## Load test

Branch `feat/16-load-test` (off `phase/16-launch-readiness`). Done by Claude, in a worktree on the Windows machine; the real stack was not run (needs the dev host).

### What was added
- `tools/loadtest/` (`main.go`, `load.go`, `report.go`, `load_test.go`, `go.mod`): stdlib-only Go load tester. See `docs/adr/0024-load-test-tool.md`.
- `go.work`: added `./tools/loadtest`. `Makefile`: `tools/loadtest` added to `GO_MODULES`, new `loadtest` target.
- `docs/adr/0024-load-test-tool.md`.

### Routes used (verified in `api/internal/server/server.go`)
`POST /auth/signup` (201), `GET /problems`, `POST /runs` (202) and `GET /runs/:id` (`status` becomes `done`), `POST /submissions` (202), `GET /submissions/:id/events` (SSE, `event: verdict` ends it). API listens on `:8080` by default (`LEETFORCE_API_ADDR`). Contest mode (corrected at integration, see the Integration section): register, list the contest problems, then `POST /submissions` with `contest_id`.

### How to run
```bash
make loadtest ARGS="-users 20 -duration 2m -ramp 20s -json out.json"
LEETFORCE_LOADTEST_BASE_URL=https://host make loadtest ARGS="-mode contest -contest <slug> -users 50 -iterations 3"
```
Flags: `-users`, `-iterations`, `-duration`, `-ramp`, `-mode mixed|contest`, `-contest`, `-problem`, `-language`, `-source-file`, `-password`, `-verdict-timeout`, `-json`, `-base-url`.
Sign-up is rate limited per IP (20 per 10 min, ADR 0017): set `LEETFORCE_LIMIT_*` higher on the test deployment for more than 20 users per run from one IP.

### Cleanup of test users (do not run against the real DB without checking the count first)
Accounts are `lfload_<runid>_<n>` with email `...@loadtest.invalid`. Check, then delete (adjust FK handling to the schema in `api/migrations`; delete dependent rows first if no `ON DELETE CASCADE`):
```sql
SELECT count(*) FROM users WHERE email LIKE '%@loadtest.invalid';
-- submissions by test users (only if user_id has no cascade):
DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@loadtest.invalid');
DELETE FROM users WHERE email LIKE '%@loadtest.invalid';  -- sessions cascade or delete them first
```
Not executed anywhere.

### Commands and results
- `cd tools/loadtest && go vet ./... && go test -count=1 ./...` -> `ok leetforce/tools/loadtest` (table-driven tests against an httptest fake: mixed flow, 429 counted apart from errors, contest mode, contest 404 abort, config errors, percentiles, JSON/text report).
- `golangci-lint fmt ./...` and `golangci-lint run ./...` in `tools/loadtest` -> 0 issues. Two findings fixed on the way: gosec G304 in the test (`filepath.Clean`) and staticcheck QF1002 (tagged switch).
- Mistake: first attempt to write the files with one long shell command was refused by the sandbox; files were written individually instead (no effect on the result).
- Full `make fmt lint` was not run across the other modules on this machine; only the new module was linted.
## Security review

Unit `feat/16-security-review`, branched from `phase/16-launch-readiness` at `62e6926`. Findings and statuses: [docs/security-review.md](../security-review.md).

1. Claude: `git checkout -b feat/16-security-review`. Result: branch created.
2. Claude: read every file under `api/internal/server`, `api/internal/store`, `api/internal/ingest`, `api/internal/catalog`, `api/cmd/api`, `runner/internal/agent`, `runner/internal/problems`, `runner/nodb_test.go`, `queue/run.go`, `storage/storage.go`, `judge/problem/bundle.go`, `judge/sandbox/args.go` and `cgroup.go` (read only), `web/src`, `web/next.config.ts`, `infra/aws/*.tf`, `infra/bootstrap`, `k8s/base/*`, `ansible/roles/hardening`, `scripts/runner/*`, `scripts/k3s/*`, `.github/workflows/deploy.yml`, `docker-compose.yml`, `.env.example`, `.gitignore`, `.trivyignore`. No files changed by this step.
3. Claude: history checks for secrets.
   - `git log --all --oneline -G 'AKIA[0-9A-Z]{16}|npg_[A-Za-z0-9]{8,}|PRIVATE KEY-----'`: no output.
   - `git log --all --oneline -G '(rediss|postgres|postgresql)://[^ :/<$]+:[^ @<$]{6,}@'`: no output.
   - `git ls-files | grep -E '(^|/)\.env'`: only `.env.example`. `git log --all --diff-filter=A -- .env '*.pem' '*.tfstate' '*.tfvars'`: attempted in a form the sandbox refused to verify; the working-tree check and the two content searches above were used instead.
   - `git grep` for hard-coded secret-looking assignments (excluding tests, docs, lockfiles): only `http_tokens = "required"` (IMDSv2 setting, not a secret).
   - `git grep` for IPv4 literals in docs, Terraform, scripts: only public DNS resolvers and private ranges in docs, no owner or host address.
4. Owner machine (Claude through Bash): `ssh -o BatchMode=yes -o ConnectTimeout=15 leetforce-dev 'echo ok'`. Result: `ok`, dev host reachable (one attempt, no retry needed).
5. Claude: `bash scripts/scan-staged.sh full` (Trivy 0.75.0 on the dev host, scans `git archive $(git write-tree)`, i.e. the tree of `62e6926`). Result: exit 1. 0 vulnerabilities in `api`, `judge`, `queue`, `runner`, `storage` Go modules and `web/package-lock.json`; Dockerfile clean; 1 HIGH misconfiguration, `AWS-0132` in `infra/bootstrap/main.tf:45-52` (S3 bucket encrypted with SSE-S3, not a customer managed key). Handled as SEC-12 (accepted, suppressed in `.trivyignore` with expiry 2027-04-03). Re-run of the scan on the final tree is left to the integrator's gate (needs a staged tree).
6. Claude: `go test` in `runner` for `TestRunnerHasNoDatabaseDependency`: pass (the "runners never connect to the database" guard still holds).
7. Fixes, each a small commit with a regression test (all `Refs: phase-16`, co-author trailer):
   - `37a8fc9` security(api): cap open event streams per client IP (SEC-01). Files: `api/internal/server/events.go`, `server.go`, new `events_ip_test.go`.
   - `f4c99f5` security(api): count login attempts per account, not per spelling (SEC-02). Files: `api/internal/server/auth.go`, new `security_login_test.go`.
   - `b3b4873` security(api): require application/json on state-changing POSTs (SEC-03). Files: `api/internal/server/server.go`, new `security_json_test.go`.
   - `6bdcab1` security(api): reap idle keep-alive connections (SEC-05). File: `api/cmd/api/main.go`. First tried `ReadTimeout` 30s; dropped it before committing because Go keeps the read deadline armed during a handler and would cut SSE streams.
   - `c5e2155` security(web): close open redirect through a backslash in ?next= (SEC-04). Files: `web/src/lib/safe-next.ts`, `safe-next.test.ts`, `web/src/components/auth/AuthForm.tsx`, `web/package.json` (new `test` script), `web/tsconfig.json` (`allowImportingTsExtensions`).
8. Mistakes and corrections during the unit:
   - The first Python edit script wrote with the Windows default encoding; the resulting diffs were checked and contained only the intended lines. Later edits used the Edit tool.
   - A `git commit -F -` in PowerShell 5.1 treated the message as a pathspec; messages were written to files in the scratchpad and passed with `-F <file>`.
   - The husky `lint-staged` hook runs `prettier` and `eslint` from `web/node_modules`, which the worktree does not have. For the web commit a directory junction `web/node_modules` pointing at the main checkout's `node_modules` was created, the commit made, and the junction removed again (the target was left intact).
   - The first version of the per-IP stream test tripped staticcheck SA4000 (identical operands); rewritten as a loop and committed as `1dba319`.
   - `npx tsc --noEmit` reports `Cannot find name 'LayoutProps'` in `web/src/app/layout.tsx`. That is a Next-generated type (needs `.next/types`, produced by `next build`) and is unrelated to these changes.
9. Checks run before commit: `golangci-lint run ./...` in `api` (0 issues after the SA4000 fix); `go vet ./internal/server/` and `go test ./internal/server/` in `api` (pass); `npm test` in `web` (2 tests pass); `prettier --check` and `eslint` on the touched web files (clean). `make fmt` was not used on the Windows machine; `gofmt -w` was applied to the touched Go files. Sandbox, adversarial and database tests were not run (they need the dev host); no sandbox code was changed.
10. Claude: added `AWS-0132 exp:2027-04-03` with a reason to `.trivyignore` (SEC-12) and wrote `docs/security-review.md`.

### Files touched by the security review

| Path | Change |
|---|---|
| `api/internal/server/events.go` | per-IP stream cap |
| `api/internal/server/server.go` | `requireJSON` middleware, slot construction |
| `api/internal/server/auth.go` | per-account-id login counter |
| `api/internal/server/events_ip_test.go`, `security_login_test.go`, `security_json_test.go` | new regression tests |
| `api/cmd/api/main.go` | `IdleTimeout` |
| `web/src/lib/safe-next.ts`, `web/src/lib/safe-next.test.ts` | new, redirect validation and its test |
| `web/src/components/auth/AuthForm.tsx`, `web/package.json`, `web/tsconfig.json` | use the new helper, `npm test`, TS extension imports |
| `.trivyignore` | `AWS-0132` with reason and expiry |
| `docs/security-review.md` | findings |
| `docs/phases/phase-16-log.md` | this file |

## Integration (Part B)

Who: Claude (integrator). Date: 2026-10-04.

| Step | Command | Result |
|---|---|---|
| Phase 16 branch | `git checkout -b phase/16-launch-readiness; git tag phase-16-start` at `main` 62e6926 | The session started on `main`, not on the phase branch; the branch and tag did not exist, so they were created |
| Part A merges | `git merge --no-ff feat/16-{backup-restore,docs-cost,load-test,security-review}` | Four merges; `docs/phases/phase-16-log.md` had an add/add conflict each time (every subagent created it); resolved by keeping every section |
| Merge Phase 14 | `git merge --no-ff phase/14-contests` | Conflicts in `Makefile` (`.PHONY` line) and `docs/FLOW.md`; resolved as a union |
| ADR numbers | `git mv docs/adr/0023-load-test-tool.md docs/adr/0024-load-test-tool.md` | Phase 14 took 0023 and Phase 15 took 0025, so the load test ADR is 0024 |
| Merge Phase 15 | `git merge --no-ff phase/15-leaderboard` | Conflicts in `Makefile`, `api/cmd/api/main.go`, `api/internal/server/server.go`, `web/src/lib/api/client.ts`, `web/src/components/contest/standings-slot.tsx`, `docs/FLOW.md`, `docs/PROGRESS.md`; resolved as a union (Deps has both `Contests` and `Ranking`; routes keep `requireJSON` on `POST /runs` from the security review); Phase 15 wins `standings-slot.tsx` |
| Stub removal | `git rm api/internal/contest/contract_stub.go api/internal/leaderboard/score_none.go`; removed the `leaderboard_stub` build tag from `score_contract.go`, `penalty_test.go`, `store/leaderboard_concurrent_test.go` | `ContractScorer` now always uses `contest.Score` |
| **Integration bug found** | `go test ./internal/leaderboard/` with and without the change | `contest.Score` ranks by `Event.Elapsed`, and the stub never set it, so every penalty came out 0. `ContractScorer` now sets `Elapsed: e.SubmittedAt.Sub(epoch)`. Regression test: `penalty_test.go` fails without it (`penalty 0, want 11`) and passes with it |
| Migrations | `ls api/migrations` | 00001 to 00006, then 00015 (leaderboard indexes): ascending, with a gap |
| Local gates (Windows PC) | `go vet`, `golangci-lint run`, `go test -count=1` for each module in `GO_MODULES` | `queue`, `storage`, `tools/loadtest`: pass. `api`: 3 lint issues (gofmt x2, noctx x1) fixed in `b00dd42`; all tests pass. `judge` and `runner`: cannot compile on Windows (Linux-only syscalls in `judge/sandbox`), so they need the dev host |
| Web gates | `npm run lint`, `typecheck`, `npm test`, `npm run build` in `web/` | All pass; routes `/contest`, `/contest/[slug]`, `/leaderboard` build |

Not yet run (need the Linux dev host and the real Neon database; waiting for the owner): `make test-sandbox`, `make test-adversarial`, `make test-api-e2e`, `make test-live-e2e`, `make test-auth-e2e`, `make test-rejudge-e2e`, `make validate-problems`, `scripts/seed-mock-contest.sh` and `scripts/run-mock-contest.sh`, `make test-leaderboard-concurrent`, the load test, the backup/restore drill, and the Trivy full scan on the merged tree.

## Fixes found by the gates (all on `fix/16-*` branches, merged with default messages)

| Found by | Defect | Fix | Regression check |
|---|---|---|---|
| Local lint | 3 lint issues in merged code (gofmt twice, noctx once) | `b00dd42` | `make lint` |
| Merge | `ContractScorer` never set `Event.Elapsed`, so every penalty was 0 | in the Phase 15 merge commit | `penalty_test.go` fails without it (`penalty 0, want 11`) |
| Review of the load tool | contest mode posted to a guessed route that does not exist | `fix/16-loadtest-contest-route` | `tools/loadtest` tests with a fake contest server |
| First gate run | `test-leaderboard-concurrent` failed with Permission denied: scripts committed without the executable bit (six of them) | `15468ac` | `make check-scripts`, part of `make lint` |
| Re-run | leaderboard test inserted `contest_problems` without the NOT NULL `label` | `c373614` | the test itself, now passing |
| Re-run | leaderboard test oracle scored verdicts after the contest end (240 submissions one minute apart, 180 minute window); the service was right | `fix/16-leaderboard-oracle` | the test itself, 3 of 3 runs |
| Load test run | a stale API and runner from before this session held the fixed port 18085, so the first load test hit an old build | script now picks a free port and checks its own API process is the one answering | `scripts/test-loadtest-local.sh` |
| Restore drill | dump file mode 644 when `pg_dump` runs in a container (SEC-17) | `32fe90b` | checked by hand on the host |
| Own review of Phases 14 and 15 | standings listed contest problem slugs before the start (SEC-16) | `e9a69c9` | `TestStandingsHideProblemsBeforeStart` fails without it |

## Mistakes and corrections

- The session was started on `main` and the phase branch did not exist; it was created with the `phase-16-start` tag.
- A first mock-contest failure came from a demo contest created in the owner's browser session (it held the same problems and hid them from the test users). The demo contest was removed for the gate and recreated afterwards.
- Two commits were rejected by commitlint (a `style` type, an over-long header) and redone. One commit was made with a reused message file, so the phase branch was reset to the commit before it (nothing had been pushed) and the change redone with its own message.
- A combined shell command failed to parse and applied nothing; edits were redone from script files.
- The restore drill first recorded one table because a Docker shim passed `-i` to `psql` and swallowed the rest of the table list from a pipe; the shim was fixed. (Not a defect in the scripts.)
- The dev host disk was 98% full; `go clean -cache` freed about 1.9 GB (regenerable). Nothing else on the host was deleted.
- A web edit to the Navbar made a mistake with `prettier` reordering classes that broke a text replacement; checked and redone.

## Final combined gate (one run on the final code)

Run on the dev host (Ubuntu 24.04, 1 vCPU, 911 MB RAM, real Neon database and Upstash Redis, sandbox as root with nsjail and cgroup v2) by `~/gates16f.sh`, head `1a153e4` (the last commit that touches code; later commits are documents only). Start 19:45:48, end 20:04:26 on the host clock. Per-step logs are in `~/gates16f/` on the host. The Trivy and Go versions are Trivy 0.75.0 and Go 1.27.1.

| Step | Command | Result |
|---|---|---|
| scripts executable | `make check-scripts` | rc 0 |
| migrations | `make migrate-status` | rc 0; head `00015_leaderboard_indexes.sql` (applied to the real Neon database earlier with `make migrate-up`, rc 0) |
| format | `make fmt` | rc 0, no files changed |
| lint | `make lint` | rc 0, 0 issues in all modules |
| unit tests | `make test` | rc 0, 24 packages ok, 0 failures |
| build | `make build-judge build-runner build-api` | rc 0 |
| problems | `make validate-problems` | rc 0, `5 of 5 problems valid` (starter warnings only) |
| sandbox | `make test-sandbox` | rc 0 |
| adversarial | `make test-adversarial` | rc 0, 49 passing tests, 0 failed or skipped |
| API e2e | `make test-api-e2e` | rc 0: `AC stored through API, runner and ingest; a duplicate verdict changed nothing; a dead-lettered job became IE` |
| live e2e | `make test-live-e2e` | rc 0: queued, judging, verdict over SSE; no hidden data in any response; an orphan re-queued and judged |
| auth e2e | `make test-auth-e2e` | rc 0: accounts, login required, limits, solved status, Run and Submit as a signed-in user |
| rejudge e2e | `make test-rejudge-e2e` | rc 0: a fixed test set re-queued the submission, the new verdict replaced the old one once |
| mock contest (Phase 14 exit) | `make test-mock-contest` | rc 0, 32 checks, `PASS: a full mock contest ran end to end` |
| leaderboard concurrency (Phase 15 exit) | `make test-leaderboard-concurrent` | rc 0, `TestLeaderboardConcurrentIngest` PASS 3 of 3 with `-race` (about 33 s each) |
| load test (Phase 16 exit) | `make test-loadtest-local` | rc 0, see below |

Load test numbers (6 users x 2 iterations per mode, a single 1 vCPU host, one runner, throwaway queue prefix, limits raised, free port):

| Mode | Submissions accepted | Reached a verdict | Errors | Rate limited | Time to verdict p50 / p95 / max |
|---|---|---|---|---|---|
| mixed (sign-up, list, Run, Submit, SSE) | 12 | 12 | 0 | 0 | 8.1 s / 8.4 s / 8.4 s |
| contest (register, contest problems, Submit with `contest_id`, SSE) | 12 | 12 | 0 | 0 | 6.6 s / 8.2 s / 8.2 s |

These figures say the system works under a small load; they are not a capacity claim (one runner, one CPU).

Web gates on the PC (final tree): `npm run lint`, `npm run typecheck`, `npm test` (2 pass), `npm run build` all rc 0. `go vet ./...` and `go test ./...` in `api/` on the PC: all pass.

Trivy: `scripts/scan-staged.sh full` (Trivy 0.75.0) on the merged tree: clean, exit 0.

Restore drill (`docs/runbook-backup-restore.md` has the table): scratch database PASS; S3 PASS but vacuous; Neon branch mode not run.

Not run: `terraform apply`, the AMI build, the runner-loss test, anything in the cloud (the owner did not ask for them; see `docs/launch-checklist.md`).

## Later web changes on `main` (after the merge)

After the Phase 16 merge the owner asked for further web polish, each done on a branch and merged with a merge commit, then pushed: hero spacing (`4661d6b`), the dark editor window in the hero (`b6ffe1e`), hero scale and alignment (`3cd9ca7`), and the placeholder text on the sign-in and sign-up forms (`cb84952`).

Mistake: `cb84952` was committed directly on `main`, not on a branch, and pushed. The first attempt to commit it stalled in the pre-commit hook; the command chain had already switched to `main` and removed the branch, and the retry committed there. The change is small and correct (placeholders only, lint and typecheck pass, checked in the browser), and `main` history is not rewritten, so it stays and is recorded here. Lesson: when a chained git command stalls, check `git branch --show-current` before retrying.


## Helm chart for the API (after the merge, branch `feat/16-helm-chart`)

The owner asked for Helm charts wherever needed. Claude did this on the PC; nothing ran in the cloud.

- Replaced the Kustomize base `k8s/base/*` with the chart `k8s/charts/leetforce-api/` (Chart.yaml, values.yaml, templates/{_helpers.tpl,configmap,deployment,service,ingress}.yaml). `git mv k8s/base/namespace.yaml k8s/namespace.yaml` (kept outside the chart, ADR 0026).
- `scripts/k3s/deploy.sh`: now `kubectl apply -f k8s/namespace.yaml`, then `helm upgrade --install api ... --set image.repository/--set image.tag=<sha> --wait=false`; on a failed rollout `helm rollback api`. Sets `KUBECONFIG=/etc/rancher/k3s/k3s.yaml` by default. `bash -n` passes.
- Ansible `k3s_server`: new tasks download the pinned Helm 3.16.2 tarball (checksum from get.helm.sh) and install `/usr/local/bin/helm`; vars in `defaults/main.yml`. Not run on a host.
- Verified with Docker (no Helm on the PC): `docker run alpine/helm:3.16.2 lint` (0 failed) and `template` with `ingress.host`/`tlsSecretName` set (renders host and tls). Rendered default output was not diffed against the old Kustomize output beyond reading it.
- Docs: ADR 0026, `k8s/README.md`, `docs/FLOW.md` (Phase 13 row text). Not done: Trivy `config` scan of the chart (dev host not used this session).
- Paths: `k8s/charts/leetforce-api/**`, `k8s/namespace.yaml`, `scripts/k3s/deploy.sh`, `ansible/roles/k3s_server/{tasks,defaults}/main.yml`, `docs/adr/0026-helm-chart-for-the-api.md`.

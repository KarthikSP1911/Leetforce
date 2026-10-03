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
`POST /auth/signup` (201), `GET /problems`, `POST /runs` (202) and `GET /runs/:id` (`status` becomes `done`), `POST /submissions` (202), `GET /submissions/:id/events` (SSE, `event: verdict` ends it). API listens on `:8080` by default (`LEETFORCE_API_ADDR`). Contest endpoint: no contest routes were committed on `phase/14-contests` when checked, so `POST /contests/<slug>/submissions` is an assumption; override with `-contest-path`. A 404 aborts with an error.

### How to run
```bash
make loadtest ARGS="-users 20 -duration 2m -ramp 20s -json out.json"
LEETFORCE_LOADTEST_BASE_URL=https://host make loadtest ARGS="-mode contest -contest <slug> -users 50 -iterations 3"
```
Flags: `-users`, `-iterations`, `-duration`, `-ramp`, `-mode mixed|contest`, `-contest`, `-contest-path`, `-problem`, `-language`, `-source-file`, `-password`, `-verdict-timeout`, `-json`, `-base-url`.
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

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

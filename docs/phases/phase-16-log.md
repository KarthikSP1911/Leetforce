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

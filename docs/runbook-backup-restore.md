# Runbook: backup and restore

Scope: the two pieces of durable LeetForce state. Everything else (runners, API hosts, Redis queue contents, AMIs) is rebuilt from code (`infra/aws`, Packer, Ansible) and holds nothing we cannot recreate.

| State | Where | Owner of truth | Backed up by |
|---|---|---|---|
| Postgres (users, problems, submissions, verdicts, goose history) | Neon project (`infra/neon`) | Neon | Neon's own history (PITR, branches) plus our logical dumps (`scripts/backup-neon.sh`) |
| Test bundles (`problems/` prefix) | S3 bucket `LEETFORCE_S3_BUCKET` (ADR 0011, 0021) | Source of truth is `problems/` in the repo; the bucket is a published copy | Bucket versioning (on), plus local sync copy (`scripts/backup-s3-bundles.sh`) |
| Terraform state (`tfstate/` prefix) | Same bucket (ADR 0021) | The bucket | Bucket versioning (old versions expire after 90 days). Not covered by our scripts. |
| Redis (Upstash) | Upstash | n/a | Not backed up: the queue is transient. Jobs lost in a Redis wipe are re-queued by the reaper from Postgres. |

## Objectives

| | Target | Basis |
|---|---|---|
| RPO, database, Neon history | Seconds to minutes inside the PITR window | UNVERIFIED: the PITR window depends on the Neon plan (free plans have a short restore window, paid plans longer). Confirm in Neon console > Settings > Storage ("Restore window") and write the value here: `____`. |
| RPO, database, logical dump | Age of the last dump (we take them by hand; target daily once scheduled) | Our own schedule |
| RPO, bundles | Zero for content in git: rebuild from `problems/` with `go run ./api/cmd/rejudge` sync; S3 object versioning covers overwrites | ADR 0021 |
| RTO, database | UNVERIFIED until the drill below is run; record the measured restore time in the results table | Drill |
| RTO, bucket | UNVERIFIED until measured; bundles are a few MB so expected minutes | Drill |

Do not quote the objectives above to anyone as guaranteed until the Result column of the drill table is filled in.

## Secrets handling

- Read connection strings and keys from the environment or git-ignored `.env` only: `DATABASE_URL`, `LEETFORCE_MIGRATE_DATABASE_URL`, `NEON_API_KEY`, `NEON_PROJECT_ID`, `LEETFORCE_S3_*`, `LEETFORCE_SCRATCH_ADMIN_URL`. In the cloud they come from SSM.
- The scripts never print a URL, password or key. Do not add `set -x`, and do not paste output of `env` into docs.
- Dumps contain user data (emails, password hashes, submitted source). They are written with mode 0600 into `backups/` (git-ignored). Keep them on an encrypted disk, never commit them, never upload them to a public location, and delete them when the drill is over (`shred -u` or `rm`).
- Scratch targets are deleted by the script (trap on exit). If a script prints `cleanup FAILED`, delete the named branch, database or prefix by hand straight away: a leftover Neon branch holds a full copy of production data and may cost compute.

## Routine backups

```bash
scripts/backup-neon.sh --dry-run        # check prerequisites (pg_dump, psql, URL set)
scripts/backup-neon.sh                  # dump -> backups/neon/leetforce-neon-<ts>.dump (+ .counts, .sha256)
scripts/backup-s3-bundles.sh --no-restore --dry-run
scripts/backup-s3-bundles.sh --no-restore   # sync problems/ to backups/s3, verify counts and bytes
```

Use a `pg_dump` whose major version is at least the Neon server version (`psql "$DATABASE_URL" -c 'show server_version'`).

## Restore procedures

### A. Total loss of the database (Neon project gone or unusable)

1. Create a new Neon project (console, or `infra/neon` with a new `project_id`; terraform apply needs owner approval). Take its direct (non-pooler) connection string.
2. Pick the newest dump and verify it: `sha256sum -c backups/neon/<file>.dump.sha256`.
3. Restore: `pg_restore --no-owner --no-privileges --exit-on-error --dbname="$NEW_URL" backups/neon/<file>.dump` (empty database; `--clean --if-exists` if it is not).
4. Verify: `goose_db_version` max applied version equals the one in `<file>.dump.counts`, and per-table counts match (the restore drill automates exactly this).
5. Point `DATABASE_URL` and `LEETFORCE_MIGRATE_DATABASE_URL` (SSM in the cloud, `.env` locally) at the new database, restart the API, run `make migrate-status` (expect no pending migrations).
6. Anything submitted after the dump is lost. Submissions still queued in Redis are judged normally; the API reports their verdicts only if the submission rows exist, so re-submit what is missing.
7. Update the Neon project id in `infra/neon` (`terraform.tfvars`, import block) and record the incident in the phase log.

### B. Bad migration or accidental data change (Neon branch from a past timestamp)

Requires the problem to be found inside the PITR window (see Objectives).

1. Stop writers if the damage is ongoing: scale the API to zero or stop it.
2. Create a branch at a time just before the bad change (console: Branches > Create branch > "Past data", or the API):
   ```bash
   curl -fsS -X POST -H "Authorization: Bearer $NEON_API_KEY" -H 'Content-Type: application/json' \
     -d '{"branch":{"name":"recover-<ts>","parent_id":"<main branch id>","parent_timestamp":"<RFC3339 UTC time before the change>"},"endpoints":[{"type":"read_write"}]}' \
     "https://console.neon.tech/api/v2/projects/$NEON_PROJECT_ID/branches"
   ```
3. Inspect the branch with its connection string (console > Connection details; do not paste it anywhere). Compare against main.
4. Either copy the affected rows back with `psql`/`pg_dump --table` from the branch, or restore main in place with the console's "Restore" (Branches > main > Restore, choose the timestamp; Neon keeps the old state as a backup branch automatically). In-place restore is the faster path for a broken migration.
5. Re-check the goose version and run `make migrate-status`. Fix the migration before re-applying it.
6. Delete the recovery branch(es) when done: they keep a full copy of the data and cost compute and storage.

### C. Bucket (test bundles lost, overwritten or deleted)

1. First choice, versioned bucket: list the versions of the lost keys (`aws s3api list-object-versions --bucket "$LEETFORCE_S3_BUCKET" --prefix problems/<slug>/`) and copy the wanted version back with `aws s3api copy-object --copy-source "<bucket>/<key>?versionId=<id>"` (or remove the delete marker).
2. Second choice, local backup copy: `aws s3 sync backups/s3/problems "s3://$LEETFORCE_S3_BUCKET/problems/"` (a `sync` without `--delete` never removes anything). Verify counts and checksums: rerun `scripts/backup-s3-bundles.sh --no-restore` and compare with `backups/s3/problems.sha256`.
3. Third choice, rebuild from git: the `problems/` directory is the source; `go run ./api/cmd/rejudge -dry-run <slug>` shows what a resync will do, then run it without `-dry-run`. Test-set versions (ADR 0011) in the bundle keys must match what submissions recorded, so prefer 1 or 2 for old versions.
4. Terraform state loss is a separate case (bucket `tfstate/` prefix): restore the previous object version of the state file, never hand-edit it.

## Restore drill

Purpose: prove that a backup restores into a working database and that the bucket copy is complete, without touching production. Run before launch and after any change to the schema tooling or the backup scripts; then at least quarterly.

Prerequisites (owner approval is needed for the billable or networked steps, see below):
- `pg_dump`, `pg_restore`, `psql` (same major version as Neon), `aws`, and for branch mode `curl` and `jq`.
- `.env` with `DATABASE_URL` (or the direct URL), `LEETFORCE_S3_*`; branch mode adds `NEON_API_KEY`, `NEON_PROJECT_ID`; scratchdb mode adds `LEETFORCE_SCRATCH_ADMIN_URL` (a local or otherwise disposable Postgres, for example `docker run -d --rm -p 5433:5432 -e POSTGRES_PASSWORD=... postgres:17`).

Steps:
1. `scripts/restore-drill-neon.sh --dry-run --mode scratchdb` (and `--mode branch`): prints the plan, checks prerequisites, creates nothing.
2. `scripts/restore-drill-neon.sh --mode scratchdb`: dumps the source (read-only), restores into a new throwaway database, checks the goose version, every table's row count, and a sample query on `problems`, then drops the database. Time the run: it is the restore component of the RTO.
3. `scripts/restore-drill-neon.sh --mode branch`: the same against a temporary Neon branch, which also proves the Neon API path used in procedure B. The script deletes the branch on exit.
4. `scripts/backup-s3-bundles.sh`: syncs `problems/` down, compares counts and bytes, restores into `restore-drill/<ts>/`, compares checksums, deletes that prefix.
5. Delete the dump and bucket copy from `backups/` if they are not needed, and record the result below.

Row counts are compared against counts taken at dump time. If the source takes writes between the dump and the verify, a FAIL on an active table can be a race, not a defect: rerun while traffic is quiet.

### Drill results

| Date | Who | Target | Dump size | Time to restore | Result | Notes |
|---|---|---|---|---|---|---|
| (not yet run) | | scratchdb | | | | |
| (not yet run) | | Neon branch | | | | |
| (not yet run) | | S3 bundles | | | | |

PITR window confirmed from the Neon console: `____` (date and who checked).

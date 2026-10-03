# ADR 0021: One real S3 bucket for test data and Terraform state

Status: accepted (owner chose "S3 everywhere", "single S3", state in S3 with locking; 2026-10-03)

## Context
Problem test bundles lived in a local RustFS container (ADR 0011) and Terraform state in local files (ADR 0020). Phase 13 runs in the cloud, where runners need the bundles and the owner wants state off the PC.

## Decision
- **One bucket**, `leetforce-<account id>-data`, created by its own stack `infra/bootstrap` (local state: it cannot store its own state in itself). Versioned, SSE-S3 encrypted, public access blocked, TLS-only policy, `prevent_destroy`.
- **Prefixes:** `problems/` (test bundles; kept, since a rejudge may need an old version) and `tfstate/` (state for `infra/aws` and `infra/neon`; old versions expire after 90 days).
- **Access by prefix, not by bucket:** runners get `GetObject` on `problems/*` only; the control host (API) adds `PutObject`. Neither can read `tfstate/`. This also replaces the shared key pair noted in ADR 0011 for cloud hosts: they use their IAM role (`storage.Open` uses the instance role when both keys are empty).
- **State locking** uses the S3 lock file (`use_lockfile`, Terraform >= 1.10), so no DynamoDB table.
- **S3 in every environment.** RustFS stays in `docker-compose.yml` as a commented-out block for offline work.

## Alternatives
- Two buckets (data, state): cleaner blast radius, but the owner asked for one; prefix-scoped IAM gives most of the same isolation.
- Bucket inside `infra/aws`: `arena.sh down` would delete the data and the state.
- DynamoDB lock table: extra resource and cost; the lock file does the same job.
- RustFS locally: no AWS dependency in tests, but a second code path to keep working.

## Consequences
- Local development and `make test-*` gates that read bundles now need AWS access (keys in `.env`) or the directory mode (`LEETFORCE_S3_ENDPOINT` empty).
- Cost: S3 storage about $0.025 per GB-month plus requests; a few MB of tests is effectively free (README cost table).
- Bootstrap state is local; losing it costs one `terraform import` of one bucket.
- Test data and state share one bucket, so a mistake in a bucket-wide policy affects both; policies here are prefix-scoped and TLS-only.

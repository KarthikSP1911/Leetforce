# Phase 13: Cloud deployment (DRAFT, phase in progress)

**Branch:** `phase/13-cloud-deployment`
**Range:** `phase-13-start..HEAD` (not yet `phase-13-done`)
**Milestone:** M4 (not yet reached)
**Log:** [phase-13-log.md](phase-13-log.md). **Decisions:** [ADR 0021](../adr/0021-single-s3-bucket.md), [ADR 0022](../adr/0022-k3s-control-and-standalone-runners.md).

## Summary
All code for the cloud deployment is written and merged: S3 storage, a remote Terraform state backend, k3s manifests for the API, a CI deploy pipeline, and a runner Auto Scaling group with first-boot SSM configuration. **Nothing is applied yet**, so the exit criterion "runs in the cloud and survives a runner loss" is not demonstrated. The AMI build, which the runners need, has failed three times and the owner chose to retry it at the end.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| System runs in the cloud | not demonstrated | `infra/aws` plan: 34 to add, 0 to change, 0 to destroy; `terraform validate` ok; not applied (billable). |
| Survives a runner loss | not demonstrated | `scripts/test-runner-loss.sh` / `make test-runner-loss` written and shellchecked, never run (needs real hosts). |
| AMI builds | not demonstrated | Attempt 3 failed on Ubuntu base-image findings; fix `7a415f1` untested. |
| Nothing billable without confirmation | met | No apply, destroy or `arena.sh up` run; AMI builds were owner-confirmed. |

## Branches merged
`feat/13-s3-storage`, `feat/13-s3-backend`, `fix/13-trivy-timeout`, `feat/13-k3s-control`, `fix/13-ami-updates`, `test/13-runner-loss`, `feat/13-ci-deploy`, `feat/13-runner-scaling` (8 merges, 24 non-merge commits to date).

## File-by-file changes (from `git diff --name-status phase-13-start..HEAD`)
### Added
| File | Purpose |
|---|---|
| `.github/workflows/deploy.yml` | Build, Trivy scan, push to GHCR, deploy via SSM Run Command |
| `ansible/roles/k3s_server/{defaults,tasks}/main.yml` | Installs pinned k3s on the control host |
| `api/Dockerfile`, `api/Dockerfile.dockerignore` | Distroless non-root API image (includes `problems/`) |
| `docs/adr/0021-single-s3-bucket.md`, `0022-k3s-control-and-standalone-runners.md` | Decisions |
| `docs/phases/phase-13-log.md` | Running log |
| `infra/aws/ci.tf`, `ci_outputs.tf` | GitHub OIDC provider and deploy role |
| `infra/aws/runner-asg.tf`, `runner-userdata.sh.tftpl` | Runner launch template, Auto Scaling group, first-boot SSM config |
| `infra/backend.hcl.example`, `infra/bootstrap/*` (5 files, incl. lockfile) | S3 state backend bootstrap |
| `k8s/README.md`, `k8s/base/*` (6 manifests) | Kustomize base for the API in k3s |
| `scripts/k3s/{deploy,push-ssm,sync-secrets}.sh` | Deploy with rollback, stage SSM parameters, SSM to k8s Secrets |
| `scripts/test-runner-loss.sh` | Runner-loss exit test |
### Modified
`.env.example`, `.gitignore`, `Makefile`, `README.md` (cost table), `ansible/roles/runner_host/tasks/main.yml`, `ansible/site.yml`, `docker-compose.yml`, `docs/FLOW.md`, `docs/PROGRESS.md`, `infra/aws/{README.md,main.tf,outputs.tf,versions.tf}`, `infra/neon/versions.tf`, `packer/runner.pkr.hcl`, `packer/scripts/trivy-scan.sh`, `scripts/arena.sh`, `scripts/test-destroy-isolation.sh`, `storage/storage.go`, `storage/storage_test.go`.
### Deleted
`k8s/.gitkeep` (directory now has content).

## Known issues and deferred work
- Not applied: SSM push, Neon plan (needs API key and project id), `infra/aws` apply, GHCR push, loss test, AMI (attempt 4).
- ASG does not drain in-flight jobs (reclaimed by `XAUTOCLAIM`); EC2-only health checks; `runner_ami_id` defaults to stock Ubuntu.
- Neon password was exposed and must be rotated before the SSM push.

## Stats (to date)
50 files changed, +1518 / -82; 24 commits excluding merges. Regenerate after the final commit.

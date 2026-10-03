# Phase 13 summary: Cloud deployment

## TL;DR
- All the code to run LeetForce in AWS now exists: S3 test storage, k3s manifests for the API, a CI deploy pipeline, and an Auto Scaling group of runners that configure themselves from SSM at first boot.
- **None of it has been applied.** The cloud run, the runner-loss test and the AMI build are not demonstrated, because they are billable and the AMI build keeps failing on base-image security findings.
- The phase is closed as partial by the owner's request; the remaining steps are recorded as deferred work.

## Where this phase fits
```
browser -> API (k3s, built) -> Redis (Upstash) -> runner (ASG, built, not applied) -> nsjail -> verdict -> API -> browser
                 CI deploy (built, not run)      test data: S3 bucket (built, not applied)
```
- Depends on Phase 12 (Terraform, Packer, Ansible); unblocks Phase 14 (Contests), which does not need the cloud.

## What I built and why
- **S3 storage, remote state** (`feat/13-s3-storage`, `feat/13-s3-backend`): one bucket for test bundles (ADR 0021); Terraform state in S3 so it survives the PC.
- **k3s control host** (`feat/13-k3s-control`): API image (distroless, non-root), Kustomize manifests, secrets synced from SSM. Runners stay as systemd services outside k3s (ADR 0022) because the sandbox needs host privileges.
- **CI deploy** (`feat/13-ci-deploy`): GitHub Actions builds, scans with Trivy, pushes to GHCR, deploys through SSM with an OIDC role limited to one environment.
- **Runner scaling** (`feat/13-runner-scaling`): Auto Scaling group replaces a lost runner; jobs it held are reclaimed by Redis `XAUTOCLAIM`.
- **Loss test** (`test/13-runner-loss`): terminates a runner mid-job and checks one verdict still arrives. Never run.
- **AMI fixes**: Trivy timeout 30m, then apt upgrade and snap refresh (untested).

## Key concepts
- **Auto Scaling group:** AWS keeps N instances alive and replaces dead ones.
- **OIDC role:** GitHub proves its identity to AWS, so no long-lived keys sit in CI.
- **SSM Parameter Store:** where secrets live in AWS; hosts read them at boot.

## Try it yourself
`terraform -chdir=infra/aws validate` and `kubectl kustomize k8s/base` work without credentials. Everything else needs the owner's approval to bill.

## Trade-offs and risks
- The ASG does not drain jobs and checks only EC2 health. The control host's IMDS hop limit 2 lets a compromised API pod reach the instance role.

## Review questions
1. Why do runners stay outside k3s?
2. What happens to a job when its runner is terminated mid-judge?
3. Why does the CI role trust only the `production` environment?

## Review Q&A
Skipped by the owner ("yes" to the merge decision). Understanding questions 1-3 unanswered. Decision: the owner accepted merging with the cloud exit criteria unproven; AMI attempt 4 and the AWS apply move to a later session. Milestone tag M4 not applied because the cloud run is not demonstrated.

## Open decisions
- Apply to AWS (billable), AMI attempt 4, rotate the Neon password, Neon API key and project id.

## Handoff
- **State:** branch `phase/13-cloud-deployment`; no cloud resources exist except the S3 state bucket and the data bucket.
- **Next phase:** 14 - Contests (contest model, timed windows, scoring).
- **Next session prompt:** Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-13-summary.md, then start Phase 14 (Contests). Ask me the recap question and show me the session plan before writing any code.

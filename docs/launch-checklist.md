# LeetForce launch checklist (Phase 16)

Status key: OPEN = needs doing, ASK = needs an owner decision (billable or irreversible), DONE = evidence given.

## A. Phase 13 leftovers (cloud exit criteria unproven)

| # | Item | Status | Cost / risk | Needs from owner |
|---|---|---|---|---|
| A1 | AMI build, attempt 4 (`make build-ami`; fix `7a415f1` untested) | ASK | Billable: temporary EC2 build instance | Confirm the build |
| A2 | `terraform plan` for `infra/neon` and `infra/aws` (never demonstrated; Phase 12 decision A) | ASK | Free, but needs AWS and Neon credentials | Provide credentials in the shell, confirm |
| A3 | `terraform apply` of `infra/aws` (k3s control host, runner ASG) | ASK | Billable, recurring: EC2, EBS, public IPv4 | Explicit "apply" in chat |
| A4 | Runner-loss test (`make test-runner-loss`, `scripts/test-runner-loss.sh`), needs A3 | ASK | Billable while hosts run | Confirm after A3 |
| A5 | Tag `M4` (Phase 13 completed without it); only after A3 and A4 pass | OPEN | None | Decide: tag after A4, or accept M4 as "code complete" and record it |
| A6 | Rotate the Neon password; update `.env` and SSM | ASK | Breaks running services until updated | Do it in the Neon console or give API key and project id |
| A7 | Neon API key and project id for the Neon Terraform plan | ASK | Secret handling | Provide via env, never committed |
| A8 | SSM push of secrets and GHCR image push | ASK | Writes to AWS and GHCR | Confirm |

## B. Carried-over unverified items from earlier phases

| # | Item | Status |
|---|---|---|
| B1 | Pages from Phases 7, 9 and the Phase 11 Grafana dashboard have not been seen in a browser by the owner | OPEN |
| B2 | Upstash plan limit vs about 216k Redis commands a month from queue sampling (Phase 11 decision B) | OPEN |
| B3 | Neon plan limits and PITR window unchecked (also needed for the backup runbook RPO) | OPEN |
| B4 | AMI Go 1.22 (Ubuntu `golang-go`) vs the newer Go on the dev host | OPEN |
| B5 | Ansible playbook has never run on a real host | OPEN |
| B6 | Dev host is up and billable; the PC runs the stack because the dev host disk was 96% full | ASK |
| B7 | Unanswered review questions for Phases 4 to 13 and Phase 14 to 16 (skipped by owner) | Recorded as "skipped by owner" |

## C. Phase 16 deliverables (filled in as they land)

| Item | Status | Evidence |
|---|---|---|
| Security review, findings fixed or accepted | OPEN | `docs/security-review.md` |
| Load test tool and a run against the local stack | OPEN | `tools/loadtest`, `docs/phases/phase-16-log.md` |
| Backup/restore drill against a scratch Neon branch | ASK | `docs/runbook-backup-restore.md` |
| README with cost table, `docs/cost-review.md`, FLOW pass | OPEN | |
| Combined test gate (after Phases 14 and 15 are merged) | OPEN | `docs/phases/phase-16-log.md` |

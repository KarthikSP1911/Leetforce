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
| B2 | Upstash plan limit vs Redis commands: the free tier ran out under idle polling (about 2.7M a month); ADR 0028 cut it to about 0.27M a month for one API and one runner. Check the plan limit and the Upstash usage page after a day | OPEN |
| B3 | Neon plan limits and PITR window unchecked (also needed for the backup runbook RPO) | OPEN |
| B4 | AMI Go 1.22 (Ubuntu `golang-go`) vs the newer Go on the dev host | OPEN |
| B5 | Ansible playbook has never run on a real host | OPEN |
| B6 | Dev host is up and billable; the PC runs the stack because the dev host disk was 96% full | ASK |
| B7 | Unanswered review questions for Phases 4 to 13 and Phase 14 to 16 (skipped by owner) | Recorded as "skipped by owner" |

## C. Phase 16 deliverables

| Item | Status | Evidence |
|---|---|---|
| Security review, findings fixed or accepted | DONE | `docs/security-review.md`: 18 findings, 7 fixed, 11 accepted in writing; SEC-06 (TLS in front of the API) and SEC-11 (protect `main`) need the owner before a public launch |
| Load test tool and a run against the local stack | DONE | `tools/loadtest`, `make test-loadtest-local`: 6 users x 2 iterations in each of mixed and contest mode, 24 of 24 submissions reached a verdict, 0 errors, 0 rate limited; time to verdict p50 about 8 s on a 1 vCPU, 911 MB host (see the log) |
| Backup/restore drill | DONE for the scratch database (PASS); Neon branch mode NOT run; S3 drill PASS but vacuous (empty bucket) | `docs/runbook-backup-restore.md` results table |
| README with architecture and cost table, `docs/cost-review.md`, FLOW pass | DONE | cost figures are UNVERIFIED estimates (see the cost review) |
| Combined test gate (Phases 14 and 15 merged) | see `docs/phases/phase-16-log.md` | final run recorded there |
| Phase 14 and 15 review Q&A | Skipped by the owner | recorded in each summary |

## D. Still open for the owner before a public launch

1. TLS in front of the API (SEC-06), which may add recurring cost.
2. Protect `main` and require reviewers on the `production` environment (SEC-11).
3. All of section A (the cloud has not been applied; M4 stays untagged).
4. Upload the test bundles to the AWS bucket and re-run the S3 drill (the bucket is empty).
5. Read the Neon restore window and plan limits in the console and fill them into the runbook.
6. Rotate the Neon password that was pasted into chat in Phase 13.
7. Delete the stale test stack on the dev host that predates this session (an API on port 18085 and a runner, up for hours); it answered a load test on the wrong build and is not mine to stop.

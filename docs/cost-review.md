# LeetForce cost review

Written in Phase 16 from the repository, the ADRs and the phase logs. Nothing was queried from AWS, Neon or Upstash, so "running now" is the last state recorded in the logs, not a live check. The owner should confirm it in the AWS console (EC2, EBS, Elastic IPs, AMIs and snapshots, S3) before relying on this page.

## Pricing basis (UNVERIFIED)

No dollar figure exists anywhere in the repo before this page; earlier phases deliberately left them out. The numbers here are my recollection of AWS public list prices for `ap-south-1` (Mumbai), on-demand, 730 hours a month. They are assumptions, not bill data:

| Item | Assumed price |
|---|---|
| `t3.micro` | $0.0104 per hour |
| `t3.small` | $0.0208 per hour |
| EBS gp3 | $0.0912 per GB-month (15 GiB: about $1.37) |
| Public IPv4 address, in use | $0.005 per hour (about $3.65 a month) |
| EBS snapshot | $0.05 per GB-month |
| S3 Standard | $0.025 per GB-month (the figure ADR 0021 uses) plus per-request charges |
| SSM Parameter Store standard, SSM Run Command, IAM, security groups, OIDC provider, S3 lock file | no charge |

Not priced because nothing in the repo or logs gives a basis: Neon plan, Upstash plan, GitHub package and Actions allowances, data transfer out of AWS.

## Recurring costs, what each is, and how to reduce it

| # | Cost | Exists today? | Per month if left running (UNVERIFIED) | Owner action to reduce |
|---|---|---|---|---|
| 1 | Dev host `leetforce-dev`, `t3.micro` | Yes. The only instance seen by `describe-instances` in the Phase 13 log; PROGRESS and the logs repeatedly note "dev host still up and billable" | $7.59 | Stop it when idle (EC2 console or `aws ec2 stop-instances`). Billing for the instance and the public IPv4 stops; EBS continues. The IP changes on start, so update `HostName` in `~/.ssh/config`. Terminate it only if the host can be rebuilt with `scripts/setup-dev-host.sh`. |
| 2 | Dev host EBS gp3, 15 GiB | Yes | $1.37 (also while stopped) | Only termination removes it. Take a snapshot first if the host state matters. |
| 3 | Dev host public IPv4 | Yes while running (auto-assigned, released on stop) | $3.65 | Stop the host. Do not attach an Elastic IP unless needed; an unattached one is billed. |
| 4 | Control host `leetforce-control`, `t3.small` | No. Code in `infra/aws/main.tf`; `terraform plan` shows 34 to add, nothing applied | $15.18 | Create only for a test session with `scripts/arena.sh up`, then `arena.sh down`. |
| 5 | Control host EBS (15 GiB) and public IPv4 | No | $1.37 + $3.65 | Removed by `arena.sh down`. |
| 6 | Runner hosts, `t3.small` x `runner_count` (default 1), in an Auto Scaling group | No. `infra/aws/runner-asg.tf` | $15.18 each, plus $1.37 EBS and $3.65 IPv4 each | Keep `runner_count` at 1 (or 0) outside tests. The group has no scaling, so every instance runs until `arena.sh down`. Terminating one instance by hand only makes the group replace it. |
| 7 | Runner AMI snapshot | No. Three build attempts failed and a fourth was not run; no AMI was registered | up to $0.75 for a full 15 GiB | After a successful build keep one AMI; deregister older ones and delete their snapshots. |
| 8 | AMI builder, `t3.small` for about 15 to 18 minutes per attempt | No (terminated by Packer each time) | about $0.005 per attempt, plus a short IPv4 and volume charge. The failed attempts cost about that each | Build only after a fix is ready; confirm no `packer` instance is left running after a failure. |
| 9 | S3 bucket `leetforce-<account id>-data` | Yes (applied in Phase 13) | well under $1: a few MB of bundles and state | Nothing needed. Old `tfstate/` versions expire after 90 days; `problems/` is kept on purpose (rejudges). Do not run `terraform destroy` in `infra/bootstrap`: `prevent_destroy` is set because it holds the state of the other stacks. |
| 10 | Neon Postgres (`ap-southeast-1`) | Yes | UNVERIFIED. The plan and limits were never checked (open since Phase 4). `infra/neon/variables.tf` assumes the free plan (a 6 h restore window is its maximum) | Check the plan in the Neon console. Keep the reaper interval long (default 15 minutes; each sweep wakes the compute). Back-up and restore are Phase 16 work. |
| 11 | Upstash Redis | Yes | UNVERIFIED. Plan and limit never checked. Idle polling was about 90k commands a day for one API and one runner (about 2.7M a month), which exhausted the free tier; ADR 0028 cut it to about 9k a day (about 0.27M a month, computed, not measured). Submissions, rate limits and Run state add more per request | Check the plan's monthly command budget and compare the Upstash usage page after a day of running. Keep `LEETFORCE_METRICS_QUEUE_EVERY` at 5 m or longer and `LEETFORCE_JOB_RECLAIM_EVERY` at 60 s or longer. Stop the API and runners when idle. |
| 12 | GHCR images (private) | No. Nothing pushed (no deploy has run) | UNVERIFIED allowance | Check the package storage and transfer allowance before pushing many tags. Keep the package private (it contains the hidden tests). |
| 13 | GitHub Actions minutes for `deploy.yml` | No run yet | UNVERIFIED allowance | The workflow runs on push to deploy; check the repo's free minutes. |
| 14 | Data transfer out of AWS | Negligible | not estimated | None needed at current traffic. |

## What is running now versus what exists only as code

Based on the Phase 13 log and PROGRESS (last recorded state; confirm in the console):

| Running or existing | Code only (not applied, not built) |
|---|---|
| EC2 `leetforce-dev` (`t3.micro`, ap-south-1) with its 15 GiB gp3 volume and public IPv4 | `infra/aws`: control host, runner Auto Scaling group and launch template, security groups, IAM roles, the GitHub OIDC provider and deploy role (34 resources to add) |
| S3 bucket `leetforce-<account id>-data` (7 resources from `infra/bootstrap`) | Runner AMI (`packer/`; three failed builds, fix `7a415f1` untested) |
| Neon project (outside AWS) | k3s on the control host, `k8s/base` manifests, the API image in GHCR |
| Upstash Redis database (outside AWS) | SSM parameters under `/leetforce/*` (`scripts/k3s/push-ssm.sh` was not applied) |
| IAM user `leetforce-terraform` with AdministratorAccess and an access key (no charge, but narrow it in Phase 16) | `infra/neon` Terraform state (plan not run: needs `NEON_API_KEY` and the project id) |
| Local observability stack runs on the PC only on demand (`make dev-obs`), no cloud cost | `make test-runner-loss` and the M4 milestone (not run, not tagged) |

## Cost if left running a month (UNVERIFIED estimate)

Assumptions: the prices above; 730 hours; the dev host running all month; the arena (`infra/aws` applied with `runner_count = 1`, one control host and one runner host) running all month; one AMI of 15 GiB; negligible S3; Neon and Upstash on plans that cost nothing (not verified, see rows 10 and 11); no data-transfer charge.

| Scenario | Per month | Per day |
|---|---|---|
| Today's state, dev host running, nothing else billable (dev + EBS + IPv4 + S3) | about $12.6 to $12.7 | about $0.42 |
| Today's state, dev host stopped (EBS and S3 only) | about $1.4 | about $0.05 |
| Arena only: control + 1 runner, each $15.18 + $1.37 + $3.65 | about $40.4 | about $1.33 (about $0.052 per hour) |
| Arena with `runner_count = 2` | about $60.6 | about $2.0 |
| Everything on: dev host + arena (1 runner) + AMI snapshot + S3 | about $53.8 | about $1.77 |

Add Neon and Upstash if their plans are not free; those are the two largest unknowns. A `t3` instance in unlimited CPU-credit mode (the default) adds a surplus-credit charge, roughly $0.05 per vCPU-hour in some regions (UNVERIFIED), when a runner stays above its baseline for long; sustained judging load or the load test in Phase 16 can do this, so it is not included above.

## Shut down when idle

1. `scripts/arena.sh down` (typed confirmation) when a cloud test session ends. It destroys `infra/aws` only; `infra/neon` and `infra/bootstrap` are separate stacks.
2. Stop `leetforce-dev` when not working on it (keeps EBS, drops instance and IPv4 charges).
3. Check for leftovers after any Packer failure: a running builder instance, a temporary security group or key pair, and snapshots of unused AMIs.
4. Look for unattached Elastic IPs and unattached EBS volumes (none are expected; confirm).
5. Stop the local API and runner when not in use so the Redis polling loops (about 9k commands a day for an API and a runner, ADR 0028) and the 15-minute reaper stop consuming Upstash commands and waking Neon.
6. Stop the local observability stack (`make down-obs`) and the SSH tunnel; they run on the PC and cost nothing in the cloud but keep polling the dev host.
7. Deregister old AMIs and delete their snapshots; keep at most one.

## Open items for the owner

- Confirm in the console that only `leetforce-dev` is running and nothing from the failed AMI builds remains.
- Confirm the Neon plan and its compute-hour and storage limits (open since Phase 4).
- Confirm the Upstash plan and its monthly command limit (open since Phase 4).
- Replace the UNVERIFIED price assumptions with the figures from the AWS pricing pages or Cost Explorer, and set an AWS budget alert (no budget alert is recorded anywhere in the repo).
- Rotate the Neon password that was pasted into chat in Phase 13 (not confirmed done).

<p align="center">
  <img src="web/public/brand/logo-mark.svg" alt="LeetForce logo" width="120">
</p>

<h1 align="center">LeetForce</h1>

<p align="center">
  A distributed, sandboxed code execution platform. Submit Python, C++, Java, or Go and get a verdict with runtime and memory.
</p>

## Cost table

Recurring costs that are billed while the resource exists or runs. Prices depend on the AWS region; check the AWS pricing pages before relying on a number.

| Resource | Introduced | Billing | Notes |
|---|---|---|---|
| EC2 `t3.micro` dev host (`leetforce-dev`) | Phase 1 | Per hour while running | Stop it when idle. t3 defaults to unlimited CPU-credit mode, so sustained CPU can add surplus-credit charges. See [ADR 0003](docs/adr/0003-dev-environment-ec2-x86.md). |
| EBS gp3 volume, 15 GiB | Phase 1 | Per GB-month, also while the instance is stopped | Deleted only when the instance is terminated. |
| Public IPv4 address | Phase 1 | Per hour while attached (AWS charges for public IPv4) | The address changes on stop/start unless an Elastic IP is attached; an unattached Elastic IP is also billed. |
| EC2 `t3.small` control host (`leetforce-control`) | Phase 12 | Per hour while it exists and runs | Created only by `scripts/arena.sh up` (typed confirmation). Instance type is `control_instance_type` in `infra/aws`. Destroy with `arena.sh down`. |
| EC2 `t3.small` runner hosts (`leetforce-runner-N`) | Phase 12 | Per hour each, times `runner_count` (default 1) | Same stack, same confirmation. t3 unlimited CPU credits can add surplus charges under sustained load, as with the dev host. |
| EBS gp3 volume, 15 GiB, encrypted, per new host | Phase 12 | Per GB-month each | Deleted with the instance (`delete_on_termination`). Adds to the dev host volume above. |
| Public IPv4 address per new host | Phase 12 | Per hour each while attached | Needed because there is no NAT gateway (cost rule); each host in `infra/aws` has one. |
| Runner AMI snapshot (Packer) | Phase 12 | Per GB-month of snapshot storage while the AMI is registered | `make build-ami` also bills a `t3.small` builder plus its volume for roughly 15 minutes per build. Deregister old AMIs and delete their snapshots. |
| Neon Postgres project | Phase 4 (adopted by Terraform in Phase 12) | Depends on the Neon plan | Plan and limits not yet checked (open item). `infra/neon` creates nothing new. |
| S3 bucket `leetforce-<account id>-data` (single bucket, versioned, encrypted) | Phase 13 | Per GB-month stored plus per request | Created in Phase 13 (`infra/bootstrap`, approved by the owner). Holds `problems/` test bundles and `tfstate/` Terraform state; a few MB, so well under a dollar a month. Old test bundles are kept (rejudges); old state versions expire after 90 days (ADR 0021). |
| Auto Scaling group for runners | Phase 13 | No charge for the group; the instances are the `t3.small` runner rows above | Replaces a lost runner so the fleet stays at `runner_count` until `arena.sh down`. Not applied yet. |
| Container images in GHCR (private) | Phase 13 | Free within GitHub's package storage and transfer allowance | Check the allowance before pushing many tags. The API image holds the hidden tests, so it must stay private. |

Not billable: SSM Parameter Store standard parameters, SSM Run Command, GitHub Actions OIDC, IAM roles and security groups, and the Terraform state lock (the S3 lock file, no DynamoDB table). No VPC endpoints or NAT gateway are used. As of Phase 13 work only the S3 bucket exists; nothing else in `infra/aws` or `packer/` has been applied or built, and the rows above apply from the first `arena.sh up` or successful `make build-ami`.

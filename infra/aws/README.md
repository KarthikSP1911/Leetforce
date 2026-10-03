# infra/aws

Terraform for the LeetForce cloud hosts, in the account's default VPC (no custom VPC, endpoints or NAT).

- One control host (k3s server) and `runner_count` runner hosts, security groups open to `owner_cidr` only, IMDSv2, encrypted gp3 volumes.
- IAM: runners may read `/leetforce/runner/*` in SSM only; the database URL lives under `/leetforce/api/*` and never reaches a runner.
- Local state, separate from `infra/neon`. Use `scripts/arena.sh` rather than raw `apply`/`destroy`.

```bash
cp terraform.tfvars.example terraform.tfvars   # set owner_cidr and key_name
terraform init && terraform plan               # needs AWS credentials; plan creates nothing
```

Billable once applied; see the cost table in the root README.

## Runner fleet

Runners are an Auto Scaling group (`runner-asg.tf`) fixed at `runner_count` instances (min = max = desired). It replaces an instance that is terminated, stopped or fails its EC2 status checks; it does not scale on load, and EC2 health checks do not notice a crashed `leetforce-runner` service (systemd restarts it on failure). A changed AMI or user data rolls the fleet through an instance refresh. `runner_ami_id` must be the Packer AMI: on stock Ubuntu there is no runner service and first boot fails.

On first boot `runner-userdata.sh.tftpl` reads the parameters below with the instance role, writes `/etc/leetforce/runner.env` (0640 root:lfrunner), then enables and restarts `leetforce-runner`. It exits nonzero (see `/var/log/cloud-init-output.log`) when a required parameter is missing. Values are never printed.

SSM parameters under `/leetforce/runner/` (SecureString; the name is the environment variable). `scripts/k3s/push-ssm.sh` writes the required four.

| Name | Required | Notes |
|---|---|---|
| `LEETFORCE_REDIS_URL` | yes | `rediss://` Upstash URL |
| `LEETFORCE_S3_ENDPOINT` | yes | for example `s3.ap-south-1.amazonaws.com` |
| `LEETFORCE_S3_BUCKET` | yes | `leetforce-<account id>-data` |
| `LEETFORCE_S3_USE_TLS` | yes | `true` |
| `LEETFORCE_QUEUE_PREFIX` | no | default `leetforce` |
| `LEETFORCE_JOB_MIN_IDLE`, `LEETFORCE_JOB_MAX_ATTEMPTS` | no | reclaim idle time, delivery limit |

No access keys: S3 uses the instance role. Runners never receive `DATABASE_URL`.

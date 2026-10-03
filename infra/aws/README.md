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

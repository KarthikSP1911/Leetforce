# ADR 0020: Infrastructure as code layout (two Terraform stacks, local state, Packer plus Ansible)

Status: accepted (Phase 12). Decisions 1, 2 and 4 were made by the owner in chat ("import it, local, ok docker"); 3 and the rest are Claude defaults the owner accepted ("ok").

## Context
Phase 12 must describe the Neon database and the AWS hosts as code without applying anything. Constraints: the Neon project already holds the data; AWS uses the default VPC only (no custom VPC, endpoints or NAT); `terraform destroy` on `infra/aws` must never touch `infra/neon`; nothing billable runs without confirmation; Packer and Ansible are not installed on the Windows PC.

## Decision
1. **Two stacks, two states.** `infra/neon` and `infra/aws` are separate Terraform roots with separate providers, variables and state files. A destroy in one cannot see the other's resources. `scripts/test-destroy-isolation.sh` checks this offline (no neon provider or resource in `infra/aws`, `prevent_destroy` on the Neon project, `arena.sh` never names `infra/neon`).
2. **Import the Neon project** (owner decision) with an `import` block and `prevent_destroy = true`. Branches, roles, databases and the schema stay with the Neon console and goose migrations; only the project and its history window are declared.
3. **Local state**, git-ignored (owner decision). No S3 bucket or lock table, so no recurring cost. Losing `infra/neon` state only means re-importing; losing `infra/aws` state while hosts exist would orphan them, so the state file must be backed up before `arena.sh up`.
4. **Packer builds the runner AMI; Ansible does the provisioning.** Packer runs the Ansible roles with `ansible-local` on the builder instance, so no Ansible is needed on Windows, then installs the runner with the existing `scripts/runner/install-runner.sh` (one install path) and gates the image with Trivy. Packer, Ansible lint and Terraform validate run in Docker or with the local Terraform binary (owner decision).
5. **Least privilege in IAM.** Runners can read only `/leetforce/runner/*` in SSM; the database URL belongs under `/leetforce/api/*`, so a compromised runner host cannot read it (CLAUDE.md: runners never connect to the database).
6. **Runner network posture.** Inbound: SSH and the k3s API from `owner_cidr` only; metrics stay on localhost behind the SSH tunnel. Egress is limited by port, but the CIDR stays `0.0.0.0/0` because Upstash, Neon and S3 have no fixed ranges and NAT or endpoints are excluded by the cost rule.
7. **x86 `t3` hosts in `ap-south-1`**, matching ADR 0003 and the dev host.

## Alternatives
- **Create a new Neon project and migrate:** cleaner state, but risks the data and needs a migration for no gain.
- **S3 backend with locking:** safer for teams; a recurring cost and a bootstrap problem for a single owner. Revisit in Phase 13 or 16.
- **Terraform workspaces in one stack:** one `destroy` would target the whole state, which is exactly what the exit criterion forbids.
- **Ansible-only (no AMI):** slower boots and drift between runners; the AMI makes runner scaling in Phase 13 a launch, not a provision.
- **Custom VPC with private subnets and NAT:** better isolation, about the price of an extra instance per month; excluded by the cost rule.

## Consequences
- `terraform plan` can only be run with an AWS profile and a Neon API key; both stacks validate and Trivy-scan clean without them.
- Trivy `AWS-0104` (open egress CIDR) is accepted in `.trivyignore` until 2027-01-03; Phase 13 revisits it.
- The AMI carries Ubuntu's `golang-go` (Go 1.22), not the newest Go the dev host uses; judged Go solutions only need the compiler, but a mismatch with the dev host is a risk to check when the first AMI runner judges a Go submission.
- The runner unit is installed but disabled in the AMI; Phase 13 supplies `/etc/leetforce/runner.env` and enables it.

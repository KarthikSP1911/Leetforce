# Phase 13 log: Cloud deployment

Running log. Secrets, key contents and public IPs are never written here; names only.

## File and path index
| Path | What |
|---|---|
| `infra/aws/terraform.tfvars` | git-ignored; `owner_cidr` (owner's public IP /32) and `key_name` |
| `C:\Users\karth\.aws\{config,credentials}` | outside the repo; profile `leetforce` |

## Session 1

### Decisions (owner answers in chat, 2026-10-03)
- B: S3 backend with locking (small recurring cost, to go in the README cost table). Owner answered.
- C: owner did not understand the question; Claude's default applies: runners stay as systemd services on AMI-based hosts, outside k3s (to be an ADR).
- D: owner said "whichever is convenient"; Claude's default: GitHub Actions OIDC role.
- E: owner OK with `up`, test, `down` in the same session (typed confirmation each time).
- Phase 12 recap question: not answered.

### Steps
1. Owner: created IAM user `leetforce-terraform` (AdministratorAccess, to be narrowed in Phase 16) and an access key; ran `aws configure --profile leetforce` (region `ap-south-1`). Verified with `aws sts get-caller-identity --profile leetforce`: account matches the owner's, user `leetforce-terraform`.
2. Claude: read-only `aws ec2 describe-key-pairs` (key pairs `leetforce`, `ec2-learning-pk`) and `describe-instances` (only `leetforce-dev`, t3.micro, running).
3. Claude: `git checkout -b phase/13-cloud-deployment`, `git tag phase-13-start`.
4. Claude: wrote git-ignored `infra/aws/terraform.tfvars` (owner IP, key `leetforce`). Mistake avoided: `terraform.tfvars.example` names key `leetforce-dev`, which does not exist in the account.
5. Claude: `AWS_PROFILE=leetforce terraform -chdir=infra/aws init` then `plan`: **25 to add, 0 to change, 0 to destroy**; nothing created. (Phase 12 exit criterion for `infra/aws` plan now met.)

### Open
- `infra/neon` plan: needs `NEON_API_KEY` and the Neon project id (`.env` has only `DATABASE_URL`).
- AMI build: waiting for owner's confirmation (billable).

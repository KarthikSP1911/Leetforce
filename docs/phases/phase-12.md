# Phase 12: Infrastructure as code

**Branch:** `phase/12-infra-as-code`
**Range:** `phase-12-start..phase-12-done`
**Dates:** 2026-10-03 to 2026-10-03 (one session)
**Milestone:** none
**Log:** [phase-12-log.md](phase-12-log.md). **Decision record:** [ADR 0020](../adr/0020-infrastructure-as-code.md).

## Summary
The Neon database and the AWS hosts are now described as code in two separate Terraform stacks, with a Packer template for the runner AMI, Ansible roles for hardening, and `scripts/arena.sh` for bringing the AWS stack up and down. Everything validates and passes Trivy without credentials. **Nothing was planned against a real account, applied or built:** no AWS or Neon credentials were available in the session, so two of the three exit criteria are only partly demonstrated (see below).

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| `terraform plan` clean | ⚠️ not demonstrated | Both stacks pass `terraform fmt -check`, `init -backend=false` and `validate` (`make tf-validate`) and `trivy config`. `plan` needs an AWS profile, `owner_cidr`, `key_name` (aws) and `NEON_API_KEY` plus the project id (neon); none were provided. |
| AMI builds | ⚠️ not demonstrated | `make packer-validate` prints "The configuration is valid." The build is billable and needs credentials and the owner's confirmation; it was not run. The cross-compiled runner binary the AMI embeds was built (`bin/runner-linux-amd64`, 31.7 MB). |
| Destroy of `infra/aws` never touches `infra/neon` | ✅ (static) | `make test-destroy-isolation`: 8 PASS, 1 SKIP (`plan -destroy`, needs credentials), `ISOLATION_OK`. Separate roots, providers and state files; `prevent_destroy` on the Neon project; `arena.sh` never names `infra/neon`. |
| Nothing applied without explicit confirmation | ✅ | No `apply`, `destroy`, `arena.sh up` or `packer build` was run. |
| Ansible hardening | ✅ (lint only) | `make lint-ansible`: `ansible-lint` passes at the production profile; `ansible-playbook --syntax-check` passes. The playbook was never run on a host. |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/12-terraform-neon` | `infra/neon`: adopt the Neon project | 1 |
| `feat/12-terraform-aws` | `infra/aws`: default-VPC hosts, security groups, IAM | 1 |
| `feat/12-arena` | `arena.sh` and the destroy-isolation check (recreated after a failed first commit; see log) | 1 |
| `feat/12-ansible` | hardening and runner_host roles | 1 |
| `feat/12-packer-ami` | runner AMI template with Trivy gate | 1 |
| `feat/12-cost-docs` | README cost table, ADR, log, FLOW, PROGRESS | 1 |

## File-by-file changes
Generated with `git diff --name-status phase-12-start..HEAD` at commit `395f83b` (before this report and the summary were added).

### Added
| File | Purpose |
|---|---|
| `infra/neon/versions.tf` | Provider `kislerdm/neon ~> 0.6`, local backend |
| `infra/neon/variables.tf` | `project_id` (required), name, region, Postgres version, history retention |
| `infra/neon/main.tf` | `import` block plus `neon_project` with `prevent_destroy` and `ignore_changes = [branch]` |
| `infra/neon/outputs.tf` | `project_id`; `database_url` as a sensitive output |
| `infra/neon/terraform.tfvars.example`, `infra/neon/README.md` | How to run the stack |
| `infra/neon/.terraform.lock.hcl` | Provider lock file (generated) |
| `infra/aws/versions.tf` | Provider `hashicorp/aws ~> 6.0`, local backend, default tags |
| `infra/aws/variables.tf` | Region, `owner_cidr` (validated `/32`), key name, instance types, runner count, AMI, SSM prefix |
| `infra/aws/main.tf` | Default VPC data, security groups and rules, per-port egress, IAM roles and instance profiles, control and runner instances |
| `infra/aws/outputs.tf` | Public IPs and the runner security group id |
| `infra/aws/terraform.tfvars.example`, `infra/aws/README.md` | How to run the stack |
| `infra/aws/.terraform.lock.hcl` | Provider lock file (generated) |
| `.trivyignore` | `AWS-0104` accepted, with reason, until 2027-01-03 |
| `scripts/arena.sh` | `up`, `down`, `status` for `infra/aws` only, typed confirmations |
| `scripts/test-destroy-isolation.sh` | Offline exit check for destroy isolation |
| `ansible/site.yml`, `ansible/ansible.cfg`, `ansible/requirements.yml` | Playbook, config, collections |
| `ansible/inventory/hosts.ini.example` | Inventory template (real file is git-ignored) |
| `ansible/roles/hardening/tasks/main.yml`, `ansible/roles/hardening/handlers/main.yml` | SSH, sysctl, ufw, unattended upgrades, auditd |
| `ansible/roles/runner_host/tasks/main.yml`, `ansible/roles/runner_host/defaults/main.yml` | Toolchains, nsjail at the pinned commit, `lfrunner` user |
| `packer/runner.pkr.hcl` | Runner AMI: Ubuntu 24.04, Ansible, runner service, Trivy gate, encrypted, IMDSv2 |
| `packer/scripts/trivy-scan.sh`, `packer/scripts/cleanup.sh` | Image scan and pre-snapshot cleanup |
| `docs/adr/0020-infrastructure-as-code.md`, `docs/phases/phase-12-log.md` | Decision record and running log |

### Modified
| File | What changed | Why |
|---|---|---|
| `Makefile` | Targets `tf-validate`, `test-destroy-isolation`, `lint-ansible`, `build-runner-linux`, `packer-validate`, `build-ami` | One command per check; `build-ami` is marked billable |
| `.gitignore` | Ignore `ansible/inventory/hosts.ini` and `packer/packer-manifest.json` | Keep host addresses and build output out of git |
| `README.md` | Six new cost-table rows and a not-billable note | CLAUDE.md: record recurring costs when introduced |
| `docs/FLOW.md` | Phase 12 "as built" section (marked in progress) | CLAUDE.md: update FLOW every phase |
| `docs/PROGRESS.md` | Phase 12 row and status | Phase tracker |

### Deleted
| File | Reason |
|---|---|
| `infra/neon/.gitkeep`, `infra/aws/.gitkeep`, `ansible/.gitkeep`, `packer/.gitkeep` | Directories now hold real files |

### Renamed / moved
None.

## Key code changes
- **Import, not create** (`infra/neon/main.tf`): an `import` block adopts the project that already holds the data; `prevent_destroy = true` makes `terraform destroy` fail. The first plan will show "1 to import".
- **Runner IAM is narrower than the control host's** (`infra/aws/main.tf`): runners read `…/parameter/leetforce/runner/*` only. The database URL goes under `/leetforce/api/*`, which a runner role cannot read.
- **Runner metadata hop limit 1** (`infra/aws/main.tf`): with IMDSv2 required and hop limit 1, a packet from a nested container cannot reach the instance metadata service, so sandboxed code cannot steal the role credentials that way.
- **`arena.sh down`** (`scripts/arena.sh`): prints `terraform plan -destroy`, then needs the exact phrase `destroy leetforce-aws`; it only ever calls Terraform with `-chdir=infra/aws`.
- **Packer reuses the existing installer**: after `ansible-local`, the build calls `scripts/runner/install-runner.sh`, so the AMI and the dev host share one install path; the service is installed but disabled.

## Decisions
- [ADR 0020](../adr/0020-infrastructure-as-code.md): two stacks and two states, import the Neon project, local state, Packer plus `ansible-local`, least-privilege SSM, per-port egress with an accepted Trivy finding.

## Tests
- `make test-destroy-isolation` (new, offline): 8 checks plus an optional `plan -destroy` check.
- `make tf-validate`, `make packer-validate`, `make lint-ansible`; `trivy config` on `infra`, `packer`, `ansible` with `.trivyignore`: exit 0.
- No Go code changed, so `make test` and `make test-adversarial` were not run (`git diff --stat phase-12-start..HEAD -- judge runner api queue storage` is empty).

## Known issues and deferred work
- `terraform plan` (both stacks) and the AMI build need credentials and confirmation: to be done when the owner provides them; Phase 13 depends on both.
- The Ansible playbook has never run on a host; the first AMI build is its first real test (for example the `ufw` and `sshd` tasks, and the build of nsjail on a `t3.small`).
- AMI Go is Ubuntu's 1.22; the dev host uses a newer Go. Check a Go submission on the first AMI runner (Phase 13).
- Trivy `AWS-0104` accepted until 2027-01-03; revisit in Phase 13.
- State is local: back up `infra/aws/terraform.tfstate` before the first `arena.sh up`.
- Neon plan and limits still unchecked.

## Stats
- Commits: 6 (excluding merges) up to `3a53481`; this report and the summary add two more `docs` commits
- Files: 30 added, 5 modified, 4 deleted (at `395f83b`, before the report files)
- Lines: +1136 / -5 (same point)

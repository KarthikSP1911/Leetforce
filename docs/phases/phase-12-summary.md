# Phase 12 summary: Infrastructure as code

## TL;DR
- The places LeetForce will run (the Neon database and the AWS servers) are now written down as code, so they can be rebuilt or removed with a command instead of clicking in a console.
- A runner "golden image" (an AMI) is described in a Packer file, and an Ansible playbook hardens every server (no passwords for SSH, a firewall, automatic security updates).
- **Nothing was created.** No cloud credentials were available, so I checked everything offline (syntax, lint, security scan) but did not run `terraform plan` or build the image. Those two exit criteria are still open.

## Where this phase fits
```
 browser --> API (P4, P9) --> Redis queue (P3) --> runner (P3) --> sandbox (P1, P2, P6) --> verdict --> browser
    all of it measured by Prometheus/Grafana/Loki (P11)
 [P12 NEW, described as code, not yet created]
   Neon database  <-- infra/neon (Terraform, adopts the existing project)
   control host + runner hosts  <-- infra/aws (Terraform)  <-- scripts/arena.sh up|down|status
   runner image   <-- packer/runner.pkr.hcl  (Ubuntu + Ansible hardening + nsjail + runner + Trivy scan)
 Still to come: running it all in the cloud (P13), contests (P14-15), launch checks (P16)
```
- It depends on Phase 1 (nsjail version, runner install script), Phase 6 (unprivileged runner unit) and Phase 11 (metrics stay on localhost).
- It unblocks Phase 13: k3s and runner scaling need hosts and an image that already exist as code.

## What I built and why
### Neon stack (`feat/12-terraform-neon`)
- **What:** `infra/neon` tells Terraform about your existing Neon project.
- **Why:** so the database is part of the written-down system, without risking the data.
- **How it works:** an `import` block "adopts" the existing project instead of creating a new one; `prevent_destroy` makes a destroy fail. Schema stays with goose migrations.
- **Alternatives:** a new project plus data migration (risky for no gain). See [ADR 0020](../adr/0020-infrastructure-as-code.md).

### AWS stack (`feat/12-terraform-aws`)
- **What:** `infra/aws` defines one control host, `runner_count` runner hosts, security groups and IAM roles in the account's default network.
- **Why:** repeatable servers with the least access they need.
- **How it works:** only your IP can reach SSH and the k3s API; servers can only call out on a short list of ports; runners can read only `/leetforce/runner/*` settings, never the database URL; each server needs the newer "IMDSv2" token to read its own metadata.
- **Alternatives:** a custom private network with NAT (costs more; ruled out by the cost rule).

### `arena.sh` and the isolation check (`feat/12-arena`)
- **What:** `scripts/arena.sh up|down|status`, and `make test-destroy-isolation`.
- **Why:** the rule that destroying the cloud servers must never touch the database.
- **How it works:** the script only ever points Terraform at `infra/aws`, and asks you to type a phrase before creating or destroying. The test checks the two stacks share no provider, no resource and no state file, and that the database project refuses to be destroyed.

### Ansible (`feat/12-ansible`)
- **What:** `ansible/` with a `hardening` role for every host and a `runner_host` role for runners.
- **Why:** a fresh Ubuntu server is not safe or ready to judge code; this makes it so, the same way every time.
- **How it works:** SSH keys only and no root login, kernel network settings, a firewall that denies incoming traffic except rate-limited SSH, security updates, an audit log; on runners, the language toolchains and nsjail built from the exact commit the dev host uses.

### Packer image (`feat/12-packer-ami`)
- **What:** a template that builds the runner AMI (a saved server disk image).
- **Why:** new runners in Phase 13 start in seconds from the image instead of being set up from scratch.
- **How it works:** it launches a temporary server, runs Ansible on it, installs the runner program (disabled until Phase 13 gives it settings), scans the result with Trivy (fails on fixable serious problems), removes keys and leftovers, then snapshots it.

### Cost table and docs (`feat/12-cost-docs`)
- The README now lists every recurring cost these files will cause once applied, plus what is free.

## How it works now, step by step
1. You fill in `infra/aws/terraform.tfvars` (your IP, key pair name) and run `terraform plan` to preview.
2. `make build-ami` (after your confirmation) builds the runner image; its id goes into `runner_ami_id`.
3. `scripts/arena.sh up` shows the plan, you type `up leetforce-aws`, and the hosts appear.
4. `scripts/arena.sh down` shows what will be destroyed, you type `destroy leetforce-aws`, and only `infra/aws` goes away. The database project is in a different stack and is untouched.

## Key concepts
- **Terraform:** describes servers and networks in files and creates, changes or removes them to match. LeetForce uses it so the cloud setup is reviewable and repeatable.
- **State file:** Terraform's memory of what it created. Ours are local files, one per stack, so one stack's destroy cannot see the other.
- **`terraform plan`:** a preview that changes nothing. It is the main safety check.
- **Import:** teaching Terraform about something that already exists.
- **AMI / Packer:** an AMI is a saved server disk image; Packer builds one from a recipe.
- **Ansible:** applies a list of configuration steps to a server, safely repeatable.
- **IMDSv2:** the token-based way a server reads its own cloud metadata; it blocks a common credential-theft trick.
- **IAM role / SSM parameter:** the permissions a server holds, and the cloud's store for settings and secrets.

## Try it yourself
```bash
make tf-validate                  # both Terraform stacks: format + validate, no credentials
make test-destroy-isolation       # expect 8 PASS, 1 SKIP, ISOLATION_OK
make lint-ansible                 # Docker; expect "Passed: 0 failure(s)"
make packer-validate              # Docker; expect "The configuration is valid."
# With credentials (nothing is created by plan):
export NEON_API_KEY=...           # then in infra/neon: copy terraform.tfvars.example, set project_id
terraform -chdir=infra/neon plan  # expect "1 to import"
terraform -chdir=infra/aws plan   # needs AWS credentials and terraform.tfvars
```

## Trade-offs and risks
- Local state files: free and simple, but losing `infra/aws` state while servers exist orphans them. Back it up before the first `up`.
- Servers need public IPv4 addresses because there is no NAT; they are billed hourly and reachable only from your IP.
- Egress is limited by port, not by destination, so a compromised host could still reach any address on those ports; accepted until 2027-01-03 (Trivy `AWS-0104`).
- The Ansible playbook and Packer template were never run for real; the first build may surface fixes.
- The AMI's Go is Ubuntu's 1.22, older than the dev host's.

## Review questions
Understanding:
1. Why are `infra/neon` and `infra/aws` separate stacks with separate state files, rather than one stack?
2. What does `prevent_destroy` do, and what would it not protect against?
3. Why can a runner host read `/leetforce/runner/*` but not the database URL?
4. Why does the AMI install the runner service but leave it disabled?
5. What does `terraform plan` change in your cloud account?

Decisions for you:
- A. Provide credentials (AWS profile, your IP, key pair name, Neon API key and project id) so the two plans can run, and confirm whether to build the AMI now (a few cents to a dollar-ish of compute and snapshot, check pricing), or defer both to the start of Phase 13?
- B. Keep local state, or move to an S3 bucket with locking before Phase 13 (small recurring cost)?

## Review Q&A
Skipped by the owner (in chat, 2026-10-03: chose "Skip review, merge now"). Understanding questions 1-5 and decisions A and B were not answered; nothing was answered on the owner's behalf. Decision B stays on the default (local state). Decision A is carried to Phase 13: `terraform plan` for both stacks and the billable AMI build are still to be demonstrated.

## Open decisions
- A (credentials and AMI build confirmation, first item of Phase 13) and B (state backend; default local). Carried over: earlier review answers in `docs/PROGRESS.md`; `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Handoff
- **State:** branch `phase/12-infra-as-code` merged into `main`, tags `phase-12-start` and `phase-12-done`. No AWS or Neon resource exists because of this phase.
- **Next phase:** 13 - Cloud deployment: k3s manifests, runner scaling, secrets in SSM, CI deploy. It needs the plans and the AMI build from this phase to be done first.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-12-summary.md, then start Phase 13 (Cloud deployment). Ask me the recap question and show me the session plan before writing any code.
  ```

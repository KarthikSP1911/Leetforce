# Phase 12 log: Infrastructure as code

Running log (CLAUDE.md "Documenting every step"). Entries: who, command, why, result, mistakes.

## Session 1

### Start
- Claude read `CLAUDE.md`, `docs/PROGRESS.md`, `docs/phases/phase-11-summary.md` and the Phase 12 section of `docs/PLAN.md`; checked `git status`, branch (`main`), `git log --oneline -5` and `git tag --list 'phase-*'`: all matched `PROGRESS.md` (only `.claude/`, `web/AGENTS.md`, `web/CLAUDE.md` untracked, as recorded).
- Claude asked the Phase 11 recap question and posted the session plan with five decisions. The owner did not answer the recap question. The owner answered (in chat): 1 import the Neon project, 2 local state, 3 "ok" (defaults: x86 `t3` in the dev host's region), 4 Docker for Packer and Ansible. Decision 5 (AWS credentials and a Neon API key for `plan`) was not answered.
- Claude created `phase/12-infra-as-code` from `main` and tagged `phase-12-start`.

### Environment facts found (Claude)
- PC: Terraform v1.15.8 (WinGet), Docker 29.6.2, AWS CLI installed but **no credentials** (`aws sts get-caller-identity` fails with NoCredentials); no Packer, Ansible or Trivy. `.env` holds only `LEETFORCE_REDIS_URL`, `DATABASE_URL`, `LEETFORCE_GRAFANA_PASSWORD`: **no Neon API key, no Neon project id**.
- Dev host (`ssh leetforce-dev`, instance metadata over IMDSv2): region `ap-south-1`, `t3.micro`, x86_64, disk 14 GB at 97% (425 MB free), so it cannot run Packer builds or Docker images. nsjail commit on the host: `4ff54a6e0d5b65a0e1633d6e2fd1425ba6c882ce` (full hash read with `git -C ~/nsjail rev-parse HEAD`).
- Neon: the pooler host in `DATABASE_URL` shows region `aws-ap-southeast-1` (credentials not copied anywhere).

### Unit 1: Neon stack (`feat/12-terraform-neon`, merged)
- `infra/neon/{versions,variables,main,outputs}.tf`, `terraform.tfvars.example`, `README.md`, `.terraform.lock.hcl`. Provider `kislerdm/neon ~> 0.6`; `neon_project.leetforce` adopted by an `import` block (`var.project_id`), `prevent_destroy`, `ignore_changes = [branch]`; `database_url` is a sensitive output. Local backend.
- Mistake: `ignore_changes` first listed `default_endpoint_settings`, which is not an attribute in this provider version; `terraform validate` caught it and it was removed.
- Mistake: tried `docker run hashicorp/terraform sh -c ...`; the image entrypoint is `terraform`. Used the local Terraform binary instead.
- Checked: `terraform fmt -check`, `terraform init -backend=false`, `terraform validate`: valid. Secret scan with `scripts/scan-staged.sh`: clean.

### Unit 2: AWS stack (`feat/12-terraform-aws`, merged)
- `infra/aws/{versions,variables,main,outputs}.tf`, `terraform.tfvars.example`, `README.md`, `.terraform.lock.hcl`, `.trivyignore`. Default VPC and first default subnet; Ubuntu 24.04 AMI from Canonical's public SSM parameter; control and runner security groups (SSH from `owner_cidr`, k3s 6443 from `owner_cidr` and from the runner group); per-port egress; two IAM roles with SSM read policies (runner: `/leetforce/runner/*`, control: `/leetforce/*`); control instance and `runner_count` runners with IMDSv2 required, hop limit 1, encrypted gp3, public IPv4.
- Trivy: `docker run aquasec/trivy config /w/infra` first reported 2 CRITICAL AWS-0104 (egress `0.0.0.0/0`, all protocols). Egress was narrowed to ports 443, 80, 6379, udp 53 and 123 (plus 5432 on the control host), which Trivy still flags per rule (11 findings) because the CIDR is open. Accepted in `.trivyignore` with the reason and expiry 2027-01-03 (ADR 0020, decision 6). After that `trivy config` over `infra`, `packer` and `ansible` returns exit 0.
- Mistake: the first attempt wrote all files with one large heredoc; the shell tool rejected it (apostrophe) before anything ran. Redone with the editor tool; no half-written files.

### Unit 3: arena and the isolation check (`feat/12-arena`, merged)
- `scripts/arena.sh up|down|status` (only `infra/aws`; `up` prints the plan and needs the typed phrase `up leetforce-aws`; `down` prints a destroy plan and needs `destroy leetforce-aws`; both need a terminal). `scripts/test-destroy-isolation.sh` (`make test-destroy-isolation`) and `make tf-validate`.
- Mistake 1: the isolation script's "own state file" check passed vacuously. A Python edit wrote a control character (`\x01`) instead of `\1` into a `sed` expression, so both stacks returned the same garbage. Found by reading the diff shown after the edit, fixed, and the check now prints the real path for each stack.
- Mistake 2: the commit failed commitlint (body line over 72 characters) but my command sequence continued, merged an empty `feat/12-arena` and deleted it; the staged changes were still in the working tree. Recreated `feat/12-arena`, committed with a valid message, merged. Lesson: chain the commit with `&&` before the merge.
- Result: `scripts/test-destroy-isolation.sh` prints PASS for 8 checks, SKIP for the credential-needing `plan -destroy` check, and `ISOLATION_OK`.

### Unit 4: Ansible (`feat/12-ansible`, merged; built before Packer because the AMI runs it)
- `ansible/{site.yml,ansible.cfg,requirements.yml}`, `inventory/hosts.ini.example`, roles `hardening` (SSH drop-in with `sshd -t` check and reload handler, sysctl, ufw deny-in with rate-limited SSH, unattended upgrades, auditd, removal of legacy network clients) and `runner_host` (apt toolchains, nsjail built at the pinned commit, `lfrunner` user, `/etc/leetforce`). `.gitignore` now ignores `ansible/inventory/hosts.ini`.
- Checked in a throwaway `python:3.12-slim` container: `ansible-playbook --syntax-check` passes and `ansible-lint site.yml` passes at the `production` profile (`make lint-ansible`). Fixed on the way: a templated ufw policy loop (split into two tasks) and an unprefixed role variable (`runner_host_nsjail_commit`); an sshd trailing comment that sshd would reject was removed before the first lint.
- Not run: the playbook has not been executed against a real host (nothing is billable-free to run it on; the dev host is at 97% disk and runs the Phase 1 setup, not this).

### Unit 5: Packer (`feat/12-packer-ami`, merged)
- `packer/runner.pkr.hcl`, `packer/scripts/trivy-scan.sh`, `packer/scripts/cleanup.sh`; `make build-runner-linux` (static x86_64 binary, 31.7 MB), `make packer-validate`, `make build-ami` (billable, not run). Plugins: `hashicorp/amazon ~> 1.3` (v1.8.2 installed) and `hashicorp/ansible ~> 1.1` (v1.1.6).
- Mistakes: `packer init` and `packer validate` ran in separate containers so the plugin was lost (now one container); `ansible-local` is not in the amazon plugin (added the ansible plugin); a Python edit wrote a literal `\n` into `trivy-scan.sh` (fixed with the editor tool). Trivy secret scan would flag the builder's SSH host keys, so they are skipped in the scan and deleted in `cleanup.sh`.
- Checked: `packer fmt -check` and `packer validate`: "The configuration is valid." The AMI was **not** built.

### Unit 6: cost table and docs (`feat/12-cost-docs`)
- README cost table: control host, runner hosts, per-host EBS and public IPv4, AMI snapshot and builder, Neon plan (unchecked). No dollar figures are written because prices vary by region and I did not look them up; the README already tells the reader to check the AWS pricing pages.
- `docs/adr/0020-infrastructure-as-code.md`, this log, `docs/FLOW.md` Phase 12 section (marked in progress), `docs/PROGRESS.md`.

### Continuation (Claude, same session)
- The owner replied "continue" without credentials. Re-checked: `aws sts get-caller-identity` returns no account, `NEON_API_KEY` is unset, no `terraform.tfvars` exists. So `terraform plan` and the AMI build were not run and are recorded as not demonstrated.
- Wrote `docs/phases/phase-12.md` (file list from `git diff --name-status phase-12-start..395f83b`: 30 added, 5 modified, 4 deleted; a first draft said 34 added, corrected after counting with `uniq -c`) and `docs/phases/phase-12-summary.md`. PROGRESS set to `in review`; FLOW ticked with the caveat.

### Review and merge
- The owner chose "Skip review, merge now" in chat. Recorded in the summary and PROGRESS; no answers were written for the owner. Plan and AMI build carried to Phase 13.

### Exit criteria status at this point
- `terraform plan` clean: **not yet shown.** Needs AWS credentials and `owner_cidr`/`key_name` (aws) and `NEON_API_KEY` plus the Neon project id (neon). Both stacks pass `fmt`, `validate` and Trivy.
- AMI builds: **not yet shown.** `packer validate` passes; the build is billable and needs the owner's confirmation and AWS credentials.
- Destroy of `infra/aws` never touches `infra/neon`: shown offline by `make test-destroy-isolation` (static); the `plan -destroy` bonus check runs once credentials exist.

### Commands run and why
- `git checkout -b phase/12-infra-as-code`, `git tag phase-12-start`, one `feat/12-*` branch per unit, `git merge --no-ff`, `git branch -d`: the workflow in CLAUDE.md.
- `ssh leetforce-dev '... 169.254.169.254 ...'`: read the region, instance type and AMI of the dev host so the new stacks match it. `git -C ~/nsjail rev-parse HEAD`: the full commit to pin.
- `terraform fmt/init -backend=false/validate`, `docker run aquasec/trivy config`, `docker run python:3.12-slim ... ansible-lint`, `docker run hashicorp/packer ...`: offline validation and lint of each stack.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/runner-linux-amd64 ./cmd/runner` (in `runner/`): the binary the AMI embeds.
- `scripts/scan-staged.sh`: Trivy secret scan of the staged tree before each commit (clean each time).

### File and path index
- `infra/neon/`, `infra/aws/`, `.trivyignore`
- `scripts/arena.sh`, `scripts/test-destroy-isolation.sh`
- `ansible/`, `packer/`, `bin/runner-linux-amd64` (git-ignored)
- `docs/adr/0020-infrastructure-as-code.md`, `docs/FLOW.md`, `README.md` (cost table), `Makefile` (targets `tf-validate`, `test-destroy-isolation`, `lint-ansible`, `build-runner-linux`, `packer-validate`, `build-ami`)
- Not created and not applied: any AWS resource, any Neon change, any AMI. No host state changed.

# Phase 1: Sandbox core

**Branch:** `phase/1-sandbox-core`
**Range:** `phase-1-start..phase-1-done`
**Status:** working draft (report is completed at the end of the phase)

## Units of work
- [ ] `docs/1-dev-environment`: ADR 0003, `scripts/setup-dev-host.sh`, README cost table
- [ ] `feat/1-go-workspace`: `go.work`, `judge/go.mod`, `.golangci.yml`, `Makefile`, `.env.example`
- [ ] `feat/1-nsjail-wrapper`: `judge/sandbox` Spec/Run, bounded output capture
- [ ] `feat/1-cgroup-limits`: cgroup v2 memory/pids/cpu limits, whole-cgroup kill, measurements
- [ ] `feat/1-result-channel`: dedicated fd for the harness result
- [ ] `test/1-adversarial`: `make test-adversarial` suite
- [ ] `docs/1-adrs-report`: ADR 0004, phase report, phase summary

## Exit criteria (from PLAN.md)
Fork bomb, memory bomb, infinite loop, output flood, network access, and file-system escape attempts are all contained; `make test-adversarial` passes.

## Environment
Developed on an Ubuntu 24.04 x86_64 EC2 t3.micro (see ADR 0003). Verified facts: cgroup v2 (`cgroup2fs`) with `cpu memory pids` controllers, `kernel.apparmor_restrict_unprivileged_userns = 1`, Go 1.27.1, nsjail built from `google/nsjail` commit `4ff54a6`.

## Environment setup log (2026-10-02)

Recorded so a later session can rebuild or audit the dev host. Rationale is in [ADR 0003](../adr/0003-dev-environment-ec2-x86.md); `scripts/setup-dev-host.sh` automates steps 3-6.

### Done by the owner (AWS console and Windows)
1. Launched EC2 instance `leetforce-dev`: Ubuntu Server 24.04 LTS x86_64, `t3.micro`, default VPC, auto-assigned public IPv4, 15 GiB gp3, ed25519 key pair, new security group with SSH from the owner's IP only (HTTP/HTTPS boxes unticked), no IAM instance profile.
2. Saved the private key as `~/.ssh/leetforce.pem` (outside the repo; `*.pem` is also git-ignored) and restricted its permissions with `icacls`.
3. Created `~/.ssh/config` with a `leetforce-dev` host entry (`User ubuntu`, `IdentityFile`, `ForwardAgent yes`). First attempt was saved as `config.txt` and renamed to `config`.
4. Connected once manually to accept the host key into `known_hosts`.

### Done by Claude over `ssh -o BatchMode=yes leetforce-dev '<cmd>'`
1. **Read-only checks:** cgroup type `cgroup2fs`; controllers `cpuset cpu io memory hugetlb pids rdma misc dmem`; `kernel.apparmor_restrict_unprivileged_userns = 1`; 911 MiB RAM, no swap, about 12 GB free disk; passwordless `sudo`; only `git` preinstalled.
2. **apt packages:** `build-essential git curl make pkg-config nodejs npm autoconf bison flex libtool libprotobuf-dev libnl-route-3-dev protobuf-compiler` (Node 18.19.1).
3. **Go 1.27.1** (linux-amd64 tarball from go.dev) unpacked to `/usr/local/go`; PATH line added to `~/.bashrc`.
4. **Swap:** 2 GiB `/swapfile`, enabled and added to `/etc/fstab`.
5. **nsjail** built from `google/nsjail` commit `4ff54a6` in `~/nsjail` and copied to `/usr/local/bin/nsjail`. Smoke test `sudo nsjail ... -- /bin/echo sandbox-ok` printed `sandbox-ok`; nsjail warned the process ran as UID 0 in the global user namespace, so unit 2 must set user-namespace options explicitly.
6. **Repo:** cloned `https://github.com/KarthikSP1911/Leetforce.git` to `~/Leetforce` (HTTPS; the box has no GitHub credentials), set global git name and email, checked out `phase/1-sandbox-core`.
7. **Script run:** `scripts/setup-dev-host.sh` run twice. First run installed golangci-lint 2.14.0 to `~/go/bin` and exposed a `pipefail` bug (fixed in commit `fix(infra): avoid pipefail abort in host setup summary`); second run changed nothing, confirming idempotence.

### State after setup
- Disk 6.4 GB used of 14 GB; swap 2 GiB; nothing running in the background.
- No credentials, instance role, or secrets on the box. No security-group or system configuration changes beyond swap.
- Pushing from the box needs credentials: the workflow is edit and commit on the Windows repo, push to `origin`, then `git pull` on the box to build and test.

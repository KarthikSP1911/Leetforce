# 0003. Development environment: Ubuntu 24.04 x86 EC2 instance, not WSL2

**Status:** accepted (Phase 1)

## Context
Phase 1 needs a real Linux kernel with cgroup v2, namespaces and seccomp to run nsjail and the adversarial suite. The owner works on Windows 11. WSL2 was the first idea; the owner chose to skip it and develop on a cloud Linux host instead.

## Decision
- Develop and test Phase 1 on one **Ubuntu 24.04 LTS, x86_64, EC2 t3.micro** in the account's default VPC, connected through VS Code Remote-SSH (and `ssh leetforce-dev '<cmd>'` for scripted commands).
- Security group: SSH (22) from the owner's IP only. No HTTP/HTTPS, no other inbound rules. No IAM instance profile, so the box holds no AWS credentials.
- Disk: 15 GiB gp3 (about 5.4 GiB used after the toolchain and repo). A 2 GiB swap file is added because the instance has about 0.9 GiB of RAM.
- **x86_64 everywhere.** Dev, CI, and the later runner fleet use the same architecture, so the sandbox, seccomp syscall tables and compiled test programs behave identically. ARM (t4g) was cheaper but would make every later phase deal with two architectures.
- nsjail is built from source (`google/nsjail`, commit `4ff54a6` at install time) and runs as **root via `sudo -n`**. Ubuntu 24.04 sets `kernel.apparmor_restrict_unprivileged_userns = 1`, and writing cgroup files needs root anyway. Phase 6 revisits running the runner with less privilege.
- `scripts/setup-dev-host.sh` reproduces the host setup (idempotent), so the instance can be thrown away and rebuilt.

## Alternatives
- **WSL2:** free and local, but kernel and cgroup setup differ from Ubuntu 24.04 servers, and nested namespaces/cgroup delegation there is a known source of surprises.
- **t4g (ARM) instance:** lower price, but a second architecture to support.
- **Larger instance (t3.small/medium):** more headroom for memory-bomb tests, higher cost. The suite runs inside a host memory cap and swap instead.

## Consequences
- The blast radius of a sandbox escape is this one box: it has no cloud credentials and only SSH inbound from one IP.
- It is billable while running (instance, EBS, public IPv4); stop it when idle. Costs are in the README table.
- t3 defaults to unlimited CPU-credit mode, so long CPU-bound loops can incur surplus-credit charges; keep test runs short. Burst throttling also makes timing noisy, so tests assert on limits, not exact durations.
- The public IP changes on every stop/start unless an Elastic IP is attached, so `HostName` in `~/.ssh/config` needs updating.

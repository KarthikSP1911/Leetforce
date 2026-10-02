# 0004. Sandbox design: nsjail, host-owned cgroup, and host-measured results

**Status:** accepted (Phase 1)

## Context
LeetForce runs code written by strangers. The sandbox must contain fork, memory and output bombs, network and file-system escapes, and it must report what happened without trusting the program. CLAUDE.md requires: untrusted code never runs outside the sandbox; results come over a dedicated channel, not parsed from user stdout; the whole cgroup is killed on a timeout or limit breach; hidden test data and raw stderr are never shown for Submit. Phase 1 builds this on the dev host from ADR 0003 (Ubuntu 24.04, kernel 6.17, cgroup v2, x86_64).

## Decision
**Isolation (nsjail, driven from Go as a subprocess).** Each run gets new user, pid, mount, network and ipc namespaces. The program runs as uid/gid 65534, mapped to the same unprivileged id on the host (`--user 65534:65534:1`). Visible files: `/usr`, `/lib`, `/lib64`, `/bin` read-only; a size-limited tmpfs at `/tmp` (cwd); `/dev/null` (writable) and `/dev/zero`, `/dev/urandom` (read-only). There is no `/proc`, `/sys`, `/etc` or home directory. The network namespace is nsjail's default (only a private loopback). A seccomp denylist returns EPERM for `ptrace, mount, pivot_root, chroot, setns, unshare, bpf, kexec_load, init_module, finit_module, delete_module, perf_event_open, keyctl, add_key, request_key, reboot, swapon, swapoff`. rlimits cap CPU time, file size, open files and core dumps (0).

**Resource limits via a host-owned cgroup.** Go creates one directory per run under `/sys/fs/cgroup/leetforce/job-<pid>-<n>` and enables `memory pids cpu`. nsjail creates and limits its own child cgroup inside it (`--cgroupv2_mount <job dir>`): `memory.max`, swap 0, `pids.max`, CPU quota. nsjail's supervisor stays outside the limited cgroup, so an OOM kill takes only the program and nsjail's log survives. The job directory keeps the totals after nsjail removes its child, so the host reads `memory.peak`, `cpu.stat`, `memory.events` (oom_kill), `pids.peak` and `pids.events` from it. Cleanup is always `cgroup.kill` on the job directory, a wait for `populated 0`, then rmdir; a cgroup that cannot be emptied makes `Run` return `ErrSandbox`.

**Facts come from the host, never from the program.** nsjail's own log goes to a dedicated descriptor (fd 3) that nsjail closes before the program starts; `Run` parses `exited with status`, `terminated with signal` and the time-limit line from it (last line wins). A run only counts as started if nsjail logged `Executing '...'`; otherwise `Run` returns `ErrSandbox` (a host failure, not a verdict). The harness result goes over a second dedicated descriptor (fd 4, kept open with `--pass_fd`) into its own capped buffer. stdout and stderr are capped separately; exceeding any cap discards the excess and kills the whole cgroup.

**Privilege.** nsjail and the Go caller run as root (via `sudo -n` in tests). Ubuntu 24.04 sets `kernel.apparmor_restrict_unprivileged_userns = 1`, cgroup writes need root, and `sudo` closes inherited descriptors so the Go process itself must be root to pass fds 3 and 4.

## Alternatives
- **Let nsjail own the whole cgroup, or start nsjail inside our cgroup (`CgroupFD`).** Rejected: nsjail deletes its cgroup at exit, so the totals are lost; and putting nsjail inside the limited cgroup lets the OOM killer take its log with the program.
- **`RLIMIT_NPROC` for the process cap.** Rejected: it counts per host uid across all jobs. `pids.max` is per run.
- **Trust nsjail's exit code (128+signal).** Rejected: a program can exit 139 on its own, and a failed jail setup sometimes ends in SIGKILL instead of 255 (observed: about 1 run in 5). The log plus the `Executing` marker is unambiguous.
- **Parse a verdict from the program's stdout.** Rejected by CLAUDE.md and by the forgery tests.
- **gVisor, Docker, other judges' sandboxes (for example `isolate`).** Not evaluated this phase; gVisor is scheduled for Phase 6. The `sandbox.Run` boundary keeps the choice replaceable.

## Consequences
**Measured on the dev host (t3.micro, 0.9 GiB RAM):**
- 40 MiB allocation: `memory.peak` 42,967,040 bytes (about 41 MiB, so roughly 1 MiB runtime overhead). 100 MiB under a 64 MiB limit: OOM-killed, `memory.peak` 67,387,392 bytes (about 0.4% over the limit; peak can overshoot slightly).
- Process limit 20: a fork loop stopped after 19 children, `pids.peak 20`, `pids.events max 1`. Fork and thread bombs under a 64 limit end at the 2 s wall limit with peak at most 64.
- `cgroup.kill` removed four processes, including a detached grandchild, in about 2 ms.
- Full adversarial gate: 32 tests, 0 skipped, repeated 5 times; about 19 s of test time per run (18.5 s and 18.9 s in the two logs summed); every OOM event in `dmesg` was `CONSTRAINT_MEMCG` (cgroup-local), none host-wide. Weakening network isolation on a copy made the suite fail, so the suite can detect a weakened sandbox.

**Limits and follow-ups:**
- A CPU-limit kill appears as `Signal=SIGKILL, TimedOut=false` because nsjail sets soft and hard `RLIMIT_CPU` equal. Phase 2 must classify time-limit-exceeded from the measured `CPUTime`, not from the signal.
- `ResultData` is only data. A harness that shares a process with user code (Python, Java drivers) cannot hide fd 4 from it, so the content can be forged. Phase 2 must never let it decide a verdict or override measured facts; outputs are compared on the host side.
- The seccomp denylist overlaps with the unprivileged user namespace (no capabilities), so the escape probe cannot show which layer blocked each call. Phase 6 adds a seccomp-only test and reviews running as root.
- The per-run cgroup parent lives under the root cgroup outside systemd's tree. The tests set a 400 MiB backstop on it; a production runner must set its own, and Phase 12 decides on a systemd slice with `Delegate=yes`.
- The sandbox keeps nsjail's private loopback (reaches nothing outside; some runtimes use it). `--iface_no_lo` would remove it.
- Runs on a burstable instance are timing-noisy: tests assert limits and outcomes, not exact durations.
- The design is Linux-only and needs root, so unit tests that run real programs skip elsewhere; `make test-sandbox` and `make test-adversarial` are the gates.

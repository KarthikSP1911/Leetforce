# 0013. Sandbox: nsjail or gVisor (and seccomp tuning)

**Status:** PROPOSED (Phase 6), awaiting the owner's confirmation at the phase review. Micro-benchmark rows are from a clean run; compile rows are marked noisy (see Measurements). Raw output is in `docs/phases/phase-6-log.md`.

## Context
Phase 1 chose nsjail with cgroup v2 limits ([ADR 0004](0004-sandbox-design.md)). nsjail shares the host kernel: user code makes real system calls to it, restricted by namespaces, a seccomp filter and capability dropping. A kernel bug reachable through an allowed syscall is therefore a path out of the box. gVisor (`runsc`) interposes a user-space kernel, so user code talks to gVisor and only gVisor talks to the host, which shrinks the host attack surface at some cost in speed and compatibility. Phase 6 must raise isolation confidence and record a measured decision (PLAN.md, Phase 6: "decision recorded with measurements; suite passes on the chosen sandbox").

## Decision
**Option C: nsjail stays the default sandbox, hardened; gVisor is built in as an opt-in backend, not the default.**

- Default: nsjail with the tightened seccomp policy (this phase) and the unprivileged runner ([ADR 0014](0014-runner-privilege-model.md)). The full adversarial suite passes on it.
- Opt-in: `LEETFORCE_SANDBOX=gvisor` (or `Spec.Backend`) runs the same jobs under gVisor (`systrap` platform; the EC2 host has no KVM). It judges all 28 cases of the verdict matrix correctly.

Why not make gVisor the default: syscall-heavy work is about 18x slower, every job costs about 85 ms more (15 ms vs 100 ms for hello world), and the first run after a cold cache is about 6x slower, which hurts a judge that runs thousands of short jobs. It also does not enforce the pids limit inside the guest (guest threads are not host threads, so `PIDLimitHit` stays false and a fork past the host pids cap kills the sentry instead of returning `EAGAIN`), and three adversarial tests assume nsjail's behaviour (see Consequences). CPU-bound work is close (1.2x), so the cost is mostly in syscalls and startup.

Revisit if: a kernel escape reachable through an allowed syscall is published and not fixable by the seccomp denylist; the host gets KVM (the `kvm` platform is faster than `systrap`); or the product starts running code from users we trust less than today.

## Alternatives
- **nsjail + tuned seccomp (chosen as default):** fastest and already integrated; the attack surface is the host kernel's syscall interface minus whatever the filter removes.
- **gVisor as the only sandbox:** much smaller host surface, but the slowdowns above, the pids-limit gap and a larger compatibility test matrix.
- **Firecracker / microVMs:** strongest isolation, highest per-job cost and operational weight; out of scope for this phase because the numbers did not force it.
- **Both layered:** gVisor already creates its own namespaces, limits and syscall filtering, so stacking nsjail around it adds cost for little gain. Kept as a per-job choice instead.

## Measurements
Same host, same programs, same limits for both columns. Median / p95, 15 micro repetitions, 3 compile repetitions. The micro rows are a **clean run** (host otherwise idle, load 1.4, 7 reps, `make bench-sandbox BENCH_ARGS="-reps 7 -skip-langs"`). The three compile rows are **noisy** (Agent B's run on a shared host, load 3.3); the quick rerun skipped them.

| Measurement | nsjail | gVisor | Ratio | Notes |
|---|---|---|---|---|
| Cold start (first run, caches dropped) | 116 ms | 672 ms | 5.8x | |
| Per-job overhead (hello world, wall) | 15 / 21 ms | 100 / 114 ms | 6.5x | |
| CPU-bound (600M multiply-add) | 799 / 813 ms | 916 / 918 ms | 1.1x | |
| Syscall-heavy (200k syscalls) | 121 / 135 ms | 2197 / 2271 ms | 18.1x | |
| Memory touch 64 MiB (wall) | 54 / 55 ms | 158 / 173 ms | 2.9x | |
| Peak memory, hello (raw cgroup) | 0.5 MiB | 18.0 MiB | | gVisor adds about 18 MiB, 36 host pids and 100 ms CPU per run; reported peaks are discounted by these baselines, `Result.Raw*` keeps the raw values |
| C++ compile + 5 tests | 0.6 s | 2.4 s | 3.9x | |
| Go compile + 5 tests | 17.5 / 20.5 s | 27.7 / 29.0 s | 1.6x | |
| Java compile + 5 tests | 9.9 / 10.0 s | 4.3 / 4.3 s | 0.4x | Java is faster under gVisor (javac CPU 9.6 s vs 3 s); cause not found, suspects unverified |
| Adversarial suite | all pass (`make test-adversarial`, merged tree) | 36 of 39 top-level tests pass | | None of the 3 failures is a containment failure |

## Seccomp tuning
Policy moved to `judge/sandbox/seccomp.go` (`seccompPolicy()`), kept as a strong denylist, not an allowlist: the runtimes' syscall sets shift with glibc, JVM and kernel versions (`clone3`, `faccessat2`, `rseq` appeared recently) and an allowlist would break judging after routine upgrades. Measured needs per runtime (`strace -f` on Python, static C++, Go, Java and the compile steps) are in the file header.

Gaps found by the new adversarial tests in the old policy: `clone` with `CLONE_NEWUSER|NEWNS|NEWNET|NEWPID` created a child in a new namespace (only `unshare` was denied); `open_tree` returned a descriptor; io_uring, `userfaultfd`, `open_by_handle_at`, the rest of the new mount API and every socket family were unfiltered.

Changes: `clone` is denied when any `CLONE_NEW*` flag is set; `clone3` returns `ENOSYS` so libc and the JVM fall back to `clone`; `socket` is limited to AF_UNIX, AF_INET and AF_INET6 and `socketpair` to AF_UNIX; denied outright: `process_vm_readv/writev`, `kcmp`, `pidfd_getfd`, `umount`, the new mount API, `name_to_handle_at`, `open_by_handle_at`, `userfaultfd`, io_uring, `kexec_file_load`, `acct`, `quotactl`, `syslog`, `lookup_dcookie`, `sethostname`, `setdomainname`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex`, `vhangup`, the NUMA calls, and on amd64 `iopl`, `ioperm`, `modify_ldt`. Pinned by `TestAdversarialDangerousSyscalls` (about 90 calls that must return `EPERM`) and the policy shape test in `seccomp_test.go`. One lesson: kafel does not know `umount2` on x86-64 (the name is `umount`), and an unknown name makes nsjail refuse to start, so any policy change needs the full suite.

## Consequences
- **Runner privileges:** the nsjail default now runs as an unprivileged user with no capabilities ([ADR 0014](0014-runner-privilege-model.md)). The gVisor backend was tested under root only.
- **Latency and throughput:** unchanged for the default. Opting into gVisor costs the numbers above.
- **gVisor host requirements:** the `runsc` package from the official apt repository (`scripts/setup-gvisor.sh`), cgroup v2, `systrap` platform.
- **Adversarial suite on gVisor:** 3 tests fail and need per-backend expectations before gVisor could become the default: `TestAdversarialThreadBomb` (pids limit not seen inside the guest; contained by wall and memory limits), `TestRunProcessLimit` (sentry exits instead of `EAGAIN`), `TestAdversarialFilesystemAndPrivilegeEscape` (reports the guest's own `/proc`, `/sys`, `unshare` and `ptrace` as "escaped"; they have no host effect and host files stayed unreachable). The fork bomb, memory bomb, network, output flood and wall-limit tests pass.
- **Shared cgroup root:** parallel test runs on one host share `/sys/fs/cgroup/leetforce` and each other's leak checks; use `LEETFORCE_CGROUP_ROOT` or run suites one at a time.
- **Known ambiguity:** under gVisor a program that itself exits with 129 to 192 is read as killed by a signal.

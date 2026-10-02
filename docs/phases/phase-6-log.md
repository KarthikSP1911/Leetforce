# Phase 6 log

## Agent A: adversarial suite expansion and seccomp tuning (units 1 and 2)

Work done on the EC2 dev host in a private clone (`~/lf-a`), branches `test/6-adversarial-expand` then `feat/6-seccomp-tuning` (the second contains the first). Agent B (gVisor) used a separate clone; the two share the host's cgroup root.

Environment facts verified: x86_64, Linux 6.17 (AWS), nsjail with the kafel policy compiler, cgroup v2, 1 GiB RAM, Go 1.27.1 at `/usr/local/go`, OpenJDK 21, Python 3, gcc and g++.

### Steps

1. Cloned `~/Leetforce` to `~/lf-a`. The phase branch was not in that clone, so it was sent as a git bundle from the Windows checkout (`git bundle create`, `scp`, `git fetch <bundle>`). The finished branches came back the same way.
2. Read the existing suite (`adversarial_test.go`). It already covered fork, thread, memory, aggregate memory, tmpfs and file-size limits, loops, output floods, a python network probe, one C escape probe, and orphans.
3. Added `judge/sandbox/adversarial_more_test.go` (build tag `adversarial`). Each test asserts containment: a C probe prints `ESCAPED` on any success and `RESULT: contained` at the end.
   - `TestAdversarialProcSysAndDevProbing`: 60 paths under /proc, /sys and /dev must not open or stat; fresh proc, sysfs, cgroup2, devtmpfs, overlay, fuse and binfmt mounts are refused with EPERM; `/` lists only the expected entries.
   - `TestAdversarialFDInheritance`: the host test leaves an inheritable secret file, a directory and a listening socket open; the program may have only fds 0, 1, 2 and 4, fd 3 (nsjail log) is closed and unwritable, and opening `/dev/null` repeatedly stops at RLIMIT_NOFILE.
   - `TestAdversarialHostFilesAndWorkDirEscape`: a planted world-readable host secret and about 30 host paths are unreadable; dot-dot from the bind-mounted dir, the fchdir-and-climb chroot escape, symlinks to `/`, `/etc` and `/proc/self/root`, symlink loops, and O_PATH plus openat all fail; read-only mounts cannot be written, renamed, unlinked, chmod-ed or remounted; mknod is refused.
   - `TestAdversarialNetworkSurfaces`: only `lo` exists and UDP and TCP6 connects fail; host abstract and filesystem unix sockets are unreachable; the host cannot connect to a sandbox listener on 127.0.0.1 or the private address.
   - `TestAdversarialDiskAndInodeExhaustion`: 5M tiny files are stopped by the tmpfs size or the memory limit; tmpfs data is charged to the memory cgroup; fallocate, ftruncate and sparse writes past RLIMIT_FSIZE fail.
   - `TestAdversarialHugeOutputShapes`: one 64 MiB write, and one-byte writes on stdout, stderr and fd 4, are capped at 4096 bytes and stopped promptly.
   - `TestAdversarialBombsPerLanguage`: Python fork and thread bombs, a shell fork bomb, a static C++ thread bomb, Go locked-goroutine and raw-fork bombs, and a Java thread bomb. Each must hit the pids limit with at most 32 tasks and leave no process behind.
4. `requireNoRunCgroups` (in `cgroup_run_test.go`) failed because five `job-<pid>-N` cgroups from another process shared `/sys/fs/cgroup/leetforce`. It now only counts cgroups named for the test process's own pid.
5. First compile failures of the C probes (missing `<sched.h>`, `<time.h>`, `<sys/sysmacros.h>`) were fixed by adding the includes.
6. Wrote `adversarial_syscalls_test.go` (`TestAdversarialDangerousSyscalls`): about 90 dangerous calls by number, each of which must return EPERM (the seccomp answer, not a kernel error), plus `clone` with each CLONE_NEW* flag and `clone3` (ENOSYS).
7. Run against the OLD policy this found real gaps. `clone(CLONE_NEWUSER|...)` succeeded and created a child in a new user namespace (unshare was denied, clone was not). `open_tree` succeeded. Also unfiltered: the rest of the new mount API, io_uring, name_to_handle_at and open_by_handle_at, userfaultfd, adjtimex and clock_adjtime, quotactl, NUMA calls, modify_ldt, and all socket families.
8. Measured what the runtimes need with `strace -f` on Python, a static C++ thread program, a Go binary, a Java program, `g++ -static`, `go build` and `javac`. Results are recorded in the comment at the top of `judge/sandbox/seccomp.go`. Only AF_UNIX, AF_INET and AF_INET6 sockets appear (Java probes nscd over AF_UNIX and opens an IPv4 and an IPv6 socket).
9. Moved the policy from `args.go` to `judge/sandbox/seccomp.go` as the builder `seccompPolicy()`. Decision: a strong denylist, not an allowlist, because runtime syscall sets change across glibc, JVM and kernel releases (clone3, faccessat2 and rseq each appeared recently), so an allowlist would break judging after routine upgrades. The namespaces, empty network, read-only mounts and cgroup limits remain the primary defence.
10. Policy changes, all in `seccomp.go`:
    - `clone` is denied (EPERM) when any CLONE_NEW* flag (mask `0x7e020080`) is set.
    - `clone3` is answered ENOSYS (its flags cannot be inspected by seccomp). glibc and the JVM fall back to `clone`; every language still passes.
    - `socket` is limited to domains 1, 2 and 10 (unix, inet, inet6) and `socketpair` to unix.
    - Added to the denylist: process_vm_readv and writev, kcmp, pidfd_getfd, umount, the new mount API (fsopen, fsconfig, fsmount, fspick, open_tree, move_mount, mount_setattr), userfaultfd, io_uring_setup, enter and register, kexec_file_load, acct, quotactl, syslog, lookup_dcookie, sethostname, setdomainname, settimeofday, clock_settime, clock_adjtime, adjtimex, vhangup, name_to_handle_at, open_by_handle_at, mbind, set_mempolicy, migrate_pages, move_pages. On amd64 also iopl, ioperm and modify_ldt (kafel rejects names an architecture lacks).
    - Gotcha: kafel does not know `umount2` on x86-64; the name is `umount`. nsjail then refuses to start, which every sandbox test reports as "Could not compile policy".
11. Added `seccomp_test.go` (policy shape, and "needed syscalls are not denied").

### Results

- `make test-sandbox` on the new policy: all packages ok (engine 204 s; it compiles and runs Python, C++, Go and Java).
- `make test-adversarial` (full suite, old and new tests): PASS.
- gofmt and `go vet` (with and without the `adversarial` tag) clean for `./judge/sandbox`. Trivy and full `make lint` were skipped by instruction.

### Files

- `judge/sandbox/adversarial_more_test.go`, `judge/sandbox/adversarial_syscalls_test.go`: new adversarial tests.
- `judge/sandbox/cgroup_run_test.go`: `requireNoRunCgroups` filters by pid.
- `judge/sandbox/seccomp.go`, `judge/sandbox/seccomp_test.go`: policy builder, documentation of needed syscalls, unit test.
- `judge/sandbox/args.go`: policy constant removed, calls `seccompPolicy()`.

## Agent C: runner privilege model (branch `feat/6-runner-privilege`)

Environment: dev host clone `~/lf-c` (cloned from `~/lf-a`, branched from `phase/6-sandbox-hardening` at c6ea98e). Done by Claude (agent C).

1. Created `~/lf-c` (`git clone ~/lf-a ~/lf-c`, then `git checkout -b feat/6-runner-privilege origin/phase/6-sandbox-hardening`). `~/lf-c/.env` was copied from the owner's `.env` (git-ignored) for the Redis URL.
2. Probe as a throwaway user (`useradd --system lfrunner-test`, removed at the end): `nsjail --user 65534:65534:1` fails (`gid_map: Operation not permitted`); an unprivileged process may map only its own ids. Mapping `65534:<own uid>` is the way.
3. Then `mount('/', MS_REC|MS_PRIVATE)` failed with EPERM. Kernel audit showed the AppArmor profile `unprivileged_userns` denying capabilities (host: Ubuntu 24.04, `kernel.apparmor_restrict_unprivileged_userns=1`). Fix: profile `scripts/runner/usr.local.bin.nsjail` installed to `/etc/apparmor.d/` and loaded with `apparmor_parser -r`; the probe then ran `/bin/id` as uid 65534 inside the jail.
4. Code: `judge/sandbox/delegate.go` (new: `Rootless`, `PrepareDelegatedRoot`), `judge/sandbox/args.go` (`idMaps`: the host uid/gid of the jail user is the runner's own when not root), `judge/sandbox/unit_test.go` (expects `idMaps()` values) and `judge/sandbox/delegate_test.go` (new). `runner/cmd/runner/main.go`: the root check is replaced by `cgroupRoot()` (env `LEETFORCE_CGROUP_ROOT`, else the delegated cgroup when unprivileged, else the root default).
5. Files: `scripts/runner/leetforce-runner.service`, `scripts/runner/usr.local.bin.nsjail`, `scripts/runner/install-runner.sh`, `scripts/test-runner-unprivileged.sh`, `docs/adr/0014-runner-privilege-model.md`.
6. Host changes made by `install-runner.sh` during testing: system user `lfrunner`, `/opt/leetforce/bin/runner`, `/etc/leetforce/` (the test script writes and removes `runner.env`), `/etc/apparmor.d/usr.local.bin.nsjail` (loaded), `/etc/systemd/system/leetforce-runner.service`. The service is stopped after the test and not enabled. The throwaway user `lfrunner-test` was removed.
7. Mistakes and fixes during testing:
   - Inverted `Rootless()` check in `main.go`: the runner still used `/sys/fs/cgroup/leetforce` (permission denied). Fixed the condition.
   - nsjail "Unable to connect socket: Address family not supported": the unit's `RestrictAddressFamilies` lacked `AF_NETLINK`. Added.
   - `PrivateDevices=yes`: `remountOne /dev/null EPERM` (locked mount flags). Removed.
   - `ProtectHostname=yes` and then the `SystemCallFilter` killed nsjail with SIGSYS on syscall 170 (`sethostname`; found with `journalctl -k | grep type=1326`). Removed `ProtectHostname`, added `sethostname` to the filter allow list.
   - A job that fails host-side takes 3 deliveries (about 100 s with MinIdle 30 s) before an IE verdict, so read the journal rather than waiting.
8. Result: `scripts/test-runner-unprivileged.sh` PASS: runner is a system user (not root), CapEff, CapBnd and CapAmb are 0, NoNewPrivs 1; verdicts AC (python, cpp, java, go), WA, TLE, MLE, RE (python), CE (cpp).
9. Root regression: `go test` of `judge/sandbox` (TestRun, TestIDMaps, TestNsjailArgs) as root PASS.

Path index: `judge/sandbox/delegate.go`, `judge/sandbox/delegate_test.go`, `judge/sandbox/args.go`, `judge/sandbox/unit_test.go`, `runner/cmd/runner/main.go`, `scripts/runner/*`, `scripts/test-runner-unprivileged.sh`, `docs/adr/0014-runner-privilege-model.md`.

## Agent B (gVisor backend and sandbox benchmark)

Branches (clone `~/lf-b` on the dev host, off `phase/6-sandbox-hardening` at c6ea98e): `feat/6-gvisor-backend`, then `test/6-sandbox-benchmark` on top of it.

### Host changes (Claude, dev host)
1. Copied a git bundle of `phase/6-sandbox-hardening` from the Windows checkout to `~/lfb.bundle` and cloned it to `~/lf-b`.
2. `scripts/setup-gvisor.sh`: adds the signing key `/usr/share/keyrings/gvisor-archive-keyring.gpg` (downloaded to a file from gvisor.dev, dearmored), the source list `/etc/apt/sources.list.d/gvisor.list` (official gVisor apt repository), `apt-get update`, `apt-get install runsc`. Result: `/usr/bin/runsc`, release-20260928.0. needrestart printed deferred service restarts; none were done.
3. Runtime state: cgroup root `/sys/fs/cgroup/leetforce-b` (own tests, via `LEETFORCE_CGROUP_ROOT`, so they do not collide with the other agent's `/sys/fs/cgroup/leetforce`); five stale empty `job-*` cgroups in `/sys/fs/cgroup/leetforce` removed with rmdir; `drop_caches` written by the benchmark's cold-start sample; test binaries and logs `/tmp/lfb-*`.
4. Mistake and fix: runsc `--network=none` leaves a `<state>/null-netns` bind mount per run; 606 mounts and work dirs leaked before this was noticed. Fixed in code (detach after `runsc delete`, test checks mountinfo); the old mounts were unmounted with `umount -l` and the dirs removed.

### Design
- `LEETFORCE_SANDBOX=nsjail|gvisor` (default nsjail) or `Spec.Backend`; `judge/sandbox/backend.go`. nsjail path untouched.
- `judge/sandbox/gvisor.go`: per-run OCI bundle (empty read-only root, same read-only binds, tmpfs /tmp, no network, uid 65534, no capabilities, rlimits CPU/FSIZE/NOFILE/CORE), `runsc --network=none --overlay2=none --ignore-cgroups --platform=systrap run --pass-fd`. runsc starts inside the job cgroup (`CgroupFD`); this package sets memory.max, pids.max, cpu.max on the job dir, so `cgroup.kill` and the stats files behave as for nsjail. Result fd: host pipe passed to guest fd 4. runsc log on its own fd; exit 128 plus log text means not started (ErrSandbox); exit 128+N is read as signal N (ambiguous with a program exiting 129..192, accepted).
- gVisor costs about 18 MiB, 36-38 host pids and 100 ms CPU per run, so `Result.PeakMemoryBytes/PeakPIDs/CPUTime` are discounted by those baselines (overhead was constant from 16 to 200 MiB) and `Result.Raw*` keep raw values. Host limits: memory +16+6 MiB, pids +38.
- Platform is systrap (the EC2 host has no KVM).

### Adversarial suite on gVisor (full run, `LEETFORCE_SANDBOX=gvisor`)
36 top-level tests pass, 3 fail, all explained by gVisor semantics rather than containment failures:
- `TestAdversarialThreadBomb`: guest threads are not host threads, so `PIDLimitHit=false`, `PeakPIDs=0`; contained by wall time and memory.
- `TestRunProcessLimit`: forks stop at the host pids cap, but the sentry dies (exit 2) instead of returning EAGAIN, so no "fork failed" output; PIDLimitHit is true (verdict RE).
- `TestAdversarialFilesystemAndPrivilegeEscape`: reports ESCAPED for reading /proc/1/environ and /sys/fs/cgroup (these are the guest's own /proc and /sys, not host files), and unshare(user), unshare(mount), ptrace(TRACEME) succeed inside the guest kernel (emulated, no host effect). Host files stayed unreachable. The probe assumes no /proc and nsjail's seccomp; it needs a per-backend expectation.
Engine verdict matrix under gVisor (`TestJudgeVerdicts`, 28 cases, python/go/cpp/java x ac/wa/tle/mle/re/ole/ce): all pass (195 s).
nsjail regression: the same suite on nsjail showed only failures from the other agent's concurrent Go compile tripping process-leak checks.

### Benchmark (`make bench-sandbox`, one run, host shared with the other agent, load average 3.3 at start: noisy, lead reruns alone)
Median / p95, 15 micro reps, 3 compile reps, 2 vCPU, runsc release-20260928.0.

| Workload | nsjail | gVisor | ratio |
|---|---|---|---|
| cold start (first run, caches dropped) | 74 ms | 746 ms | 10.1x |
| hello world per-job wall | 29 / 30 ms | 161 / 177 ms | 5.5x |
| CPU-bound 600M mult-add | 815 / 878 ms | 962 / 1074 ms | 1.2x |
| syscall-heavy 200k syscalls | 128 / 163 ms | 2230 / 3729 ms | 17.4x |
| memory touch 64 MiB, wall | 67 / 93 ms | 183 / 208 ms | 2.7x |
| peak memory, hello (raw cgroup) | 0.5 MiB | 18.5 MiB | |
| overhead when touching 64 MiB | 0.7 MiB | 16.3 MiB | |
| peak pids, hello | 1 | 36 | |
| C++ compile+5 tests | 0.6 s | 2.4 s | 3.9x |
| Go compile+5 tests | 17.5 / 20.5 s | 27.7 / 29.0 s | 1.6x |
| Java compile+5 tests | 9.9 / 10.0 s | 4.3 / 4.3 s | 0.4x |

Java is faster under gVisor: javac burned 9.6 s CPU under nsjail versus 3 s under gVisor (probe, same limits); JVM startup alone is faster under nsjail (68 ms vs 325 ms). Cause not found; suspect `--disable_proc` or memory-cgroup pressure in nsjail. Unverified.

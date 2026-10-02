# Phase 6 log

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

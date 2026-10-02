# Phase 6 log

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

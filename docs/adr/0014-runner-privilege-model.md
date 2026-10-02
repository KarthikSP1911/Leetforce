# 0014. Unprivileged runner: dedicated user, no capabilities, delegated cgroup, AppArmor userns profile

**Status:** accepted (Phase 6, unit 5). Supersedes the "runner runs as root" part of ADR 0008. The number may need renumbering when merged with the other Phase 6 ADRs.

## Context
The runner ran as root because nsjail and the cgroup writes needed it (ADR 0004, 0008). A bug in the runner (it parses job data and talks to Redis and S3) would then be a root bug. Phase 6 asked to reduce that.

## Decision
The runner runs as the system user `lfrunner` under `scripts/runner/leetforce-runner.service`, with an empty capability bounding set, `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, kernel-protect options and a `SystemCallFilter`. Three things replace root:

1. **User namespace instead of root for nsjail.** nsjail creates a user namespace and does all mounts, the pid, net, uts and ipc namespaces and `pivot_root` with capabilities that exist only inside it. An unprivileged process may map only its own uid and gid, so the jail's uid 65534 now maps to the runner's host uid (`sandbox.idMaps`). Root keeps the old map to host 65534.
2. **cgroup delegation.** `Delegate=memory pids cpu` and `DelegateSubgroup=runner` give the user a cgroup subtree. At startup `sandbox.PrepareDelegatedRoot` finds it from `/proc/self/cgroup`, enables the controllers on the parent (the runner itself sits in the `runner` leaf, as cgroup v2 forbids processes in a cgroup that delegates controllers) and uses `<parent>/jobs` as the per-run cgroup root. `cgroup.kill`, `memory.peak` and nsjail's limits work unchanged because the runner owns the directories it creates.
3. **AppArmor profile for nsjail.** Ubuntu 24.04 sets `kernel.apparmor_restrict_unprivileged_userns=1`, which strips all capabilities inside a user namespace made by an unconfined process (`mount('/')` fails with EPERM). `scripts/runner/usr.local.bin.nsjail` gives only `/usr/local/bin/nsjail` the `userns` permission (the profile is otherwise `unconfined`).

## Alternatives
- **Ambient CAP_SETUID/CAP_SETGID** to keep the 65534 host mapping: rejected; those capabilities allow becoming any user including root, which defeats the point.
- **Ambient CAP_SYS_ADMIN** (and nsjail without a user namespace): rejected; CAP_SYS_ADMIN is nearly root.
- **A small root helper that starts nsjail**, with the rest unprivileged: more code and an IPC surface; not needed since user namespaces suffice.
- **Disable the sysctl `apparmor_restrict_unprivileged_userns` globally:** rejected; it re-enables userns for every unconfined program on the host.
- **gVisor (runsc) rootless:** evaluated separately in the Phase 6 gVisor ADR.

## Consequences
- Measured on the dev host: as `lfrunner` (CapEff, CapBnd and CapAmb all zero, NoNewPrivs 1) the unit judged AC for python, cpp, java and go and WA, TLE, MLE, RE, CE (`scripts/test-runner-unprivileged.sh`).
- The sandboxed program and the runner now share one host uid. The program cannot see the runner (own pid namespace, no /proc), cannot read the runner's files (only /usr, /lib, /bin and the job directory are mounted) and `ptrace`, `unshare`, `setns` are seccomp-denied. A separate uid would be stronger; getting one without CAP_SETUID needs `newuidmap` and a subuid range, a possible later hardening.
- Unit options deliberately left off, with reasons in the unit file: `ProtectHostname` (its seccomp filter kills nsjail on `sethostname`), `PrivateDevices` (nsjail cannot remount the /dev nodes it binds), `ProtectControlGroups`, `RestrictNamespaces`, `MemoryDenyWriteExecute` (JVM children). `sethostname` must be added to the `SystemCallFilter` allow list because it is outside `@system-service`.
- `AF_NETLINK` is needed in `RestrictAddressFamilies` because nsjail brings up the jail's loopback over netlink.

## What still needs root
- Installing: `useradd`, writing `/opt/leetforce`, `/etc/leetforce`, the AppArmor profile and the systemd unit (`install-runner.sh`, run once by an admin or Ansible).
- The adversarial suite and `make test-sandbox` still run as root in a memory-capped scope; they exercise the sandbox, not the runner's privilege.
- Hosts without systemd 254 or newer (no `DelegateSubgroup`) work too (the runner creates its own leaf) but need `Delegate=` set.
- Hosts that forbid unprivileged user namespaces (`user.max_user_namespaces=0`) cannot run unprivileged; use the root runner there.

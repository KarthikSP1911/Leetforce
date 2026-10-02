# Phase 6: Sandbox hardening

**Branch:** `phase/6-sandbox-hardening`
**Range:** `phase-6-start..HEAD` (`phase-6-done` is added when the phase is merged to `main`)
**Dates:** 2026-10-02 to 2026-10-02
**Milestone:** none
**Log:** [phase-6-log.md](phase-6-log.md)

## Summary
Phase 6 made the sandbox harder to break out of and cheaper to trust. The seccomp policy was tightened after new adversarial tests found real holes, the runner now runs unprivileged, and gVisor was built as an opt-in second backend and measured against nsjail. The proposed decision (nsjail default, gVisor opt-in) is in [ADR 0013](../adr/0013-sandbox-nsjail-vs-gvisor.md) and awaits the owner's confirmation.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Decision recorded with measurements | Yes, proposed | ADR 0013 table; clean micro-benchmark run via `make bench-sandbox BENCH_ARGS="-reps 7 -skip-langs"` (log, Lead step 4); compile rows noisy |
| Suite passes on the chosen sandbox (nsjail) | Yes | `make test-adversarial` exit 0 on the merged tree; `make test-sandbox` exit 0. Not rerun after the lint-only fix (`fix/6-lint`) |

## Branches merged
| Branch | Purpose |
|---|---|
| `test/6-adversarial-expand` (via `feat/6-seccomp-tuning`) | New adversarial tests |
| `feat/6-seccomp-tuning` | Tightened seccomp denylist |
| `feat/6-runner-privilege` | Unprivileged runner, hardened unit, AppArmor profile, ADR 0014 |
| `feat/6-gvisor-backend` | gVisor backend and install script |
| `test/6-sandbox-benchmark` | `make bench-sandbox` |
| `docs/6-scaffold`, `docs/6-scan-baseline` | ADR draft, summary skeleton, flow draft; baseline Trivy scan |
| `fix/6-lint` | gosec and noctx findings in new code |

Merge commits (7):
- 283e04b Merge branch 'fix/6-lint' into phase/6-sandbox-hardening
- 6c8edd2 Merge branch 'docs/6-scan-baseline' into phase/6-sandbox-hardening
- f27c071 Merge branch 'docs/6-scaffold' into phase/6-sandbox-hardening
- a8ecb1f Merge branch 'test/6-sandbox-benchmark' into phase/6-sandbox-hardening
- a785eeb Merge branch 'feat/6-gvisor-backend' into phase/6-sandbox-hardening
- 267d59e Merge branch 'feat/6-runner-privilege' into phase/6-sandbox-hardening
- b501217 Merge branch 'feat/6-seccomp-tuning' into phase/6-sandbox-hardening

## File-by-file changes (from `git diff --name-status phase-6-start..HEAD`)

### Added
| File | Purpose |
|---|---|
| `docs/adr/0013-sandbox-nsjail-vs-gvisor.md` | ADR: nsjail default, gVisor opt-in, seccomp changes, measurements (proposed) |
| `docs/adr/0014-runner-privilege-model.md` | ADR: unprivileged runner, what still needs root, shared uid risk |
| `docs/phases/phase-6-log.md` | Running log: sections for Agents A, B, C and the lead |
| `docs/phases/phase-6-scan-baseline.md` | Baseline Trivy scan of `main` before the phase |
| `docs/phases/phase-6-summary.md` | Plain-language phase summary |
| `judge/cmd/sandbox-bench/main.go` | Benchmark tool behind `make bench-sandbox` |
| `judge/sandbox/adversarial_more_test.go` | New adversarial tests: /proc, /sys, fds, host files, network, disk, output, per-language bombs |
| `judge/sandbox/adversarial_syscalls_test.go` | About 90 dangerous syscalls that must return EPERM, plus clone namespace flags and clone3 |
| `judge/sandbox/backend.go` | Backend selection (`LEETFORCE_SANDBOX`, `Spec.Backend`) |
| `judge/sandbox/delegate.go` | Finds and prepares the delegated cgroup for the unprivileged runner |
| `judge/sandbox/delegate_test.go` | Tests for the delegated cgroup logic |
| `judge/sandbox/gvisor.go` | gVisor (runsc) backend: OCI bundle, cgroup placement, result fd, baseline discount |
| `judge/sandbox/gvisor_test.go` | Tests for the gVisor backend |
| `judge/sandbox/seccomp.go` | `seccompPolicy()`: the tightened syscall denylist and per-runtime needs |
| `judge/sandbox/seccomp_test.go` | Unit test of the policy shape |
| `scripts/runner/install-runner.sh` | One-time root install of the unprivileged runner |
| `scripts/runner/leetforce-runner.service` | Hardened systemd unit (no capabilities, delegated cgroups) |
| `scripts/runner/usr.local.bin.nsjail` | AppArmor profile giving only nsjail the userns permission |
| `scripts/setup-gvisor.sh` | Installs runsc from the official apt repository |
| `scripts/test-runner-unprivileged.sh` | End-to-end check: unprivileged runner judges all four languages via the queue |

### Modified
| File | What changed | Why |
|---|---|---|
| `Makefile` | Added `bench-sandbox` target | Runs the nsjail vs gVisor benchmark with a root-capable PATH |
| `docs/FLOW.md` | Replaced the draft Phase 6 section with the as-built flow | Per-phase flow must stay true |
| `judge/sandbox/args.go` | Seccomp policy moved out to `seccomp.go`; jail uid map via `idMaps()` (maps the jail user to the runner uid when unprivileged) | Tighter policy; unprivileged runner can only map its own uid |
| `judge/sandbox/cgroup.go` | Cgroup root can be set by `LEETFORCE_CGROUP_ROOT`; gVisor accounting hooks | Parallel suites and delegated cgroups need their own root |
| `judge/sandbox/cgroup_run_test.go` | `requireNoRunCgroups` counts only cgroups of the test process | Concurrent runs under the shared root made the suite flaky |
| `judge/sandbox/run.go` | Backend dispatch (nsjail or gVisor) | Second backend behind the same interface |
| `judge/sandbox/spec.go` | `Spec.Backend` and raw measurement fields | Backend selection; raw values kept next to baseline-discounted ones |
| `judge/sandbox/unit_test.go` | `TestNsjailArgs` expects `idMaps()` | Follows the uid-map change |
| `runner/cmd/runner/main.go` | Root check replaced by `cgroupRoot()` | Runner can start unprivileged with a delegated cgroup |

### Deleted
None.

### Renamed / moved
None.

## Key code changes
- **Seccomp policy** (`judge/sandbox/seccomp.go`): a strong denylist built by `seccompPolicy()`. `clone` with any `CLONE_NEW*` flag is denied, `clone3` returns `ENOSYS`, sockets are limited to AF_UNIX, AF_INET and AF_INET6, and io_uring, `userfaultfd`, the new mount API, clock changes, NUMA and others are denied. A denylist, not an allowlist, because runtime syscall sets shift with glibc, JVM and kernel versions.
- **gVisor backend** (`judge/sandbox/gvisor.go`): one OCI bundle per run, started inside the job cgroup so `cgroup.kill` and limits work as for nsjail; result over `runsc --pass-fd` to guest fd 4; reported peaks discounted by the measured gVisor baseline (about 18 MiB, 36 pids, 100 ms CPU), raw values kept. A leaked `null-netns` mount per run was found and fixed (`6b8d1dd`).
- **Unprivileged runner** (`judge/sandbox/args.go`, `delegate.go`, `runner/cmd/runner/main.go`): the jail uid maps to the runner's own uid, cgroups come from systemd delegation, and an AppArmor profile lets only nsjail create user namespaces.

## Decisions
- [ADR 0013](../adr/0013-sandbox-nsjail-vs-gvisor.md): nsjail default, gVisor opt-in (proposed).
- [ADR 0014](../adr/0014-runner-privilege-model.md): runner as `lfrunner` with no capabilities; shared uid with the program accepted.

## Tests
- New: `adversarial_more_test.go` (7 test functions), `adversarial_syscalls_test.go` (`TestAdversarialDangerousSyscalls`), `seccomp_test.go`, `delegate_test.go`, `gvisor_test.go`; `scripts/test-runner-unprivileged.sh`.
- Run on the dev host: `make lint`, `make test-adversarial`, `make test-sandbox`, `make bench-sandbox`.
- gVisor: 36 of 39 adversarial top-level tests pass; `TestJudgeVerdicts` 28 of 28.
- Scans: Trivy 0.75.0 on the committed tree, 0 vulnerabilities, 0 secrets, 0 misconfigurations; `redis:7-alpine` has 4 HIGH rows (CVE-2026-75804, CVE-2026-84782 in libcrypto3 and libssl3; local dev only; fix by pinning a rebuilt tag).

## Known issues and deferred work
- Adversarial suite not run against the unprivileged runner (only the end-to-end verdict script): next hardening or Phase 12 (Ansible).
- Three adversarial tests need per-backend expectations before gVisor could become the default.
- gVisor does not enforce the process limit inside the guest; a program exiting 129 to 192 is ambiguous under gVisor.
- Compile-time benchmark rows are noisy; Java faster under gVisor is unexplained.
- Runners and the program share one host uid (ADR 0014).
- Stale `redis:7-alpine` HIGH findings (local dev image).
- Hosts with `user.max_user_namespaces=0` cannot run the unprivileged runner.
- Docs still saying the runner needs root (ADR 0008, comments) need a touch-up.

## Stats
- Commits: 18 (excluding merges)
- Files: 20 added, 9 modified, 0 deleted
- Lines: +2680 / -20

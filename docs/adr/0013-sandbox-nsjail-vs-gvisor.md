# 0013. Sandbox: nsjail or gVisor (and seccomp tuning)

**Status:** DRAFT (Phase 6). Decision pending measurements. Do not treat any row below as a result until it is filled in from a recorded run (see `docs/phases/phase-6-log.md`).

## Context
Phase 1 chose nsjail with cgroup v2 limits ([ADR 0004](0004-sandbox-design.md)). nsjail shares the host kernel: user code makes real system calls to it, restricted by namespaces, a seccomp filter and capability dropping. A kernel bug reachable through an allowed syscall is therefore a path out of the box. gVisor (`runsc`) interposes a user-space kernel, so user code talks to gVisor and only gVisor talks to the host, which shrinks the host attack surface at some cost in speed and compatibility. Phase 6 must raise isolation confidence and record a measured decision (PLAN.md, Phase 6: "decision recorded with measurements; suite passes on the chosen sandbox").

## Decision
**PENDING measurements.** Options to choose between: (A) keep nsjail with a tuned seccomp filter, (B) switch to gVisor, (C) nsjail by default with gVisor as an opt-in or per-language backend. The choice, the numbers that drove it and the conditions under which to revisit go here once the measurements table is complete.

TODO: write the decision.

## Alternatives
- **nsjail + tuned seccomp (status quo, hardened):** fastest and already integrated; the attack surface is the host kernel's syscall interface minus whatever the filter removes.
- **gVisor (runsc):** much smaller host surface; slower on syscall-heavy work, some syscalls and `/proc` or `/sys` details differ, Java and Go runtimes need checking.
- **Firecracker / microVMs:** strongest isolation, highest per-job cost and operational weight; not evaluated in this phase unless the numbers force it (TODO: confirm out of scope).
- **Both, selectable per job:** more code and a larger test matrix; decide only if the numbers split by workload.

## Measurements
Same host, same programs, same limits for both columns. Fill each cell with the number, the unit, the sample count and the log entry that holds the raw output. Empty means not measured.

| Measurement | nsjail | gVisor | Notes (workload, n, log ref) |
|---|---|---|---|
| Cold start (empty program, ms) | | | |
| Per-job overhead (ms, median over N jobs) | | | |
| CPU-bound workload (ms) | | | |
| Syscall-heavy workload (ms) | | | |
| Memory overhead (KB, peak RSS of the box) | | | |
| Go compile (ms) | | | |
| Java compile (ms) | | | |
| C++ compile (ms) | | | |
| Adversarial suite: passed / total | | | |

## Seccomp tuning
TODO: syscalls removed or allowed, how each was justified, and the tests that pin them.

## Consequences
TODO: fill after the decision. To cover: effect on the runner's privileges and host requirements (kernel, `runsc` install, the runner privilege model left open by ADR 0008), per-job latency and throughput, language compatibility, the adversarial suite on the chosen sandbox, what would make us revisit.

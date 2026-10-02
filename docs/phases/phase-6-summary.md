# Phase 6 summary: Sandbox hardening

## TL;DR
- The box is **harder to break out of**: the list of banned system calls is longer, about 90 dangerous calls are now tested to be refused, and the adversarial suite has new attacks (host files, leaked file descriptors, disk fill, per-language fork and thread bombs). Writing those tests found real holes in the old filter, for example a program could create a new user namespace through `clone` even though `unshare` was blocked.
- The **runner no longer needs root**. It runs as an ordinary system user with no special powers, and still judges Python, C++, Java and Go.
- **gVisor was built and measured, and is not the default.** nsjail stays the default, gVisor is an opt-in backend (`LEETFORCE_SANDBOX=gvisor`). The main number: syscall-heavy work is about 18x slower under gVisor, and every job costs about 85 ms more. The decision is *proposed* in [ADR 0013](../adr/0013-sandbox-nsjail-vs-gvisor.md) and waits for your confirmation.
- Exit criteria: decision recorded with measurements (yes, pending your confirmation); suite passes on the chosen sandbox (yes, `make test-adversarial` on the merged code).

## Where this phase fits
```
 browser ---> API ---> Redis queue ---> runner ---> judge engine ---> SANDBOX ---> measured facts ---> verdict ---> browser
 [shell]      [4,5]    [3]              [3]         [2]               [1; HARDENED NOW]
```
Phase 6 changes nothing about the flow, only the strength of the box in the middle. The detailed version is in [docs/FLOW.md](../FLOW.md), section 3, "Phase 6".

- **Depends on Phase 1:** the sandbox wrapper, cgroup limits and the adversarial suite that this phase extends.
- **Depends on Phases 2 to 5:** a working end-to-end flow, so isolation can be judged against real languages and real jobs.
- **Unblocks:** Phase 7 onward can build on a sandbox whose strength is measured, and Phases 12 and 13 know what the runner hosts must provide.

## What I built and why
This phase was built by five helper sessions in parallel (each in its own copy of the repo on the dev host), then merged.

### Bigger adversarial suite (`test/6-adversarial-expand`)
- **What:** new hostile programs in `judge/sandbox/adversarial_more_test.go`: probing `/proc`, `/sys` and `/dev`, leaked file descriptors, reading host files and escaping by symlink, network surfaces, filling the disk, huge output, and fork and thread bombs in Python, shell, C++, Go and Java.
- **Why:** you cannot tighten a filter if you do not know what gets through. A sandbox is only as trusted as the attacks it has survived.
- **How it works:** each test runs a real program in the sandbox and asserts it was contained. The shared cgroup check was changed to count only the test's own cgroups, so parallel runs stop failing each other.

### Tighter seccomp policy (`feat/6-seccomp-tuning`)
- **What:** the banned-syscall list moved to `judge/sandbox/seccomp.go` and grew. `clone` is refused when it asks for a new namespace, `clone3` returns "not supported" so programs fall back to `clone`, only local and internet sockets are allowed, and io_uring, `userfaultfd`, the new mount calls, clock changes and others are refused.
- **Why:** the new tests showed holes: `clone` with namespace flags worked, `open_tree` returned a descriptor, io_uring and every socket family were open.
- **How it works:** it is a *denylist*, not an allowlist, on purpose: the calls Python, Java and Go need change with every glibc or JVM release, and an allowlist would break judging after routine upgrades. `TestAdversarialDangerousSyscalls` pins about 90 refusals. A wrong syscall name makes nsjail refuse to start at all, so every policy change needs the full suite.
- **Alternatives considered:** a full allowlist (too fragile), see [ADR 0013](../adr/0013-sandbox-nsjail-vs-gvisor.md).

### gVisor backend and benchmark (`feat/6-gvisor-backend`, `test/6-sandbox-benchmark`)
- **What:** gVisor (`runsc`) installed from the official apt repository (`scripts/setup-gvisor.sh`) and added as a second backend (`judge/sandbox/gvisor.go`, `backend.go`), plus `make bench-sandbox`, which prints an nsjail vs gVisor table.
- **Why:** gVisor puts its own kernel between the program and the host, so a host kernel bug is much harder to reach. Whether that is worth it depends on the cost, which needed measuring.
- **How it works:** each job gets an OCI bundle (empty read-only root, the same read-only binds, no network, no capabilities). It runs inside the job's cgroup so the same kill and limit logic works. The result still travels over its own file descriptor, not stdout.
- **What the numbers say:** CPU-bound work is about 1.1x, memory work about 2.9x, a hello-world job 6.5x (15 ms vs 100 ms), syscall-heavy work 18x, and gVisor adds about 18 MiB and 36 host processes per job. Java compiled faster under gVisor (cause not found). The adversarial suite on gVisor: 36 of 39 pass; the 3 failures are not escapes but differences (it does not count threads against the process limit, and it reports its own emulated `/proc` and `unshare` as reachable).

### Unprivileged runner (`feat/6-runner-privilege`)
- **What:** the runner runs as the system user `lfrunner` with no capabilities. Files: `scripts/runner/leetforce-runner.service`, `usr.local.bin.nsjail` (AppArmor profile), `install-runner.sh`, `judge/sandbox/delegate.go`.
- **Why:** a runner that is root turns any runner bug into a host takeover.
- **How it works:** nsjail builds its box inside an unprivileged user namespace, per-job cgroups come from systemd delegation, and an AppArmor profile gives only nsjail the permission Ubuntu 24.04 otherwise withholds.
- **Accepted risk:** the program and the runner share one host user id (walled apart by namespaces, no `/proc` and the `ptrace` block). [ADR 0014](../adr/0014-runner-privilege-model.md).
- **Not verified:** the adversarial suite has not been run against the unprivileged runner, only the end-to-end verdict script.

## How it works now, step by step
1. A submission reaches the runner exactly as in Phase 5 (queue, test bundle, `judging` note).
2. The runner process is the unprivileged user `lfrunner`; it asks the engine to run the compiled program.
3. The sandbox picks the backend: nsjail unless `LEETFORCE_SANDBOX=gvisor`.
4. nsjail creates new namespaces without any capabilities, applies the seccomp policy from `seccomp.go`, and starts the program in a cgroup under the delegated root.
5. A banned call (for example `clone` with a namespace flag) gets an error instead of working.
6. On timeout or limit breach the whole cgroup is killed. The verdict is read from the result file descriptor and the rest is unchanged.

## Key concepts
- **System call (syscall):** the request a program makes to the operating system kernel to do anything outside its own memory: open a file, create a process, open a network socket. Untrusted code can only affect the machine through syscalls, so they are where isolation is won or lost.
- **Kernel:** the privileged core of the operating system. A bug in it, reachable by a syscall, can let user code escape every other protection.
- **Attack surface:** the total set of things untrusted code can poke at to try to break out. Fewer reachable syscalls and less shared kernel code mean a smaller surface.
- **seccomp:** a Linux feature that filters syscalls per process. The filter lists which calls are allowed (or killed or denied). LeetForce uses it so a submission cannot, for example, load kernel modules or trace other processes even if it finds a way to try.
- **Capabilities:** Linux splits root's power into small named permissions (mount filesystems, change the clock, load modules, and so on). Dropping them means that even code that runs as "root" inside the box cannot do those things.
- **gVisor:** a sandbox that puts a kernel written in a memory-safe language, running in user space, between the program and the real kernel. The program's syscalls are answered by gVisor, and gVisor itself makes only a small, tightly filtered set of calls to the host. The trade is extra work per syscall (slower) and imperfect compatibility, for a much smaller host attack surface.
- **nsjail:** the Phase 1 sandbox. It uses namespaces (private views of processes, filesystem, network), cgroups (resource limits) and seccomp, but the program's syscalls still go to the host kernel.
- **Adversarial suite:** hostile test programs (fork bombs, memory hogs, escape attempts) that must all be contained. A sandbox change is only acceptable if the whole suite passes.

## Try it yourself
On the EC2 host (`ssh leetforce-dev`), checkout `phase/6-sandbox-hardening`:
```bash
cd ~/Leetforce && export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
make lint                                  # 0 issues
make test-adversarial                      # whole suite passes (takes ~15 min)
make test-sandbox                          # real programs in all four languages
make bench-sandbox BENCH_ARGS="-reps 7 -skip-langs"   # nsjail vs gVisor table (~2 min); without BENCH_ARGS about 10 min with compiles
scripts/test-runner-unprivileged.sh        # the runner as lfrunner judges all four languages (needs .env for Redis)
```

## Trade-offs and risks
- **Speed vs isolation:** keeping nsjail as default means fast judging but the host kernel is still directly reachable through every allowed syscall; the denylist narrows that but cannot remove it. gVisor narrows it much more at 6x to 18x cost on startup and syscalls.
- **A denylist can miss a call.** New kernel syscalls appear over time; the suite and a periodic review are the guard.
- **Shared user id** between runner and program in the unprivileged setup (ADR 0014).
- **gVisor gaps:** the process limit is not enforced inside it, three adversarial tests need per-backend expectations, and a program exiting with 129 to 192 is read as killed by a signal.
- **Benchmark caveat:** the compile-time rows are from a noisy shared-host run; the quick clean rerun skipped them. The Java result is unexplained.
- Revisit if a kernel escape through an allowed call is published, the host gets KVM (faster gVisor platform), or less trusted users arrive.

## Review questions
See the chat review for this phase; the questions are copied here when asked.

## Review Q&A
Not yet answered.

## Open decisions
- **Phase 6 decision:** confirm [ADR 0013](../adr/0013-sandbox-nsjail-vs-gvisor.md) (nsjail default, gVisor opt-in) and the shared user id in [ADR 0014](../adr/0014-runner-privilege-model.md).
- Carried over: Phase 5 decisions A, B, C on defaults; Phase 4 and 5 understanding questions unanswered; Neon plan and Upstash usage unchecked; Phase 2 decisions A and C on defaults; `web/AGENTS.md` and `web/CLAUDE.md` untracked.

## Handoff
- **State:** branch `phase/6-sandbox-hardening` (not yet merged to `main`, no `phase-6-done` tag until the review). The dev host has gVisor (`runsc`) installed, the `lfrunner` user, `/opt/leetforce`, `/etc/leetforce`, a loaded AppArmor profile for nsjail and a stopped, disabled `leetforce-runner` service; clones `~/lf-a`, `~/lf-b`, `~/lf-c`, `~/lf-d` and scratch files in `/tmp` remain. The instance is still running (billable).
- **Next phase:** 7 - Web: problems and workspace. Goal: the LeetCode-style problem list and split-pane workspace on real data. Think about the problem format (Phase 2 decision A).
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-6-summary.md, then start Phase 7 (Web: problems and workspace). Ask me the recap question and show me the session plan before writing any code.
  ```

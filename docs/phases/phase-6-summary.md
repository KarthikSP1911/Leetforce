# Phase 6 summary: Sandbox hardening

> DRAFT skeleton. Concept sections are written; everything marked TODO waits for the phase's results. Nothing here claims a result yet.

## TL;DR
- TODO: what exists now that did not before (seccomp tuning, bigger adversarial suite, the nsjail vs gVisor decision).
- TODO: the decision and the main number behind it.
- TODO: exit criteria status.

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
TODO: one block per unit of work, using the template (What, Why, How it works with file paths, Alternatives considered with the ADR link). Expected units: seccomp tuning and larger adversarial suite, gVisor backend and benchmarks, runner privileges, the ADR ([0013](../adr/0013-sandbox-nsjail-vs-gvisor.md)).

## How it works now, step by step
TODO: one submission through the hardened sandbox, numbered, after the code is final.

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
TODO: exact commands (for example `make test-adversarial`, the benchmark command) and expected output, once they exist.

## Trade-offs and risks
TODO: after the decision (speed vs isolation, compatibility, host requirements, what would make us revisit).

## Review questions
TODO: asked in chat at the end of the phase (3 to 5 understanding questions, 0 to 3 decision questions).

## Review Q&A
TODO: filled in after the review, with who answered.

## Open decisions
TODO: carried over from `docs/PROGRESS.md`, plus anything raised in this phase (including the runner privilege model).

## Handoff
- **State:** TODO (branch, tags, anything left running or configured).
- **Next phase:** 7 - Web: problems and workspace. TODO: goal and decisions to think about.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-6-summary.md, then start Phase 7 (Web: problems and workspace). Ask me the recap question and show me the session plan before writing any code.
  ```

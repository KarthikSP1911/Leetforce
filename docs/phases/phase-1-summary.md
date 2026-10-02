# Phase 1 summary: Sandbox core

## TL;DR
- LeetForce can now run a stranger's program in a locked box (a **sandbox**) on a Linux server and measure it, using a Go function `sandbox.Run`.
- The box stops the classic attacks: programs that copy themselves forever, eat all memory, run forever, print forever, reach the network, or read secret files. 32 automated tests prove it, and they pass.
- Nothing judges answers yet. There is no API, queue, runner or website connection; those come in later phases. Phase 2 turns the box into a judge.

## Where this phase fits
```
browser --> API --> queue --> runner --> sandbox --> verdict --> browser
[shell]    [todo]  [todo]    [todo]     [BUILT NOW]  [todo]     [shell]
 Phase 0    4,5     3          3         *Phase 1*    2,3        7,8
```
- **Built before:** the web page shell (Phase 0).
- **Added this phase:** the sandbox (`judge/sandbox`) and its attack tests.
- **Still to come:** judging answers (Phase 2), queue and runner (Phase 3), API and database (Phase 4), live status (Phase 5), and the real website pages (Phases 7-8).
- **Dependencies:** Phase 1 needed only the repo layout from Phase 0. It unblocks Phase 2 (a judge needs a safe place to run code) and Phase 3 (runners call the sandbox).

## What I built and why

### Dev environment (`docs/1-dev-environment`)
- **What:** an Ubuntu Linux server on AWS (an EC2 `t3.micro`) set up as the place to build and test, plus a script that rebuilds it.
- **Why:** the sandbox uses Linux kernel features that Windows does not have, so we need a real Linux machine. You chose this over WSL2.
- **How it works:** you connect over SSH with a key file; `scripts/setup-dev-host.sh` installs Go, nsjail, the compiler and the linter. See [ADR 0003](../adr/0003-dev-environment-ec2-x86.md).
- **Alternatives considered:** WSL2 (kernel differences), an ARM server (a second architecture to support).

### Go workspace and Makefile (`feat/1-go-workspace`)
- **What:** the `judge` Go module, a `Makefile` with short commands (`make lint`, `make test`, `make test-adversarial`), and lint rules.
- **Why:** everyone, and later the build system, should run the same commands. The workspace follows [ADR 0002](../adr/0002-go-module-layout.md).

### The nsjail wrapper (`feat/1-nsjail-wrapper`)
- **What:** `sandbox.Run(ctx, spec)` starts a program inside **nsjail**, a tool that builds the locked box, and returns what happened.
- **Why:** running untrusted code directly would let it harm the server.
- **How it works:** (`judge/sandbox/args.go`, `run.go`) the program gets its own private process list, its own empty network, and a small read-only copy of system files. It runs as a powerless user. Output is captured with a size cap. Exit status and signals come from nsjail's own log, not from what the program prints.
- **Alternatives considered:** trusting the exit code only (ambiguous; see "Key concepts"), Docker, gVisor (Phase 6).

### CPU, memory and process limits (`feat/1-cgroup-limits`)
- **What:** each run gets its own **cgroup** (a kernel group that limits and measures a set of processes). Limits: memory, number of processes, CPU share. When a run ends or breaks a limit, the whole group is killed.
- **Why:** without a memory limit one program can crash the server; without a process limit a "fork bomb" can freeze it; without killing the whole group, hidden child processes survive.
- **How it works:** (`judge/sandbox/cgroup.go`) our Go code makes a folder for the run under `/sys/fs/cgroup/leetforce`. nsjail makes the limited group inside it. After the run we read the peak memory, CPU time, and whether the kernel killed it for using too much memory. Then we empty and delete the folder.
- **Alternatives considered:** letting nsjail handle everything (it deletes the numbers at the end), and a per-user process limit (it is shared across runs). See [ADR 0004](../adr/0004-sandbox-design.md).

### The result channel (`feat/1-result-channel`)
- **What:** a dedicated "pipe" (file descriptor 4) that a test harness can write its answer to, read separately from normal output.
- **Why:** a program can print anything, including a fake "I passed!". We must never decide a verdict from what the program prints.
- **How it works:** the host reads fd 4 into its own capped buffer. The facts that decide a verdict (exit status, signal, time, memory) still come from nsjail's log and the cgroup, which the program cannot write to. Five tests try to forge results and fail.

### The attack tests (`test/1-adversarial`)
- **What:** `make test-adversarial` runs 11 hostile programs against the sandbox.
- **Why:** this is how we know the box holds. It is also the exit criterion for the phase.
- **How it works:** (`judge/sandbox/adversarial_test.go`) each test runs one attack and checks it was stopped, ended quickly, and left nothing running. The whole suite runs inside a memory-capped scope so a failure cannot crash the server. I also weakened the sandbox on purpose once, and the tests failed as they should.

## How it works now, step by step
1. A test (later, a runner) calls `sandbox.Run` with a command and limits.
2. `Run` makes a cgroup folder for this run.
3. It starts nsjail, which builds the box and starts the command inside. nsjail's log goes to one pipe (fd 3) and the harness result to another (fd 4).
4. The program runs. If it prints too much, runs too long, or breaks a limit, the whole cgroup is killed.
5. When it ends, `Run` reads nsjail's log to learn the real exit status or signal, and reads the cgroup for peak memory, CPU time and out-of-memory kills.
6. `Run` kills anything left, deletes the cgroup, and returns the result. If the box itself failed to start, it returns an error, not a verdict.

## Key concepts
- **Sandbox:** a restricted environment for running untrusted code. We need it because strangers send us programs.
- **Namespace:** a private view of part of the system (processes, network, files). Each run gets its own, so it sees almost nothing of the server.
- **cgroup (control group):** a kernel feature that limits and measures a group of processes together. We need it for memory, process and CPU limits and to kill a whole group at once.
- **nsjail:** a small tool that builds the box using namespaces, cgroups and system-call filters.
- **seccomp:** a filter that blocks dangerous system calls. We use a denylist (block named calls).
- **Fork bomb:** a program that copies itself without end to exhaust the machine.
- **OOM (out of memory) kill:** the kernel killing a process that used more memory than allowed.
- **File descriptor:** a numbered handle to a pipe or file. fd 3 and 4 are our dedicated channels.
- **Host-measured:** facts we read from the kernel and nsjail, never from the program's own words.

## Try it yourself
On the EC2 host (connect with `ssh leetforce-dev`):
```bash
cd ~/Leetforce && git checkout phase/1-sandbox-core && git pull
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
make fmt lint                 # expect: 0 issues.
make test                     # expect: ok  leetforce/judge/sandbox
make test-adversarial         # expect: PASS (32 tests, about 20 s)
make test-adversarial RUN=TestAdversarialForkBomb   # one attack only
```
Expect: all tests pass, no leftover processes (`pgrep nsjail` prints nothing), and no job folders (`sudo ls /sys/fs/cgroup/leetforce` shows nothing). If the instance is stopped, start it in the AWS console first, then update `HostName` in `C:\Users\karth\.ssh\config` (the IP changes).

## Trade-offs and risks
- **The box runs as root** on the server (nsjail needs it here). A bug in nsjail or the kernel is the biggest risk; Phase 6 reviews this and evaluates gVisor.
- **A harness inside the box can be forged.** Anything the program can reach, it can fake, so verdicts must come from host-measured facts plus a host-side answer comparison (Phase 2).
- **CPU-time kills look like SIGKILL**, so the judge must decide "time limit exceeded" from measured CPU time (Phase 2).
- **The tests share a small, slow machine** (1 GiB RAM, burstable CPU); timing checks are generous on purpose.
- **Cost:** the instance, disk and public IP cost money while they exist; stop the instance when idle.
- We would revisit nsjail if a real escape appears, or if runners need to run as a normal user.

## Review questions
Understanding:
1. Why do we get the exit status and memory use from nsjail's log and the cgroup, instead of from what the program prints? What could an attacker do if we trusted the program's output?
2. When a run times out, why do we kill the whole cgroup instead of just the main process?
3. If nsjail cannot build the box (for example a bad mount), why do we return an error instead of a verdict like "Runtime Error"?
4. Why must `Run` leave no process and no cgroup folder behind, and what would eventually happen to a runner that leaked them?
5. What would happen if a program wrote 10 GB to its output, and which two things stop it?

Decisions needed before or during Phase 2:
- A. `web/AGENTS.md` and `web/CLAUDE.md` are untracked files from before this phase. Delete, commit, or ignore them?
- B. Phase 2 needs compilers and runtimes on the dev host (g++, a JDK, Go) and about 1 GB more disk. The instance has about 7 GB free (6.5 GB of 14 GB used). Is that enough, or should the disk be grown?
- C. Do you approve the Phase 2 section of `docs/PLAN.md` as drafted (drivers for Python, C++, Java and Go, verdicts, `problem.yaml`, test-set versioning, `judge run` CLI), or do you want changes?

## Review Q&A
Filled in after the review: each question, your answer, and feedback or clarification.

## Open decisions
- `web/AGENTS.md` and `web/CLAUDE.md` (see A above). Matters now.
- Disk size for runtimes (B). Matters at the start of Phase 2.
- Owner approval of the PLAN.md Phase 2 section (C), and the standing draft review of the whole plan.
- Where the production runner's cgroup parent and root privileges are decided: Phase 3 and Phase 6.

## Handoff
- **State:** branch `phase/1-sandbox-core` (all units merged and pushed), tag `phase-1-start`. `phase-1-done` and the merge to `main` happen after the review. The EC2 instance `leetforce-dev` is running (billable) with the repo at the phase branch; stop it when idle. Scratch files remain in `/tmp/spike/` and an empty `/sys/fs/cgroup/leetforce` on the host (see the log).
- **Next phase:** 2 - Judge engine (M1). Judge a solution against a problem locally: drivers for Python, C++, Java and Go, a compile step, verdicts AC/WA/TLE/MLE/RE/CE/OLE with runtime and memory, `problem.yaml`, test-set versioning, and a `judge run problems/<slug> <file>` command. Think about decisions A, B and C above first. Carry over: classify TLE from measured CPU time, and never decide a verdict from `ResultData`.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-1-summary.md, then start Phase 2 (Judge engine). Ask me the recap question and show me the session plan before writing any code.
  ```

# Phase 2 summary: Judge engine (M1)

## TL;DR
- LeetForce can now **judge a solution**: give it a problem and a program in Python, C++, Java or Go, and it compiles the program, runs it on every test inside the Phase 1 sandbox, and says AC, WA, TLE, MLE, RE, CE or OLE, with runtime and memory.
- You can do it yourself from a terminal: `sudo -n bin/judge run problems/sample-sum <file>`. 28 test solutions (every verdict in every language) are judged correctly, and the Phase 1 attack tests still pass. This is **milestone M1: the judge works locally**.
- It still runs on one machine with no queue, API, database or website. Phase 3 puts a queue and runners around it.

## Where this phase fits
```
 browser ---> API ---> Redis queue ---> runner ---> JUDGE ENGINE ---> SANDBOX ---> measured facts
 [shell]     [todo]     [todo]          [todo]      [ADDED NOW]       [Phase 1]    [Phase 1]
 Phase 0     Ph 4       Ph 3            Ph 3        Phase 2

 verdict <--- API <---- runner <-------- JUDGE ENGINE <--------------- SANDBOX
 [todo]       [todo]    [todo]           [ADDED NOW]
 (shown in browser, Ph 8; live status over SSE, Ph 5)

 Today (one machine):   bin/judge run  ->  engine.Judge  ->  sandbox.Run (compile, then once per test)  ->  verdict
                        [ADDED NOW]        [ADDED NOW]       [Phase 1]
```
The step-by-step version with file paths is in [docs/FLOW.md](../FLOW.md), section 3, "Phase 2".

- **Depends on Phase 1:** the engine can only be trusted because the sandbox already contains hostile code and reports facts the program cannot forge.
- **Unblocks Phase 3:** a runner is "pull a job from the queue, call `engine.Judge`, report the result".
- **Still to come:** queue and runner (Phase 3), API and database (4), live status (5), stronger isolation (6), the website (7 and 8).

## What I built and why

### Runtimes on the dev host (`docs/2-runtimes`)
- **What:** installed the Java JDK on the EC2 server (g++, Go and Python were already there) and added it to `scripts/setup-dev-host.sh`.
- **Why:** you cannot judge Java without a Java compiler on the machine.

### Problem format (`feat/2-problem-format`)
- **What:** a problem is a folder: `problem.yaml` (title, difficulty, checker, time and memory limits, which tests are samples) plus `tests/NN.in` and `NN.out`. The sample problem `problems/sample-sum` (sum of n numbers) is original.
- **Why:** the judge needs one precise definition of "a problem" that can be checked for mistakes.
- **How:** `judge/problem/problem.go` loads and validates it and rejects unknown fields, bad names and tests missing a pair. `version.go` computes a **test-set version**: a short fingerprint (a hash) of all tests. If anyone edits a test, the fingerprint changes, so later we can find and re-judge old submissions. Limits can differ per language (Java gets 2 s and 256 MB where the others get 1 s and 128 MB).
- **Alternatives:** version numbers typed by hand (easy to forget), function-signature problems like LeetCode's (deferred). See [ADR 0005](../adr/0005-problem-format-and-test-set-version.md).

### Verdict rules (`feat/2-verdicts`)
- **What:** `judge/verdict/verdict.go` turns the facts the sandbox measured into a verdict, checking in this order: output flood (OLE), memory (MLE), time (TLE), crash (RE), otherwise "ran fine, now compare the answer".
- **Why:** a verdict must never be something the program can write.
- **How:** it only reads exit status, signal, CPU time, peak memory and the caps, never the program's output. TLE is decided from **measured CPU time**, because a program killed for using too much CPU just looks "killed", exactly like a crash. See [ADR 0007](../adr/0007-verdicts-from-host-facts.md).

### Answer checkers (`feat/2-checkers`)
- **What:** `judge/checker/checker.go` compares the program's output with the expected output: `tokens` (ignore spacing and blank lines, the default) or `exact` (byte for byte, except one final newline).
- **Why:** the comparison runs on the host, so the program never sees the expected answer.

### Language drivers and the engine (`feat/2-drivers-python-go`, `feat/2-drivers-cpp-java`)
- **What:** `judge/lang/lang.go` describes each language (file name, how to compile, how to run, limits). `judge/engine/engine.go` does the work: write the source, compile it in a sandbox, then run every test in a sandbox.
- **Why:** compilers are programs too, and they run on strangers' source, so they must be sandboxed like everything else.
- **How:** the compile step hands the built file back to the host through the special "result" pipe (fd 4), and the host stores it. The sandbox is never given a folder it can write to on the host. Python gets a syntax check so a typo is CE instead of a runtime error. Go and C++ build one binary; Java builds one jar. See [ADR 0006](../adr/0006-compile-in-sandbox-artifact-over-fd4.md).
- **Things that needed solving:** the sandbox has no `/proc` (Go and the JDK both rely on it), so some settings are spelled out; the JDK keeps config files in `/etc`, which is bound in read-only; and the Java memory rule (below).
- **Alternatives:** a writable shared folder (a new risk in the sandbox), compiling on the host (never allowed), compiling and running in one go (cannot tell CE from RE).

### The `judge` command (`feat/2-judge-cli`)
- **What:** `bin/judge run [-all] [-detail] problems/<slug> <file>` prints the verdict, runtime, memory and a per-test table. Exit code 0 means AC.
- **Why:** it is the way to see the judge work, and it is the Phase 2 exit demo.

### The verdict matrix (`test/2-verdict-matrix`)
- **What:** `make test-matrix` judges 28 solutions: for each of the 4 languages, one that is AC, WA, TLE, MLE, RE, OLE and CE. A second test fails if any of the 28 files is missing.
- **Why:** this is the exit criterion, and it proves every verdict path in every language.

### A bug I found at the end (`fix/2-test-outputs-ignored`)
- **What:** while generating the file list for the report I saw that the expected-output files (`tests/NN.out`) were not in git. A line in `.gitignore` meant for build output (`*.out`) had hidden them. Every test had passed only because the files existed on this machine and on the server; a fresh copy of the repository could not load the sample problem.
- **Fix:** an exception in `.gitignore`, the five files committed, and a test that asks git whether any problem test file is ignored (it failed before the fix, passes after). I also checked from a fresh clone. This was my mistake in an earlier unit.

## How it works now, step by step
Judging a C++ solution that is wrong on one hidden test:
1. You run `sudo -n bin/judge run problems/sample-sum problems/sample-sum/solutions/cpp/wa.cpp`. The command picks "cpp" from `.cpp` and loads the problem and its 5 tests.
2. The engine writes the source into a private job folder under `/var/tmp` (never `/tmp`, because the sandbox hides `/tmp`).
3. **Compile:** a sandbox is started that can only see the source. It runs `g++`, then copies the finished binary to the result pipe. The host saves it. A compile problem here means verdict CE and the compiler's message.
4. **Test 01:** a fresh sandbox starts with the job folder read-only, the test input on stdin, a CPU limit, a memory limit and an output cap. The program runs; the sandbox returns exit status, CPU time, memory and what it printed.
5. The verdict code asks: output flood? out of memory? too much CPU time? crash? None, so it compares the printed answer with `01.out` on the host: match, so AC.
6. Tests 02 passes; **test 03** prints a different number, so WA. The engine stops at the first failure (with `-all` it would continue).
7. The job folder is deleted and the table is printed: verdict WA, failed test 03, runtime and memory.

## Key concepts
- **Test-set version:** a fingerprint of all tests. Needed so a submission can say which tests it was judged against and be re-judged after a fix.
- **Checker:** the rule that decides whether two outputs count as the same. LeetForce has two.
- **Artifact:** the file produced by compiling (the binary or jar).
- **fd 4 (the result pipe):** a pipe separate from normal output. The host reads the compiled file from it. The program's own output is never trusted as a verdict.
- **CPU time vs wall time:** CPU time is how long the processor worked for the program; wall time is clock time. A program that sleeps uses wall time but no CPU. We judge TLE on CPU time and use wall time only as a backstop.
- **Kill slack:** the kernel kills a runaway program one second after the limit, so it always measures over the limit and cannot pass as a "crash".
- **Cold compile:** compiling with no cached results. Go and Java compilers are slow when cold.
- **Job folder:** the private temporary folder for one judging job; deleted afterwards.

## Try it yourself
On the EC2 host (`ssh leetforce-dev`; start the instance in the AWS console first if it is stopped, then update `HostName` in `C:\Users\karth\.ssh\config`):
```bash
cd ~/Leetforce && git checkout phase/2-judge-engine && git pull
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
make build-judge
S=problems/sample-sum
sudo -n bin/judge run $S $S/solutions/python/ac.py            # Verdict AC, 5 tests
sudo -n bin/judge run $S $S/solutions/cpp/wa.cpp              # WA, failed test 03 (a hidden test)
sudo -n bin/judge run -all $S $S/solutions/python/wa.py       # WA on 03, 04 and 05
sudo -n bin/judge run -detail $S $S/solutions/python/re.py    # RE, shows the Python error for sample test 01
sudo -n bin/judge run $S $S/solutions/java/ce.java            # CE with the compiler's message
sudo -n bin/judge run $S $S/solutions/go/tle.go               # TLE, about 2 s (the Go compile takes about 10 s first)
make test-matrix        # 28 of 28 PASS, about 3 minutes
make test-adversarial   # PASS: the Phase 1 attacks are still contained
```
Expected: the verdicts above, and afterwards `pgrep nsjail` prints nothing and `ls /var/tmp | grep leetforce-job` prints nothing.

## Trade-offs and risks
- **Slow compiles on this small server.** A Go submission costs 10 to 20 s and Java about 10 s on the t3.micro, because nothing is cached (a cache the program could write to would let one user poison others). Fine for a local judge; it needs a warm read-only cache or a bigger machine before real traffic.
- **stdin/stdout problems only.** Users read input and print output; there are no function-signature templates like LeetCode's yet. Easy to add later without touching the sandbox.
- **Java's memory rule is a compromise.** The heap is set 64 MiB under the limit and the JVM is told to exit with status 3 when it runs out; that exit status means MLE. A program that exits 3 on purpose only turns its own crash into MLE. Java also runs with the faster-starting but slower-peak compiler setting.
- **CPU time includes runtime start-up,** which is why Java gets a bigger time limit.
- **Compile errors are shown on Submit.** That is the compiler's message about the user's own code, but it differs from the strict wording "never return raw stderr for Submit". Your decision, below.
- **The `judge` command names a failed hidden test.** It is a local author tool; the engine never records details for hidden tests.
- We would revisit these if real problems need heavy CPU Java, much faster Go submissions, or function-signature problems.

## Review questions
Understanding:
1. The compiled program travels from the compile sandbox to the host over the result pipe (fd 4) instead of being written to a folder both can see. What could go wrong if the sandbox had a writable host folder, and what do we do with the bytes that come back?
2. A Go solution is `for {}`. Which measurement decides TLE, and why does the kernel only kill the program one second *after* the problem's time limit instead of exactly at it?
3. A Java program allocates a 400 MB array on a 256 MB limit. Why would the memory cgroup alone report a plain crash, how does the engine report MLE instead, and what can a user do to exploit that rule?
4. Why is the output compared on the host and not inside the sandbox? What would a hostile program be able to do if the expected output were inside?
5. What would have happened to `judge run` on a fresh clone of the repository before the `.gitignore` fix, and why did none of our tests notice?

Decisions needed from you:
- **A. Problem format:** are stdin/stdout problems enough for now, or do you want function-signature templates (LeetCode style) added in a specific later phase (for example 7 or 8)?
- **B. Compile errors on Submit:** show the compiler's message to the user (my recommendation, users cannot fix code without it), or follow the strict wording and hide it?
- **C. Go and Java compile speed:** accept 10 to 20 s per submission for now and plan a warm cache for Phase 6 or 13, or do you want it addressed earlier (Phase 3)?

## Review Q&A
Review skipped at the owner's request (2026-10-02: after the report and summary were pushed, the owner wrote "merge it").

**Understanding questions 1 to 5: not answered.** No answers were given, so no gaps in understanding could be assessed. They can be used as the recap question at the start of the next session.

**Decisions A, B and C: not answered.** The defaults used in Phase 2 stay in force and remain open for the owner to change: A = stdin/stdout problems only (ADR 0005); B = compile errors are shown on Submit (ADR 0007); C = Go and Java compile time accepted for now, warm cache deferred (ADR 0006).

## Open decisions
- A, B and C above: not answered at the review (skipped); the Phase 2 defaults stay until the owner decides (they affect Phases 3, 6, 7, 8 and 13).
- Where the production runner's privileges and cgroup parent are decided: Phase 3 and Phase 6 (carried over from Phase 1).
- The dev host holds several identical git stash entries and compiled test binaries in `/tmp`; they can be dropped at any time.

## Handoff
- **State:** Phase 2 is merged into `main` and tagged `phase-2-done` and `M1`; `phase-2-start` marks the start. The branch `phase/2-judge-engine` is kept. Everything is pushed to `origin`. The EC2 instance `leetforce-dev` is running (billable); stop it when idle and update `HostName` after a restart. The host checkout is on the phase branch. `web/AGENTS.md` and `web/CLAUDE.md` are still untracked (decision: ignore).
- **Next phase:** 3 - Queue and runner. Goal: jobs flow through Redis (Upstash) to runners, a crashed runner's job is reclaimed and judged once, and the runner never touches the database. Before it starts, have the Upstash `rediss://` URL ready (`LEETFORCE_REDIS_URL`, never committed), and think about decision C and where the runner's privileges are decided.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-2-summary.md, then start Phase 3 (Queue and runner). Ask me the recap question and show me the session plan before writing any code.
  ```

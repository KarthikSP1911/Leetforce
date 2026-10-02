# LeetForce flow, phase by phase

This file shows how a submission flows through the system and which phase builds each part. It is updated at the end of every phase. Phases 0 and 1 are **as built** (section 3). Phases 2 to 16 are **planned**, taken from [PLAN.md](PLAN.md); only Phases 1 to 3 are firm in the plan, the rest are outlines that get refined at the start of their session.

## 1. The end-to-end flow (the finished system)
```
 browser            API (Gin)           Redis Streams        runner             sandbox
    |  1 submit code    |                      |                 |                  |
    |------------------>| 2 store submission   |                 |                  |
    |                   |   (Postgres, Neon)   |                 |                  |
    |                   | 3 enqueue job ------>|                 |                  |
    |                   |                      | 4 runner pulls  |                  |
    |                   |                      |  (consumer      |                  |
    |                   |                      |   group)------->| 5 judge:         |
    |                   |                      |                 |  compile, run    |
    |                   |                      |                 |  each test ----->| 6 nsjail +
    |                   |                      |                 |                  |   cgroup box
    |                   |                      |                 |<-----------------| 7 measured
    |                   |<-- 8 verdict (POST) -|-----------------|  result          |   exit, time,
    |                   | 9 write verdict      |                 |                  |   memory
    |                   |   (idempotent)       |                 |                  |
    |<-- 10 live status (SSE): Queued -> Judging -> AC / WA / TLE / MLE / RE / CE / OLE
```
Rules that shape the flow (from CLAUDE.md): runners never connect to the database (they talk only to Redis and the API); hidden test inputs, expected outputs and raw stderr are never returned for Submit; a crashed runner's job is reclaimed (`XAUTOCLAIM`) and judged once; verdict writes are idempotent per submission ID; each submission records the test-set version it was judged against.

## 2. What each phase adds to the flow
`[x]` = built. Stage numbers refer to the diagram in section 1.

| Phase | Name | Adds to the flow | Flow after this phase |
|---|---|---|---|
| 0 | Foundation `[x]` | The browser-side page shell: navbar, theme, problem table with sample data; repo rules and layout | `browser (shell only, sample data)` |
| 1 | Sandbox core `[x]` | Stage 6 and 7: run untrusted code in nsjail + cgroup and return host-measured facts | `Go test -> sandbox.Run -> nsjail box -> measured result` |
| 2 | Judge engine (M1) `[x]` | Stage 5: drivers for Python, C++, Java, Go; compile step; checkers; verdicts; `problem.yaml`; test-set versions; `judge run` CLI | `judge CLI -> judge engine -> sandbox -> verdict` (local, one machine, no network) |
| 3 | Queue and runner | Stages 3-4 and 8: Redis Streams, runner module, crash recovery; the runner talks only to Redis and the API | `job in Redis -> runner -> judge -> sandbox -> verdict sent to the API` |
| 4 | API and database | Stages 1-2 and 9: Gin API, Neon Postgres, migrations, idempotent verdict writes, test-set version recorded | `API <-> Postgres`, plus the runner reporting to the API |
| 5 | Live status and storage (M2) | Stage 10 and test data: SSE status stream, MinIO/S3 for tests, hidden-test redaction for Submit | `curl submit -> queue -> runner -> sandbox -> verdict -> SSE`, end to end on one machine |
| 6 | Sandbox hardening | Inside stage 6: gVisor vs nsjail decision, seccomp tuning, bigger adversarial suite | same flow, stronger box |
| 7 | Web: problems and workspace | Browser side of stage 1 with real data: problem list, split-pane workspace, Monaco | `browser shows real problems` |
| 8 | Web: run, submit, results | Stages 1 and 10 in the UI: Run and Submit, console, result panel, SSE client | `browser submit -> ... -> verdict shown in the page` |
| 9 | Auth and limits (M3) | Sign-up/login, sessions, rate limits per user and per IP, solved status in front of stage 1 | usable product on one machine |
| 10 | Problem pipeline | Authoring side: import, validation, reference-solution check, rejudge by test-set version | `fixed test set -> queue -> rejudge` |
| 11 | Observability | Watching every stage: metrics, dashboards, logs, alerts | dashboards show a live submission |
| 12 | Infrastructure as code | Terraform, Packer, Ansible for the places the stages run (nothing applied without confirmation) | the system can be described and rebuilt as code |
| 13 | Cloud deployment (M4) | The same flow running in the cloud (k3s), runner scaling, secrets via SSM, CI deploy | the flow survives losing a runner |
| 14 | Contests | Contest model, timed windows, contest-only problems, scoring in stages 2 and 9 | a mock contest runs end to end |
| 15 | Leaderboard | Rankings fed by verdicts, caching, penalty rules | rankings correct under concurrent submissions |
| 16 | Launch readiness (M5) | Load test, security review, backup and restore drill | findings resolved or accepted in writing |

Notes: the phase names, builds and exits come from `docs/PLAN.md`. In the "Flow after" column, Phases 3 to 16 are my reading of the plan's build lists, not a promise, and will be corrected when each phase is planned.

## 3. As built

### Phase 0: Foundation
```
browser --> /  --redirect--> /problems --> ProblemTable (rows from web/src/lib/sample-problems.ts)
              theme script reads localStorage "lf-theme" before paint --> data-theme on <html>
              theme toggle flips data-theme --> every color changes (components use CSS variables)
```
Files: `web/src/app/` (routes, `globals.css` tokens), `web/src/components/`, `web/src/lib/sample-problems.ts`. See [phase-0-summary.md](phases/phase-0-summary.md).

### Phase 1: Sandbox core
What happens inside one call to `sandbox.Run(ctx, spec)`. File paths are under `judge/sandbox/`.
```
caller (a test today; the judge in Phase 2; the runner in Phase 3)
   |  Run(ctx, Spec{Argv, Env, Stdin, Limits})
   v
 1  validate the spec                                       spec.go   Spec.Validate
 2  make the run's cgroup folder                            cgroup.go newCgroupJob
      /sys/fs/cgroup/leetforce/job-<pid>-<n>   (+memory +pids +cpu enabled)
 3  make two pipes: nsjail log (fd 3), harness result (fd 4)  run.go  runJob
 4  start nsjail with the command line built from the spec   args.go  nsjailArgs
      +-- nsjail builds the box ------------------------------------------+
      |  new user, pid, mount, network, ipc namespaces                    |
      |  runs as uid 65534 (also unprivileged on the host)                 |
      |  read-only /usr /lib /lib64 /bin; tmpfs /tmp; /dev/null,zero,urandom|
      |  no /proc; seccomp denylist; rlimits (cpu, file size, fds, core)    |
      |  its own child cgroup inside the job folder, with the limits:       |
      |     memory.max, swap 0, pids.max, cpu quota                         |
      |  logs "Executing ..." to fd 3, then starts the program              |
      +--------------------------------------------------------------------+
 5  while it runs, Go:                                       run.go, capture.go
      reads stdout and stderr (each capped), fd 4 (capped), fd 3 (the log)
      enforces the wall-time limit through the context
      on any overflow or timeout: kills the WHOLE cgroup     cgroup.go kill
 6  the program ends (or was killed); nsjail exits
 7  read nsjail's log: did it start? exit status, signal,    log.go parseLog
      time-limit kill, or setup errors
        not started -> return ErrSandbox (a host failure, not a verdict)
 8  read the cgroup totals                                   cgroup.go stats
      memory.peak, cpu.stat, memory.events (oom_kill), pids.peak, pids.events
 9  kill anything left, wait until empty, delete the folder  cgroup.go remove
      cannot empty it -> return ErrSandbox
10  return Result{Stdout, Stderr, ResultData, ExitCode, Signal, TimedOut,
      OutputExceeded, WallTime, PeakMemoryBytes, CPUTime, OOMKilled,
      PeakPIDs, PIDLimitHit}
```
Trust rule in this flow: steps 7 and 8 come from nsjail's own log and the kernel, which the program cannot write to. Stdout, stderr and the fd 4 result are data from the program and are never used to decide what happened.

How the attack tests exercise it (`adversarial_test.go`, run by `make test-adversarial`): each test builds a hostile program, runs steps 1 to 10, and then checks that the attack was stopped, `Run` returned promptly, and no process or cgroup folder is left. The design reasoning is in [ADR 0004](adr/0004-sandbox-design.md); the full explanation is in [phase-1-summary.md](phases/phase-1-summary.md).

### Phase 2: Judge engine (as built; done)
What `engine.Judge(ctx, problem, language, source, opts)` does today (`judge/engine/engine.go`):
```
 sudo bin/judge run [-all] [-detail] problems/<slug> <file>   judge/cmd/judge/main.go: picks the language from the
        |   file extension, loads the problem, calls engine.Judge, prints the verdict table; exit 0 = AC, 1 = other, 2 = error
        v
 problems/<slug>/ ---> problem.Load ---> Problem{Spec, Tests, TestSetVer}   judge/problem/problem.go, version.go
                                                  |
 source text + language name                      v
        |                              1. lang.Get(language): source name, compile argv, run argv, limits   judge/lang/lang.go
        v                              2. write <job dir>/src/main.<ext> (host, world-readable, under /var/tmp)
 3. COMPILE (if the language has one) in sandbox.Run: only src/ is visible, read-only
      python: compile() syntax check          go / cpp: go build or g++ -static to /tmp/main, then cat /tmp/main >&4
      java: javac, jar into /tmp/main.jar, then cat /tmp/main.jar >&4   (JDK needs LD_LIBRARY_PATH and a bind of /etc/java-21-openjdk)
      the artifact comes back over fd 4 (ResultData); the host writes it to <job dir>/bin/main
      any non-zero exit, signal, timeout or OOM  ->  verdict CE with the (cleaned, 4 KiB) compiler output; stop
 4. for each test, in name order: sandbox.Run with the whole job dir read-only, test input on stdin, limits from
      problem.yaml (CPU limit + 1 s kill slack, wall 2x + 1 s, memory limit, output cap from the expected size)
 5. verdict.Classify(result, limits)        host facts only: OLE > MLE > TLE > RE, else Completed      judge/verdict/verdict.go
      (MLE also when the exit status equals the language's OOMExitCode: the JVM exits 3 on heap exhaustion)
 6. if Completed: checker.Check(mode, expected, stdout) -> AC or WA                                       judge/checker/checker.go
 7. stop at the first non-AC (or run all with ContinueOnFail); verdict.Summarize -> Overall{verdict, first failed test, max time, max memory}
 8. remove the job dir; return Report{TestSetVersion, Overall, Cases, CompileOutput}
```
Details of a failing test (input, expected, actual, stderr) are recorded only for sample tests and only when the caller sets `Options.Detail` (Run); hidden tests never get any. Design reasoning: [ADR 0005](adr/0005-problem-format-and-test-set-version.md) and [ADR 0006](adr/0006-compile-in-sandbox-artifact-over-fd4.md).

## 4. Keeping this file true
At the end of each phase: tick the phase in section 2, add its "as built" flow to section 3 (the detailed step list with file paths), and correct the "planned" rows if the plan changed.

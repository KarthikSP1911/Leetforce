# LeetForce flow, phase by phase

This file shows how a submission flows through the system and which phase builds each part. It is updated at the end of every phase. Phases 0 to 8 are **as built** (section 3). Phases 9 to 16 are **planned**, taken from [PLAN.md](PLAN.md); the plan is firm only a phase or two ahead, the rest are outlines that get refined at the start of their session.

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
| 3 | Queue and runner `[x]` | Stages 3-4 and 8: Redis Streams, runner module, crash recovery; the runner talks only to Redis and the API | `job in Redis -> runner -> judge -> sandbox -> verdict sent to the API` |
| 4 | API and database `[x]` | Stages 1-2 and 9: Gin API, Neon Postgres, migrations, idempotent verdict writes, test-set version recorded | `curl -> API -> Postgres + Redis -> runner -> Redis -> API ingest -> Postgres` (verdict idempotent; poll `GET /submissions/:id`) |
| 5 | Live status and storage (M2) `[x]` | Stage 10 and test data: SSE status stream, S3-compatible bucket for tests (RustFS locally, ADR 0011), the Judging state, hidden-test redaction checked end to end, a reaper for rows never queued | `curl submit -> API -> queue -> runner (tests from the bucket) -> sandbox -> verdict -> SSE`, end to end on one machine |
| 6 | Sandbox hardening `[x]` | Inside stage 6: gVisor vs nsjail decision, seccomp tuning, bigger adversarial suite | same flow, stronger box |
| 7 | Web: problems and workspace `[x]` | Browser side of stage 1 with real data: problem list, split-pane workspace, Monaco | `browser -> /api rewrite -> API -> catalog + Postgres -> real problems shown` |
| 8 | Web: run, submit, results `[x]` | Stages 1 and 10 in the UI: Run and Submit, console, result panel, SSE client | `browser submit -> ... -> verdict shown in the page` |
| 9 | Auth and limits (M3) `[x]` | Sign-up/login, sessions, rate limits per user and per IP, solved status in front of stage 1 | `browser (signed in) -> API (session, limits in Redis) -> ... -> verdict`; usable product on one machine |
| 10 | Problem pipeline `[x]` | Authoring side: import, validation, reference-solution check, rejudge by test-set version | `fixed test set -> API start detects the new version -> queue -> runner -> new verdict replaces the old one` |
| 11 | Observability `[x]` | Watching every stage: metrics, dashboards, logs, alerts | dashboards show a live submission |
| 12 | Infrastructure as code | Terraform, Packer, Ansible for the places the stages run (nothing applied without confirmation) | the system can be described and rebuilt as code |
| 13 | Cloud deployment (M4) | The same flow running in the cloud (k3s), runner scaling, secrets via SSM, CI deploy | the flow survives losing a runner |
| 14 | Contests | Contest model, timed windows, contest-only problems, scoring in stages 2 and 9 | a mock contest runs end to end |
| 15 | Leaderboard | Rankings fed by verdicts, caching, penalty rules | rankings correct under concurrent submissions |
| 16 | Launch readiness (M5) | Load test, security review, backup and restore drill | findings resolved or accepted in writing |

Notes: the phase names, builds and exits come from `docs/PLAN.md`. In the "Flow after" column, Phases 6 to 16 are my reading of the plan's build lists, not a promise, and will be corrected when each phase is planned.

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

### Phase 3: Queue and runner (as built)
What happens to a job today. There is no API yet, so `lfq` plays the API's part (`queue/cmd/lfq/main.go`):
```
 lfq enqueue [-id ID] <slug> <language> <file>      XADD <prefix>:jobs   job = {submission_id, problem, language, source}
        |      (Setup first creates the consumer group "runners" from ID 0)                        queue/queue.go
        v
 Redis (Upstash rediss:// in production, local Redis in tests)
      <prefix>:jobs  <prefix>:results  <prefix>:jobs:dead  <prefix>:verdict:<submission id>
        |
        v   runner (sudo bin/runner)   runner/cmd/runner/main.go: env config, must be root, SIGINT/SIGTERM stop the loop
 1. Receive(consumer, 5 s)  first XAUTOCLAIM jobs idle > MinIdle (a dead runner's job), else XREADGROUP ">"
      delivered more than MaxDeliveries times, or undecodable -> <prefix>:jobs:dead, never judged      queue.Receive
 2. Published(id)?          a verdict already exists -> Ack and stop (no second judging)                agent.Process
 3. heartbeat goroutine     Touch every MinIdle/3 (Lua: only the current owner may reset the idle time)
      Touch says ErrLost (someone else owns the job) -> cancel the judging and discard the result
 4. judge: slug must match ^[a-z0-9]+(-[a-z0-9]+)*$, problem.Load(problems/<slug>),
      engine.Judge(..., Options{})  (Detail off: this is Submit)                                       agent.judge
 5. outcome: ok -> result from the Report | bad job -> IE | host error -> leave pending (IE on the last attempt)
 6. Publish(result)         Lua: SET <prefix>:verdict:<id> NX EX 7d, then XADD <prefix>:results         queue.Publish
      a second publish for the same submission does nothing (returns false)
 7. Ack                     XACK + XDEL; a crash between 6 and 7 is repaired by a redelivery that stops at step 2
 lfq results                XRANGE <prefix>:results, one JSON line per verdict (the API reads this stream in Phase 4)
```
Crash case (`make test-crash`, `scripts/test-crash-reclaim.sh`): runner A takes the job and starts judging, then `kill -9`. Its claim stops being refreshed; after `MinIdle` runner B's `XAUTOCLAIM` takes the job (delivery 2, `reclaimed: true`), judges it and publishes the only verdict. The module boundary keeps the runner away from the database: `runner/nodb_test.go` fails if `database/sql`, pgx, lib/pq, sqlx, gorm or Gin appear anywhere in the runner's dependency graph. Design reasoning: [ADR 0008](adr/0008-queue-reclaim-and-runner-privileges.md).

### Phase 4: API and database (as built)
The API now plays the part `lfq` played in Phase 3, and verdicts end up in Postgres. Binary `bin/api` (`make build-api`), code in `api/`:
```
 curl POST /submissions {problem, language, source}                          api/internal/server/submissions.go
 1. validate        language in python/cpp/java/go, source 1..64 KiB, body <= ~260 KiB (413), problem given (422)
 2. store           INSERT INTO submissions ... SELECT ... FROM problems WHERE slug = $2       store.InsertSubmission
                    (stamps the problem's CURRENT test_set_version in the same statement; unknown problem -> 404)
 3. enqueue         queue.Enqueue -> XADD <prefix>:jobs {submission_id, problem, language, source}
                    enqueue fails -> the row is deleted and the client gets 503 (no orphan queued row)
 4. respond         202 {"id": <uuid>, "status": "queued"}
        |
        v   runner (Phase 3, unchanged): receive, judge in the sandbox, Publish -> XADD <prefix>:results, Ack
        |
        v   API ingest loop (api/internal/ingest, started by api/cmd/api/main.go; consumer group "api")
 5. ReceiveResult   XAUTOCLAIM entries idle > MinIdle (a crashed API instance's), else XREADGROUP BLOCK 5 s     queue/ingest.go
 6. RecordVerdict   one statement: INSERT INTO verdicts ... ON CONFLICT (submission_id) DO NOTHING, and
                    UPDATE submissions SET status = 'judged' only for the row just inserted      store/verdicts.go
                    duplicate or unknown submission -> no change (acknowledged); transient DB error -> not acknowledged, redelivered;
                    permanent error (class 22/23), undecodable entry or non-UUID id -> logged and acknowledged
 7. AckResult       XACK only (entries stay in the stream for lfq and debugging)
 8. every 30 s      drain <prefix>:jobs:dead: each dead-lettered job -> IE verdict, runner "dead-letter"   ingest.HandleDead
 curl GET /submissions/:id        {id, problem, language, status: queued|judged, created_at, verdict?: {verdict, runtime_ms, memory_kb, passed, total}}
                                  never the source, test data, stderr or test-set version                 server/submissions.go
 curl GET /problems, /problems/:slug   metadata from Postgres; the detail adds only the sample tests (the hidden ones never leave the files)
 GET /healthz (no I/O), GET /readyz (pings Postgres and Redis; 503 names the failing dependency, no error text)
```
At startup the API loads `LEETFORCE_PROBLEMS_DIR` (judge's `problem.Load`) and upserts each problem and its test-set version into Postgres. Database: Neon, schema from `api/migrations/00001_init.sql` (`make migrate-up`); pool settings and test isolation in [ADR 0010](adr/0010-neon-access-migrations-and-test-schemas.md); why the verdict write is idempotent and how dead letters become `IE`: [ADR 0009](adr/0009-idempotent-verdict-ingest.md). Exit test: `make test-api-e2e` (`scripts/test-api-e2e.sh`): AC through the whole chain, a conflicting WA injected into the results stream changes nothing, a dead-lettered job becomes IE. Not yet: SSE live status and the Judging state (Phase 5), authentication and rate limits (Phase 9), a reaper for rows queued but never enqueued after a crash (built in Phase 5).

### Phase 5: Live status and storage (as built)
Four additions to the Phase 4 flow, all on one machine (RustFS, Redis, the API and a runner on the dev host; Neon and Upstash in the cloud). Why: [ADR 0011](adr/0011-problem-tests-in-object-storage.md) (tests in a bucket) and [ADR 0012](adr/0012-live-status-sse-and-reaper.md) (status, SSE, reaper).
```
 API startup (api/cmd/api/main.go)
 0. publish tests   catalog.Publish: Pack each problem (problem.yaml + tests/, no solutions) -> PutBundle
                    problems/<slug>/<test-set-version>.tar.gz in the bucket, tagged with its SHA-256;
                    unchanged bundles are not rewritten. Done before the API listens. /readyz also pings the bucket.
                    reaper: one sweep at startup (step 9), then every 15 min

 curl POST /submissions                                                  api/internal/server/submissions.go
 1-2. validate, store   as Phase 4; InsertSubmission now RETURNs the test_set_version it stamped
 3. enqueue             XADD <prefix>:jobs {submission_id, problem, language, source, test_set_version}
 3b. MarkEnqueued       UPDATE submissions SET enqueued_at = now()   (failure only logged)
 4. respond             202 {"id", "status": "queued"}

 curl -N GET /submissions/:id/events                                     api/internal/server/events.go
 5. stream              first event = current state; then one `event: status` per change (queued -> judging) and
                        `event: verdict` {"status":"judged","verdict":{verdict, runtime_ms, memory_kb, passed, total}}, then close.
                        Reads Postgres every 500 ms (state, not history: a judgement shorter than a poll can go queued -> verdict).
                        Keep-alive every 15 s, timeout event after 10 min, 503 beyond 200 streams per instance, 404 for an unknown id.
        |
        v   runner (runner/internal/agent, runner/internal/problems)
 6. receive             Receive (XAUTOCLAIM / XREADGROUP) as Phase 3
 6b. report judging     XADD <prefix>:status {submission_id, state: judging, runner_id}   queue/status.go (best effort, capped ~5000)
 7. load tests          problems.S3.Load: Stat the bundle (its SHA-256), use the unpacked cache dir for that hash if present,
                        else GetBundle -> Unpack -> problem.Load -> recompute the version; != job.test_set_version -> permanent error (IE)
                        no LEETFORCE_S3_ENDPOINT -> problems.Dir (the old directory mode)
 8. judge, Publish, Ack as Phases 2-3
        |
        v   API (api/internal/ingest)
 8b. status watcher     XREAD <prefix>:status from the tail (no consumer group) -> MarkJudging:
                        UPDATE ... SET status = 'judging' WHERE status = 'queued'  (never undoes a verdict)   ingest/status.go
 8c. verdict ingest     as Phase 4: RecordVerdict (idempotent), status = judged

 9. reaper              store.ReapUnqueued: queued, older than 2 min, enqueued_at IS NULL -> FOR UPDATE SKIP LOCKED in one
                        transaction, Enqueue with the stored test_set_version, mark enqueued_at, commit   api/internal/reaper
```
Database: migrations `00002_judging_status.sql` (status may be `judging`) and `00003_enqueued_at.sql` (the column and a partial index). Redaction for Submit: the POST response, `GET /submissions/:id` and every SSE event carry only state and the verdict view; the compiler output, stdout and stderr never leave the runner's result or the sandbox. Exit test: `make test-live-e2e` (`scripts/test-live-e2e.sh`): (1) queued, judging and AC 5/5 in order over a `curl -N` stream, with a runner that has no problems directory and no database URL; (2) hostile programs echo the hidden input and a marker to stdout, stderr and the compiler, and no response contains them, the source or the version (the detector has a self-test); (3) an orphaned row is re-queued after an API restart and judged. Not yet: Run (custom and sample input, Phase 8), authentication and per-user limits (Phase 9), per-role bucket credentials (Phase 12), retention of old bundles and the results stream (Phases 10 and 11).

### Phase 6: Sandbox hardening (as built)

The flow does not change. Only stages 6 and 7 (the box) get stronger, and the runner stops being root. Decision records: [ADR 0013](adr/0013-sandbox-nsjail-vs-gvisor.md) (nsjail default, gVisor opt-in; proposed, owner to confirm) and [ADR 0014](adr/0014-runner-privilege-model.md) (unprivileged runner).
```
 runner (user lfrunner, no capabilities) -> judge engine -> sandbox backend -> measured facts
 1. backend choice     LEETFORCE_SANDBOX=nsjail (default) | gvisor, or Spec.Backend: judge/sandbox/backend.go, gvisor.go
 2. seccomp filter     judge/sandbox/seccomp.go: seccompPolicy() denylist; clone with CLONE_NEW* denied, clone3 -> ENOSYS, sockets limited to unix/inet/inet6
 3. runner privileges  scripts/runner/leetforce-runner.service (+ AppArmor profile usr.local.bin.nsjail, install-runner.sh); cgroups via systemd delegation, judge/sandbox/delegate.go; LEETFORCE_CGROUP_ROOT
 4. adversarial suite  judge/sandbox/adversarial_more_test.go, adversarial_syscalls_test.go (~90 refused calls); make test-adversarial
 5. benchmarks         make bench-sandbox (judge/cmd/sandbox-bench); numbers in ADR 0013 and docs/phases/phase-6-log.md
```
Exit test: `make test-adversarial` passes on the merged tree (nsjail); `make test-sandbox` passes; `make bench-sandbox` prints the nsjail vs gVisor table. Not yet: the adversarial suite against the unprivileged runner, and per-backend expectations for the 3 adversarial tests that fail under gVisor.

### Phase 7: Web: problems and workspace (as built)

The submission flow does not change. The browser can now read real problems; Run and Submit stay disabled until Phase 8. Decision record: [ADR 0015](adr/0015-catalog-content-and-workspace-delivery.md).
```
 browser (/problems, /problems/<slug>) -> Next.js server components -> API -> catalog on disk + Postgres
 1. list page        web/src/app/problems/page.tsx; components/problems/{ProblemFilters,ProblemTable,Pagination}.tsx (GET form: q, difficulty, tag, page)
 2. workspace page   web/src/app/problems/[slug]/page.tsx; components/workspace/{Workspace,SplitPane,Tabs,CodeEditor}.tsx (statement markdown, language selector, Monaco, console with sample cases)
 3. API client       web/src/lib/api/client.ts (server: LEETFORCE_API_URL; browser: /api rewrite in web/next.config.ts); types in web/src/types/problem.ts
 4. API              GET /problems (filter, paginate, acceptance) and GET /problems/:slug (statement, starters, samples): api/internal/server/problems.go, api/internal/store/problems.go
 5. catalog          judge/problem loads statement.md and starters/<lang>.<ext> beside problem.yaml; api/internal/catalog serves them (read at startup)
 6. theme            --link role in web/src/app/globals.css (blue in light, sky in dark)
```
Exit check: browsed the list and a problem in dark and light mode in Chrome against the real API and Neon; focus ring, tab order, separator arrow keys and tab roles were checked; 16 reference solutions (4 new problems x 4 languages) judge AC in the sandbox. Not yet: Run and Submit (Phase 8), the narrow-screen layout verified in a browser, auth and solved status (Phase 9).

### Phase 8: Web: run, submit, results (as built)

Run is new; Submit keeps its Phase 5 path and gains a UI. Decision record: [ADR 0016](adr/0016-run-and-submit-paths.md). Unit 1 (the run endpoint) is committed on `feat/8-api-run-endpoint` and not yet merged or verified on the dev host; the end-to-end browser check is pending (see [phase-8-log.md](phases/phase-8-log.md)).
```
 Run on samples (Run button or Ctrl+Enter, Testcase tab on "Samples")
 1. client      web/src/hooks/useJudge.ts -> createRun (web/src/lib/api/client.ts): POST /api/runs {problem, language, source}
 2. API         createRun (api/internal/server/runs.go): validate, current test-set version, SetRun(queued) in Redis key <prefix>:run:<id>,
                Enqueue Job{Kind: run, SubmissionID: "run-<uuid>"} on the same jobs stream; 202 {"id", "status": "queued"}
 3. runner      Process -> processRun (runner/internal/agent/run.go): SetRun(judging), heartbeat as for Submit, load the bundle,
                keep only Sample tests, engine.Judge(..., Options{ContinueOnFail, Detail}) (judge/engine/engine.go)
 4. result      SetRun(done, RunResult{verdict, runtime, memory, cases[]}); failing samples carry input/expected/actual/stderr;
                never the results stream, never Postgres; the key expires after 10 min (queue/run.go)
 5. browser     watchRun (web/src/lib/api/watch.ts) polls GET /api/runs/:id every 500 ms until done; ResultPanel.tsx shows the verdict
                (largest text), runtime, memory and the failing-sample details

 Run on custom input (Testcase tab on "Custom input")
 1. client      POST /api/runs with "input" set (up to 8 KiB)
 2. API         same as above; the job has Custom=true and Input
 3. runner      run -> engine.RunCustom: compile, run once on the input under the language limits
 4. result      verdict is a judge verdict or OK (ran cleanly, nothing to compare); stdout and stderr returned; compiler output only for CE
 5. dead letter if runners give up, ingest.HandleDead -> handleDeadRun stores an IE result in the run key (api/internal/ingest/ingest.go)

 Submit with SSE and the polling fallback (Submit button or Ctrl+Shift+Enter)
 1. client      createSubmission: POST /api/submissions with header X-LeetForce-Client (id from localStorage "lf-client")
 2. API         Phase 4-5 path unchanged: insert row (client_id tagged after the insert), enqueue, 202 {"id", "status": "queued"}
 3. runner      Phase 3-5 path unchanged: judge against ALL tests, Detail off, Publish to the results stream
 4. browser     watchSubmission (watch.ts): EventSource on /api/submissions/:id/events -> status and verdict events;
                on stream error, timeout or no EventSource, polls GET /api/submissions/:id every second
 5. display     ResultPanel.tsx shows verdict, runtime, memory and passed/total; never input, expected output or stderr

 Submissions tab
 1. API         GET /problems/:slug/submissions (listSubmissions in api/internal/server/submissions.go): newest 50 rows for the
                X-LeetForce-Client id, verdict view only; missing or malformed id -> empty list, no query (migration 00004_submission_client.sql)
 2. browser     SubmissionsTab.tsx refreshes when useJudge reports a created or judged submission and re-reads every 2 s while
                a row is queued or judging; Workspace.tsx hosts the tab and the console
```
Not yet: unit 1 merged and verified on the dev host, migration 00004 applied to Neon, a real-browser end-to-end check in both themes, authentication and rate limits (Phase 9; the client id is not a credential).

### Phase 9: Auth and limits (as built)

Decision record: [ADR 0017](adr/0017-accounts-sessions-and-limits.md). Log: [phase-9-log.md](phases/phase-9-log.md).
```
 Sign up / log in
 1. client      web/src/components/auth/AuthForm.tsx -> signup/login (web/src/lib/api/client.ts): POST /api/auth/signup | /api/auth/login
 2. API         signup/login (api/internal/server/auth.go): limit per IP (and per account name for login), validate, bcrypt hash or compare
                (dummy hash for unknown accounts), CreateUser / UserByLogin (api/internal/store/users.go)
 3. session     startSession: 32 random bytes -> cookie lf_session (HttpOnly, SameSite=Lax); sessions.token_hash = SHA-256(token), expires in 30 days
 4. browser     AuthProvider.tsx calls GET /api/me (currentUser looks the hash up) and shares the user; UserMenu.tsx shows the name

 Submit or Run when signed in
 1. API         createSubmission / createRun (api/internal/server/submissions.go, runs.go): requireUser (401 if no session),
                then limitUserAndIP -> Queue.Allow (queue/limit.go): INCR + PEXPIRE on <prefix>:rl:<scope>-user:<id> and <scope>-ip:<ip>;
                over the limit -> 429 with Retry-After. Defaults: Submit 10/30 per min, Run 20/60 per min
 2. API         the rest is the Phase 4-8 path; the row is tagged with submissions.user_id (SetSubmissionUser)
 3. client IP   c.ClientIP() honours X-Forwarded-For only from LEETFORCE_TRUSTED_PROXIES (default none)

 Lists
 1. GET /problems -> problemItem.solved from SolvedProblems (an AC verdict on any of the user's submissions); the Problems page forwards lf_session
 2. GET /problems/:slug/submissions -> the signed-in user's newest 50 (ListUserSubmissions); anonymous -> empty list, no query
```
Exit check: `make test-auth-e2e` on the dev host (all PASS), unit and Redis tests, web lint/typecheck/build. Not yet: the login and signup pages looked at in a browser.

### Phase 10: Problem pipeline (as built)

Decision record: [ADR 0018](adr/0018-problem-pipeline.md). Log: [phase-10-log.md](phases/phase-10-log.md).
```
 Validate a problem (author, before publishing)
 1. CLI         judge validate [-structure-only] [-strict] problems/<slug> (judge/cmd/judge/validate.go; make validate-problems)
 2. structure   validate.Structure (judge/validate/validate.go): slug = directory, title, difficulty, limits, NAME.in/NAME.out pairs,
                sizes, samples, at least one hidden test, problem.Load; missing statement or starters are warnings
 3. reference   validate.Reference (judge/validate/reference.go): every solutions/<lang>/<verdict>.<ext> is judged by the engine in the
                sandbox and must produce that verdict (ac passes every test); needs root + nsjail, otherwise exit 2

 Fix a test set -> rejudge
 1. publish     the catalog derives a content hash as the test-set version (api/internal/catalog); at start the API publishes the new bundle
                to the bucket under that version; old bundles stay
 2. detect      rejudge.Sync (api/internal/rejudge/rejudge.go) -> store.SyncProblem (api/internal/store/rejudge.go): reads the old version
                under FOR UPDATE and upserts in one transaction; returns the problems whose version changed
 3. requeue     Rejudger.RunChanged -> store.BeginRejudge: per batch of 100, FOR UPDATE SKIP LOCKED over judged rows whose verdict is for an
                older version (and queued/judging rows on an older version): set test_set_version = current, status = queued,
                enqueued_at = NULL; then Enqueue(Job{TestSetVersion: new}) and MarkEnqueued; a failed enqueue leaves enqueued_at NULL and
                the reaper retries it. On demand: bin/rejudge [-dry-run] <slug> (api/cmd/rejudge/main.go)
 4. runner      judges against the bundle for the job's version; the results marker is <prefix>:verdict:<id>:<version> (queue/queue.go
                Publish/Published), so the rejudge is not mistaken for a duplicate
 5. store       store.RecordVerdict (api/internal/store/verdicts.go): replaces the stored verdict only if the incoming version equals the
                submission's current version and differs from the stored one; an IE never replaces a real verdict; stale or duplicate
                results change nothing
 6. browser     the Submissions tab shows the row as queued while the rejudge runs, then the new verdict
```
Exit check: `make test-rejudge-e2e` on the dev host (a changed test file turned AC into WA at the new version, once; a repeat changed nothing), `make validate-problems` (5 of 5 valid), `make test` and the other gates. Not yet: a rejudge of a large backlog in the background (it runs before the API listens), pruning old bundles.

### Phase 11: Observability (as built)

Decision record: [ADR 0019](adr/0019-observability.md). Log: [phase-11-log.md](phases/phase-11-log.md).
```
 Metrics (pull)
 1. API         every request is counted by route template (api/internal/server/server.go); submissions, runs, verdicts stored, ingest
                outcomes, 429s by scope, SSE streams, rejudges and reaper re-queues are counted where they happen (api/internal/metrics)
 2. queue       a sampler (metrics.SampleQueue, every LEETFORCE_METRICS_QUEUE_EVERY, default 60s) calls queue.Stats (queue/stats.go):
                waiting = XLEN - pending, pending, oldest unfinished job age (Redis TIME), dead letters -> leetforce_queue_* gauges
 3. runner      jobs by kind and verdict, judge time by language, in flight, reclaimed, lost, host failures, last queue poll
                (runner/internal/agent, runner/internal/metrics)
 4. listeners   API 127.0.0.1:9102 and runner 127.0.0.1:9101 (LEETFORCE_METRICS_ADDR), separate from the public API port
 5. tunnel      scripts/obs-tunnel.sh forwards both ports to the PC; Prometheus (observability/prometheus) scrapes every 15s

 Logs
 6. files       the demo script writes API and runner JSON logs to ~/obs on the host; scripts/obs-logs.sh copies them to observability/logs
 7. Alloy       tails them, labels service and level, pushes to Loki (observability/alloy/config.alloy)

 Seeing and alerting
 8. Grafana     provisioned datasources and the "LeetForce Submission flow" dashboard (observability/grafana): queue, flow, verdicts,
                judge time, runners, API, firing alerts, warnings and errors from Loki
 9. alerts      observability/prometheus/alerts.yml: RunnerDown, RunnerSilent, RunnerHostFailures, QueueBacklog, QueueStuck, DeadLetters,
                QueueSampleFailing, APIDown, API5xxRate, InternalErrorVerdicts; no notifier; make test-alerts runs promtool tests
```
Exit check: `make test-obs-e2e` on the dev host, `make test-alerts` and the dashboard filling during `scripts/obs-demo.sh`.

### Phase 12: Infrastructure as code (as built, in progress)

Decision record: [ADR 0020](adr/0020-infrastructure-as-code.md). Log: [phase-12-log.md](phases/phase-12-log.md). Nothing here is applied or built yet; the flow itself does not change, this phase describes the hosts it will run on.
```
 Describing the places the stages run
 1. infra/neon    Terraform imports the existing Neon project (import block, prevent_destroy); state is infra/neon/terraform.tfstate;
                  output database_url is sensitive and goes to SSM in Phase 13
 2. infra/aws     default VPC only: control host + runner_count runner hosts, security groups open to owner_cidr (22, k3s 6443),
                  per-port egress, IMDSv2, encrypted gp3; IAM: runners read /leetforce/runner/* only, the control host /leetforce/*
 3. arena.sh      scripts/arena.sh up|down|status acts on infra/aws only; up and down each need a typed phrase
 4. Ansible       ansible/site.yml: hardening (SSH, sysctl, ufw, unattended upgrades, auditd) on every host; runner_host (toolchains,
                  nsjail at the pinned commit, lfrunner user) on runners
 5. Packer        packer/runner.pkr.hcl: Ubuntu 24.04 + Ansible + runner binary and unit (disabled) + Trivy gate -> encrypted, IMDSv2 AMI;
                  infra/aws takes it through runner_ami_id
```
Exit checks: `make test-destroy-isolation` (offline), `make tf-validate`, `make packer-validate`, `make lint-ansible`; `terraform plan` and `make build-ami` need credentials and the owner's confirmation.

## 4. Keeping this file true
At the end of each phase: tick the phase in section 2, add its "as built" flow to section 3 (the detailed step list with file paths), and correct the "planned" rows if the plan changed.

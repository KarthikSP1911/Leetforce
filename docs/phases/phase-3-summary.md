# Phase 3 summary: Queue and runner

## TL;DR
- Judging is no longer a command you type. A job (submission ID, problem, language, source) is put into a **queue** in Redis, and a **runner** program picks it up, judges it with the Phase 2 engine, and reports a verdict.
- If a runner dies halfway through a job, nothing is lost: another runner takes the job over and exactly one verdict is recorded. I tested this by killing a runner with `kill -9` in the middle of a Go compile.
- The runner never touches the database, and a test fails the build if that ever changes. The queue lives on Upstash (your Redis), so it works across machines.

## Where this phase fits
```
 browser ---> API ---> Redis queue ---> RUNNER ---> judge engine ---> sandbox ---> measured facts
 [shell]     [todo]    [ADDED NOW]     [ADDED NOW]   [Phase 2]        [Phase 1]    [Phase 1]
 Phase 0     Ph 4      Phase 3         Phase 3

 verdict <--- API <---- results stream <--- RUNNER
 [todo]       [todo]    [ADDED NOW]         [ADDED NOW]
 (shown in browser, Ph 8; live status over SSE, Ph 5; the API reads the results stream in Ph 4)

 Today:  lfq enqueue -> Redis jobs stream -> bin/runner -> engine.Judge -> sandbox -> results stream -> lfq results
         (lfq stands in for the API until Phase 4)
```
The numbered, file-level version is in [docs/FLOW.md](../FLOW.md), section 3, "Phase 3".

- **Depends on Phase 2:** the runner is a thin loop around `engine.Judge`; it is only safe to run jobs from a queue because the judge and sandbox already contain hostile code.
- **Unblocks Phase 4:** the API only has to put jobs on the stream and read verdicts from the results stream; it does not need to know about runners.
- **Still to come:** API and database (4), live status and storage (5), stronger isolation (6), the website (7 and 8).

## What I built and why

### Local services (`feat/3-compose`)
- **What:** `docker-compose.yml` for Redis and MinIO on your own machine, plus `make dev` and `make down`. I also installed Redis on the EC2 dev host.
- **Why:** tests need a real Redis; mocks cannot show how `XAUTOCLAIM` really behaves. MinIO is for test data in Phase 5.
- **Caveat:** I could not run `docker compose` (Docker is not installed on the host and Docker Desktop was off), so the file is unchecked. Redis on the host is what the tests use.

### The queue (`feat/3-queue-package`)
- **What:** a Go module `queue/` that wraps Redis Streams: put a job in, take one out, acknowledge it, publish a verdict.
- **Why:** many runners must share one list of jobs without two of them taking the same job, and a crashed runner's job must not be lost.
- **How it works** (`queue/queue.go`):
  - Jobs sit in a **stream**. Runners form a **consumer group**: Redis hands each job to one runner and remembers it as "pending" until that runner acknowledges it.
  - A runner asks for work in two steps: first "give me a pending job that has sat unacknowledged for 30 s or more" (`XAUTOCLAIM`, that is a dead runner's job), then "give me a new job".
  - While judging, the runner sends a **heartbeat** every `MinIdle/3`. It resets the job's idle timer, so a long job is not taken away. The heartbeat only works for the current owner (a small Lua script runs inside Redis to check this), so a slow runner cannot grab a job back from the runner that took it over.
  - A job that was handed out more than 3 times goes to a **dead-letter stream** instead of being retried forever (a "poison" job that kills every runner).
  - Publishing a verdict is one Lua script: "if no verdict exists for this submission, record it and add it to the results stream". A second publish does nothing.
- **Alternatives considered:** a database table as the queue (the runner must not touch the database), hand-made Redis lists, a heartbeat-less long timeout. See [ADR 0008](../adr/0008-queue-reclaim-and-runner-privileges.md).

### The runner (`feat/3-runner-module`)
- **What:** `runner/` is its own Go module. `runner/internal/agent/agent.go` is the loop; `runner/cmd/runner/main.go` reads settings from environment variables and refuses to start unless it is root.
- **Why:** this is the piece that turns "a job in Redis" into "a verdict", on any number of machines.
- **How it works:** receive a job; if a verdict already exists, just acknowledge it; start the heartbeat; check that the problem name is a plain slug (it comes from a queue, so it must never turn into a path like `../x`); load the problem and call the engine in **Submit** mode (no detail about hidden tests); publish the verdict; acknowledge. What happens on failure matters:
  - the **host** fails (disk, sandbox cannot start): leave the job pending so it is retried; on the last try report `IE` (internal error);
  - the **job** is bad (unknown problem or language, source too large): report `IE` right away;
  - the runner **loses the job** (heartbeat says someone else owns it): stop judging and say nothing.
  `IE` is not a judge verdict; it says the platform failed, not the program. It carries no error text, so internal details do not reach users.
- **Alternatives:** the runner posting to the API over HTTP. The API does not exist yet, so I used the results stream, which the API will read later.

### The `lfq` tool and the crash test (`feat/3-lfq-tool`)
- **What:** `lfq enqueue`, `lfq results`, `lfq destroy` (a small command that plays the API's part), and `make test-crash`.
- **Why:** to *see* the system work, and to turn the exit criterion into a repeatable test.
- **How the crash test works:** start runner A, enqueue a Go solution (its cold compile takes 10 to 20 s, a wide window), wait until A logs "judging", `kill -9` A, start runner B, and check that B reclaims the job, that exactly one `AC` verdict exists (also after waiting 8 more seconds, which would expose a duplicate), and that it names runner B.

### No database in the runner (`test/3-no-db-dependency`)
- **What:** a test that lists everything the runner is built from (`go list -deps`) and fails if it finds `database/sql`, pgx, lib/pq, sqlx, gorm or Gin.
- **Why:** "runners never connect to the database" was a rule on paper; now it is a failing build. A runner that reached the database would be a high-value target, because it runs next to hostile code.
- **Proof it works:** I forbade a package the runner really uses (`log/slog`) and the test failed, then reverted.

## How it works now, step by step
A user's Python solution that is correct, with two runners running:
1. `lfq enqueue sample-sum python solution.py` creates the consumer group if needed and adds a job to the `jobs` stream. Redis returns an entry ID.
2. Runner 1 (waiting up to 5 s for work) asks for pending jobs idle for 30 s or more: none. It asks for a new job and Redis gives it this one, marking it pending for runner 1.
3. Runner 1 checks the verdict marker for this submission: not there. It starts the heartbeat.
4. It loads `problems/sample-sum` and calls `engine.Judge`: compile check, then each test in a sandbox (Phase 2).
5. Every 10 s the heartbeat resets the idle timer; each check confirms runner 1 still owns the job.
6. The engine returns AC. The runner publishes: Redis sets the marker `verdict:<id>` and appends the result to `results` in one step.
7. The runner acknowledges the job (removed from pending and from the stream).
8. `lfq results` prints one line with the verdict, runtime, memory, test-set version, tests passed and the runner's name.

If runner 1 had been killed at step 4: steps 5 to 7 never happen, the idle timer runs out, and runner 2's step "give me a pending job idle 30 s or more" takes the job. It judges and publishes. If runner 1 had instead been killed *between* steps 6 and 7, runner 2 would find the verdict marker at step 3 and just acknowledge.

## Key concepts
- **Queue:** a list of jobs waiting to be done. LeetForce needs one so the API can accept a submission instantly and runners can work at their own pace.
- **Redis Stream:** an append-only list in Redis with built-in tracking of who is working on which entry.
- **Consumer group:** a set of workers sharing one stream; each entry goes to exactly one of them.
- **Pending entry:** a job handed to a worker that has not yet acknowledged it; Redis remembers who has it and for how long it has been idle.
- **`XAUTOCLAIM`:** "take over pending entries idle longer than X". It is how a dead runner's job is recovered.
- **Heartbeat:** a periodic "I am still working on it" that resets the idle timer.
- **Idempotent:** doing it twice has the same effect as doing it once. Publishing a verdict is idempotent per submission ID, so a redelivery cannot create a second verdict.
- **At-least-once vs exactly-once:** the queue may deliver a job twice (after a crash); the verdict is still recorded once.
- **Dead-letter stream:** where jobs go when they have failed too many times, so they stop blocking the queue.
- **Lua script (in Redis):** a small program Redis runs as one indivisible step; used for "check the owner then refresh" and "record the verdict once".
- **`IE`:** internal error; the platform, not the user's program, failed.

## Try it yourself
On the EC2 host (`ssh leetforce-dev`; start the instance first if it is stopped and update `HostName` in `C:\Users\karth\.ssh\config`). The Upstash URL is already in `~/Leetforce/.env`.
```bash
cd ~/Leetforce && git checkout phase/3-queue-runner && git pull
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
make build-runner && make fmt lint test        # 0 issues, all ok

make test-crash                                 # about a minute
# expect: steps 1-5, then "PASS: runner killed mid-job; job reclaimed and judged once"

# by hand, with your own throwaway prefix:
set -a; . ./.env; set +a
export LEETFORCE_QUEUE_PREFIX=mydemo LEETFORCE_JOB_MIN_IDLE=6s
sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_JOB_MIN_IDLE \
  env LEETFORCE_PROBLEMS_DIR=$PWD/problems bin/runner &     # JSON logs on the terminal
bin/lfq enqueue sample-sum python problems/sample-sum/solutions/python/ac.py
bin/lfq results                                  # one JSON line, verdict AC
bin/lfq enqueue -id same sample-sum python problems/sample-sum/solutions/python/wa.py
bin/lfq enqueue -id same sample-sum python problems/sample-sum/solutions/python/ac.py
bin/lfq results                                  # only ONE line for "same"; the second is ignored
sudo -n pkill -TERM -x runner                    # clean stop ("runner stopped")
bin/lfq destroy                                  # removes only the mydemo keys
```
Afterwards: `pgrep nsjail` prints nothing. A killed runner leaves a folder in `/var/tmp` (`ls /var/tmp | grep leetforce-job`); that is a known issue, remove with `sudo rm -rf /var/tmp/leetforce-job-*`.

## Trade-offs and risks
- **Root runner.** nsjail needs root, so a bug in the runner has root's power. Accepted on the dev host; Phase 6 decides a safer split. Do not run runners on a shared machine before then.
- **Slow Upstash from here.** Each command takes 0.5 to 1 s from the dev host, so the idle window must be long (default 30 s) and my timing tests scale to the measured round trip (my first version of those tests failed for this reason). The cost of idle runners polling Upstash is unmeasured.
- **A poison job ends with no verdict.** After 3 deliveries it moves to the dead-letter stream, and nobody tells the user. Phase 4's API must watch that stream and mark such submissions `IE`.
- **Leftover job folders** after a runner is killed; nothing cleans them yet.
- **Problems come from a local folder**, not object storage, until Phase 5.
- **Shared secret in chat.** The Upstash token you pasted is in two git-ignored `.env` files only; rotating it in the Upstash console after the phase is wise.
- We would revisit these if a fleet shows high Upstash command counts, or if root becomes a problem for provisioning.

## Review questions
Understanding:
1. Why do we kill-test with a *Go* submission, and what would the test prove less convincingly if the job finished in 50 ms?
2. A runner is judging a long job and its network drops for a minute. What happens to the job, and what does the original runner do when the network comes back? Which two mechanisms make this safe?
3. What is the difference between "the job was delivered twice" and "the verdict was recorded twice", and which code makes the second one impossible?
4. Why does `Touch` check who owns the job inside Redis instead of just resetting the timer?
5. A job is submitted whose problem name is `../../etc`. Walk through what the runner does and what the user sees.

Decisions for you:
- **A.** Upstash: do you want to rotate the token you pasted in chat now (a Console click), and should I keep using the same database for tests, or create a second one so tests never share data with production?
- **B.** Leftover job folders: sweep them when a runner starts (simple, small risk if two runners share a host) or give each runner its own work folder (cleaner, slightly more code)? I recommend the second, in Phase 6.
- **C.** Phase 2's open decisions (A: problem format, B: compile errors on Submit, C: Go and Java compile speed) are still on the defaults. Phase 4 needs B (the API stores what the runner returns).

## Review Q&A
Review skipped at the owner's request (2026-10-02: after the report and summary were pushed, the owner wrote "merge to main").

**Understanding questions 1 to 5: not answered by the owner.** After the questions, the owner pasted them back with "ans"; Claude read that as a request for the answers and gave its own explanations in chat (they are not the owner's answers and do not show what the owner understands). The explanations were: (1) a Go compile takes 10 to 20 s, so the kill is provably mid-job, while a 50 ms job could finish before the kill and never exercise reclaim; (2) heartbeat calls fail and are retried, after `MinIdle` another runner reclaims, and the old runner's next `Touch` gets `ErrLost` and discards its work, with idempotent `Publish` as the second safety; (3) delivery is at-least-once, the verdict is exactly-once through the atomic `SET NX` plus `XADD` script and the `Published` check; (4) `XCLAIM` with min-idle 0 would reassign the job to a slow runner, so the owner check and claim are one Lua script; (5) the slug check fails, the engine never runs, the runner publishes `IE` with no text and acknowledges, the user sees only `IE`. They can serve as the recap question at the start of the next session.

**Decisions A, B and C: not answered.** Defaults stay in force: the Upstash token is not rotated and tests share the one Upstash database (the owner may change this); leftover job folders are not swept (deferred to Phase 6, per-runner work folder recommended); Phase 2 decisions A, B and C keep their defaults.

## Open decisions
- Phase 2 decisions A, B, C remain on their defaults (B affects Phase 4, A affects 7 and 8, C affects 6 and 13).
- Review decisions A, B, C above: not answered at the review (skipped); defaults stay until the owner decides.
- Runner privilege model: Phase 6.

## Handoff
- **State:** branch `phase/3-queue-runner`, tag `phase-3-start`; Phase 3 is merged into `main` and tagged `phase-3-done`; `phase-3-start` marks the start. The branch `phase/3-queue-runner` is kept. Everything is pushed to `origin`. The EC2 instance `leetforce-dev` is running (billable); its checkout is on this branch; Redis is running there; `~/Leetforce/.env` holds the Upstash URL (mode 600). No runners are left running. `web/AGENTS.md` and `web/CLAUDE.md` are still untracked.
- **Next phase:** 4 - API and database. Goal: accept submissions through a Gin API, store them and the verdicts in Neon Postgres with idempotent verdict writes, record the test-set version, and read the results stream. Before it starts: have the Neon connection string ready (`DATABASE_URL`, never committed), and decide Phase 2 decision B (compile errors on Submit).
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-3-summary.md, then start Phase 4 (API and database). Ask me the recap question and show me the session plan before writing any code.
  ```

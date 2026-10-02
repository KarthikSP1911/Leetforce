# Phase 4 summary: API and database

## TL;DR
- LeetForce now has a real front door. You can `POST` code to an API, it is saved in a database (Neon Postgres) and put on the queue, and you can `GET` the submission later to see whether it is still `queued` or `judged`, and its verdict, runtime and memory.
- The verdict is stored **exactly once**. If the same verdict arrives twice, or a different one tries to overwrite it, nothing changes. A job that crashes every runner now ends with an `IE` (internal error) verdict instead of silently disappearing.
- Hidden test data never leaves the server: the API returns only the sample tests and never the source, the test-set version, or error text from inside.

## Where this phase fits
```
 browser ---> API ---------> Redis queue ---> runner ---> judge engine ---> sandbox ---> measured facts
 [shell]     [ADDED NOW]    [Phase 3]        [Phase 3]    [Phase 2]        [Phase 1]    [Phase 1]
 Phase 0       |                                |
               | stores the submission          | publishes the verdict
               v                                v
         Postgres (Neon)  <--- API ingest <--- Redis results stream
         [ADDED NOW]          [ADDED NOW]      [Phase 3]

 dead-lettered jobs --> API ingest --> IE verdict   [ADDED NOW]

 still to come: live status over SSE and the "Judging" state (Phase 5), object storage for tests (5),
                stronger isolation (6), the website talking to the API (7, 8), users and rate limits (9)
```
The numbered, file-level version is in [docs/FLOW.md](../FLOW.md), section 3, "Phase 4".

- **Depends on Phase 3:** the API only puts jobs on the queue and reads the results stream; it does not need to know which runner did the work or how.
- **Depends on Phase 2:** the API loads problems with the judge's own loader, so it computes the same test-set version the runner will use.
- **Unblocks Phase 5:** live status needs rows to read and a queue to watch; both exist now.

## What I built and why

### Database schema (`feat/4-migrations`)
- **What:** `api/migrations/00001_init.sql` creates three tables: `problems`, `submissions` and `verdicts`. `make migrate-up` applies it with a tool called goose.
- **Why:** without a database there is nowhere to keep a submission or its result once Redis forgets it. The database also enforces rules the code could forget: only four languages, only the eight verdicts, only one verdict per submission.
- **How it works:** a *migration* is a numbered SQL file that moves the database from one shape to the next, with a matching "down" step to undo it. goose remembers which ones were applied. I ran it against your empty Neon database, undid it and re-applied it.
- **Alternatives considered:** golang-migrate (equivalent), an ORM (more machinery than six queries need). See [ADR 0010](../adr/0010-neon-access-migrations-and-test-schemas.md).

### The API skeleton (`feat/4-api-skeleton`)
- **What:** a new Go program in `api/` using the Gin web framework. `/healthz` says "the process is alive" without touching anything; `/readyz` checks Postgres and Redis.
- **Why:** an orchestrator or a load balancer needs to ask "is this instance able to serve traffic?" Liveness and readiness are different questions.
- **How it works:** the database pool (`api/internal/store/store.go`) closes idle connections after 30 seconds because Neon puts an idle database to sleep, and uses a query mode that works behind Neon's connection pooler. Errors from inside are logged, never sent to the caller, because they can contain host names.

### Problems endpoints (`feat/4-problems-endpoints`)
- **What:** `GET /problems` and `GET /problems/:slug`. At startup the API reads the `problems/` folder and copies each problem's title, difficulty, tags and test-set version into Postgres.
- **Why:** the website needs a list; submissions need a problem to point at.
- **How it works:** the detail page returns the **sample** tests only. Hidden tests are loaded (the version is computed from them) but there is no code path that returns them, and a test checks that only the samples come out.

### Submissions (`feat/4-submissions`)
- **What:** `POST /submissions` and `GET /submissions/:id`.
- **Why:** this is what the browser will call when you press Submit.
- **How it works:** the API checks the language and size, then saves the row **and stamps it with the problem's current test-set version in the same SQL statement**, then puts the job on the queue. If the queue is down it removes the row and says "503, try again", so a row never waits for a job that was never queued. A malformed id is a clean "not found", not a database error.

### Storing verdicts exactly once (`feat/4-verdict-ingest`)
- **What:** a loop inside the API that reads the results stream and writes verdicts to Postgres, plus a check on the dead-letter stream.
- **Why:** this is the exit criterion. Verdicts can arrive twice for ordinary reasons (a crash and a redelivery, two API instances), so the system must tolerate duplicates instead of hoping they never happen.
- **How it works:** one SQL statement inserts the verdict and says "if this submission already has one, do nothing", and only if it really inserted does it mark the submission `judged`. The "already has one" check is the database's primary key, which is the one thing that cannot be raced. Around it: a duplicate is acknowledged and ignored; a temporary database problem leaves the entry in the queue to be retried; an entry that can never be stored (bad data) is logged and dropped so it cannot block the line. Every 30 seconds the loop also looks at the dead-letter stream and records `IE` for each job the runners gave up on.
- **Alternatives considered:** check-then-insert in code (racy), "last write wins" (a stale duplicate could overwrite the truth), the runner posting over HTTP. See [ADR 0009](../adr/0009-idempotent-verdict-ingest.md).

### The end-to-end test (`test/4-idempotency`)
- **What:** `make test-api-e2e` runs the real API and a real runner against Neon and Upstash: submit, wait for AC, inject a fake conflicting WA into the stream, check nothing changed, then simulate a dead-lettered job and check it becomes `IE`.
- **Why:** unit tests prove the pieces; this proves they fit. I also broke the code on purpose (changed "do nothing" to "update") and both the database test and this test failed, then I restored it. A test that has never failed proves little.
- **Security scan:** the first full Trivy scan found 16 serious known vulnerabilities in three library dependencies of the API (pulled in by gin and pgx). I upgraded them and the scan is clean.

## How it works now, step by step
A correct Python solution for `sample-sum`, with one runner and the API running:
1. `POST /submissions` with `{problem, language, source}`. The API checks language, size and that the problem exists.
2. The API inserts a row in `submissions` with status `queued` and the problem's current test-set version, and gets a new random id.
3. The API adds a job to the Redis `jobs` stream and answers `202 {"id": ..., "status": "queued"}`.
4. The runner takes the job, judges it in the sandbox (Phases 1 to 3) and publishes the verdict to the `results` stream. Redis keeps only the first verdict per submission.
5. The API's ingest loop reads that entry, runs the one-statement insert, the database stores the verdict and flips the submission to `judged`, and the loop acknowledges the entry.
6. `GET /submissions/:id` now returns `status: judged` and `verdict: {AC, runtime_ms, memory_kb, passed, total}`.
If a second verdict for the same id arrives at step 5, the insert does nothing and the entry is acknowledged.

## Key concepts
- **Postgres / Neon:** a relational database. Neon is a hosted Postgres that can pause itself when idle, which is why our connection pool closes idle connections early.
- **Migration:** a versioned SQL file that changes the database shape; goose applies them in order and can undo them.
- **Primary key and constraint:** a rule the database itself enforces (for example "one row per submission id"). LeetForce relies on the verdicts primary key to make duplicates impossible.
- **Idempotent write:** doing it twice has the same effect as doing it once. Required because the queue can deliver twice.
- **`ON CONFLICT DO NOTHING`:** "insert, but if the key exists, quietly skip". It is atomic, so two writers racing cannot both win.
- **Consumer group (the API's own):** the API reads the results stream under its own group name, separate from the runners' group, so the two do not steal from each other.
- **Dead letter:** where the queue puts a job after too many failed deliveries; the API now turns those into `IE`.
- **Connection pooler (pgbouncer):** a middleman that shares database connections between many clients; it limits some Postgres features, which is why we pick a compatible query mode.
- **Throwaway schema:** a temporary, separate set of tables inside the same database. Database tests use one so they never touch the real data.
- **Readiness vs liveness:** "the process is up" versus "the process can do its job".

## Try it yourself
On the EC2 host (`ssh leetforce-dev`; start the instance first if it is stopped and update `HostName` in `C:\Users\karth\.ssh\config`). Both secrets are already in `~/Leetforce/.env`.
```bash
cd ~/Leetforce && git fetch && git checkout phase/4-api-database && git pull
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
set -a; . ./.env; set +a              # load the secrets; without this the database tests are skipped, not run
make migrate-status                 # shows 00001_init.sql applied (Neon already has it)
make fmt lint test                  # 0 issues, all ok
make test-api-e2e                   # about a minute; ends with PASS: AC stored ... a dead-lettered job became IE

# by hand:
export LEETFORCE_QUEUE_PREFIX=mydemo LEETFORCE_API_ADDR=127.0.0.1:18080 LEETFORCE_PROBLEMS_DIR=$PWD/problems
make build-api build-runner
bin/api &                                            # JSON logs; "problems synced", "api listening"
sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR bin/runner &
curl -s 127.0.0.1:18080/readyz                       # {"checks":{"database":"ok","redis":"ok"}}
curl -s 127.0.0.1:18080/problems/sample-sum          # two samples, never the three hidden tests
curl -s -X POST -H 'Content-Type: application/json' -d '{"problem":"sample-sum","language":"python","source":"print(1)"}' 127.0.0.1:18080/submissions
# copy the id, then: curl -s 127.0.0.1:18080/submissions/<id>   (queued at first, then judged; print(1) is not the right answer, so expect a verdict other than AC)
sudo -n pkill -TERM -x runner; pkill -TERM -x api; bin/lfq destroy
```
Afterwards: `pgrep nsjail` prints nothing. The runs leave a few rows in the real Neon tables (see Trade-offs).

## Trade-offs and risks
- **First verdict wins.** A later, different verdict for the same submission is ignored. That is what we want for duplicates, but a deliberate rejudge (Phase 10) will need its own operation.
- **No users yet.** Anyone who can reach the API and knows a submission id can read its status. The ids are random, and the response never contains the source or test data, but real protection is Phase 9.
- **A crash between "save the row" and "queue the job"** would leave a `queued` row that no runner will ever see. Rare, handled in Phase 5.
- **Polling cost.** The API polls Upstash like the runners do (an estimated 35,000 commands a day while idle). Not measured against your bill.
- **The results stream keeps growing** because we acknowledge entries but do not delete them (so `lfq` still works). A retention rule comes with observability (Phase 11).
- **Test leftovers in the real database:** 1 problem, 6 submissions and 5 verdicts from my live runs. Harmless, but there is no cleanup command yet.
- **Secrets pasted in chat.** The Neon connection string (this phase) and the Upstash token (Phase 3) are in chat history; rotating both passwords is wise. I also have not checked your Neon plan or its limits.
- We would revisit these if the API's polling shows up on the Upstash bill, or when real users arrive (Phase 9).

## Review questions
Understanding:
1. Why is the verdict write one SQL statement with `ON CONFLICT DO NOTHING` instead of "look up, then insert if missing"? What could go wrong with two API instances if it were done in two steps?
2. A runner publishes a verdict and crashes before acknowledging the job. Walk through what happens next and name each layer (Redis, database) that stops a second verdict from being stored.
3. A job crashes every runner that touches it. What does the user eventually see, and which parts of the code make that happen?
4. Why do we stamp the test-set version on the submission when it is accepted, instead of reading the problem's current version when the verdict arrives?
5. The database is asleep (Neon paused) when the ingest loop tries to store a verdict. What happens to that verdict? What would happen instead if the entry were invalid data?

Decisions for you:
- **A.** Secrets and database: do you want to rotate the Neon password (and the Upstash token) now, and should I plan for a separate Neon branch for tests? Which Neon plan are you on?
- **B.** Test leftovers: should I add a cleanup command (for example `make clean-test-data` that deletes only rows created by the e2e test), or leave the rows?
- **C.** Phase 5 needs MinIO (object storage for test data). Docker is still not installed on the EC2 host and Docker Desktop was off on your PC. Where should MinIO run for Phase 5: Docker on the EC2 host (I install it), Docker Desktop on your PC, or skip MinIO until S3 in the cloud?

## Review Q&A
**Understanding questions 1 to 5: not answered yet** (the owner answered only the decisions below; recorded 2026-10-02).

**Decisions (the owner's words: "dont rotate, b and c your wish"):**
- **A (owner):** do not rotate the Neon password or the Upstash token. The secrets stay as they are; they remain only in the git-ignored `.env` files. The owner did not say which Neon plan is in use, so the plan and its limits remain unchecked, and no separate Neon test branch was requested, so tests keep using throwaway schemas.
- **B (delegated to Claude, decided by Claude):** leave the test rows (1 problem, 6 submissions, 5 verdicts) and add no cleanup command now. They are small and harmless; a cleanup target is worth adding once the e2e test runs often or Phase 5 adds more writes.
- **C (delegated to Claude, decided by Claude):** run MinIO in Docker on the EC2 dev host in Phase 5, installed from Docker's official apt repository (a host change, logged and added to `scripts/setup-dev-host.sh`). Reason: the runner, API and Redis already live there, and the end-to-end flow must work on one machine (the Phase 5 exit criterion); Docker Desktop on the owner's PC would add a network hop and depend on the PC being on. It adds no new cloud cost. The `docker-compose.yml` from Phase 3 is finally verified then. Changeable at the start of Phase 5.

## Open decisions
- Decided at the review (see Review Q&A): A no rotation (owner), B leave the rows, C MinIO in Docker on the dev host (Claude). Still open: the Neon plan and limits.
- Phase 2 decisions A (problem format; Phases 7 and 8) and C (Go and Java compile speed; Phases 6 and 13) stay on their defaults. Decision B (compile errors on Submit) is settled for now: only the `CE` label is stored and returned.
- Runner privilege model: Phase 6.

## Handoff
(To be filled in after the review and merge.)

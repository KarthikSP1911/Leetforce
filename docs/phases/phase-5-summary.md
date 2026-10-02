# Phase 5 summary: Live status and storage

## TL;DR
- A submission now runs **end to end on one machine with live status**: `POST` the code, open `curl -N .../submissions/<id>/events`, and watch `queued`, then `judging`, then the verdict arrive, without polling.
- The hidden tests now live in **a storage bucket** (S3 style) instead of a folder beside the runner. A runner fetches exactly the version of the tests the submission was accepted against and refuses anything else. This is the groundwork for runners on other machines and for rejudging after a test fix.
- **Submit still never leaks hidden data**, and now there is a test that proves it: programs that echo the hidden input and a secret marker everywhere they can (output, errors, the compiler) are submitted, and every answer the API gives is searched for them. Also, a submission that was saved but never queued (the API crashed in between) is now noticed and queued by a **reaper**.

## Where this phase fits
```
 browser ---> API ----------> Redis queue ---> runner ----> judge engine ---> sandbox ---> measured facts
 [shell]      [Phase 4,       [Phase 3]        [Phase 3,     [Phase 2]        [Phase 1]
 Phase 0       +SSE NOW]                        +fetches
                 |  ^                           tests NOW]
   stores row    |  | live status (SSE): queued -> judging -> verdict      [ADDED NOW]
                 v  |                              ^
          Postgres (Neon) <--- API ingest <--- Redis: results + status streams     [status stream ADDED NOW]
                 ^
                 | reaper re-queues rows that never got a job              [ADDED NOW]

  S3 bucket (RustFS locally) <--- API publishes tests at startup ... runner fetches them per job   [ADDED NOW]

 still to come: stronger isolation (6), the website using all this (7, 8), users and rate limits (9), problem pipeline (10)
```
The numbered, file-level version is in [docs/FLOW.md](../FLOW.md), section 3, "Phase 5".

- **Depends on Phase 4:** it needs submissions stored in Postgres, a results stream to watch and the test-set version stamped on every submission.
- **Depends on Phases 2 and 3:** the judge's problem loader and version hash are reused to check the downloaded tests; the runner loop is where the "judging" report goes.
- **Unblocks Phases 7 and 8:** the website can now call real endpoints, including the live stream, and Phase 6 can harden the sandbox knowing the whole flow works.

## What I built and why

### Docker and a local object store (`feat/5-dev-host-docker`)
- **What:** Docker installed on the EC2 dev host, and a local S3-compatible storage server (RustFS) started from `docker-compose.yml`.
- **Why:** the tests need a bucket to live in. The plan said MinIO, but MinIO no longer publishes its container images (the pull was refused on the host), so I chose RustFS, which speaks the same S3 language. The code only uses the S3 language, so the server can be swapped (Garage, or real Amazon S3 in the cloud) without code changes.
- **How it works:** `docker compose up -d s3`; credentials come from `LEETFORCE_S3_*` in the git-ignored `.env`. The image was scanned with Trivy (0 serious findings) and pinned to version 1.0.0.
- **Alternatives considered:** Garage (more setup), building MinIO from source (heavy), skipping storage until the cloud (leaves Phase 5 half done). [ADR 0011](../adr/0011-problem-tests-in-object-storage.md).

### Tests in the bucket (`feat/5-test-storage`)
- **What:** a "bundle" is one compressed file holding a problem's `problem.yaml` and tests. The API uploads one per problem at startup under the name `<problem>/<test-set-version>`. Each job now carries the version, and the runner downloads that bundle.
- **Why:** a runner on another machine cannot see your `problems/` folder, and a folder only holds the *current* tests, so you could never judge against the tests a submission was accepted with.
- **How it works:** `judge/problem/bundle.go` packs deterministically (same input, same bytes) and unpacks defensively (only two kinds of path, no links, size cap). `runner/internal/problems` downloads, unpacks, **recomputes the version from the tests and refuses a mismatch**, and keeps a cache named after the bundle's fingerprint (not its version, because the version ignores time limits, so limits can change without it). The `storage/` module is the small S3 client wrapper.
- **Alternatives considered:** runner fetching tests from the API over HTTP (puts hidden tests behind an endpoint), writing our own S3 signing code (security-sensitive and unnecessary).
- **One security-related change to look at:** the S3 library brings in two helpers that mention a Go standard-library package named `database/sql/driver` (only a type definition, it cannot open a connection). The test that keeps runners away from the database now allows exactly those two interface-only packages and still forbids the real database package, every driver and the API framework (ADR 0011).

### The "judging" state (`feat/5-judging-state`)
- **What:** when a runner takes a job it drops a short note in a new Redis stream, `status`; the API reads it and sets the submission to `judging` in Postgres.
- **Why:** only the runner knows when judging starts, and runners must not touch the database.
- **How it works:** `queue/status.go` (publish and read), `runner/internal/agent/agent.go` (publish before judging; if that fails the job continues), `api/internal/ingest/status.go` (apply it). The database update only moves `queued` to `judging`, so a late note can never overwrite a verdict. The status stream is read with a simple "read from where I left off" call instead of a consumer group: nothing to acknowledge, nothing that can get stuck, fewer Redis commands.
- **Alternatives considered:** runner calls the API (needs credentials on every runner), a consumer group (retries are pointless for a note that is useless when late). [ADR 0012](../adr/0012-live-status-sse-and-reaper.md).

### The live stream (`feat/5-sse-status`)
- **What:** `GET /submissions/:id/events` is a Server-Sent Events stream, which is a normal web response that stays open and sends small text events.
- **Why:** the website (Phase 8) should show progress without hammering the API.
- **How it works:** `api/internal/server/events.go`. The first event is the current state; after that one event per change; the last is the verdict, then the stream closes. It reads Postgres every half a second, so any API instance can serve it. It sends a keep-alive comment every 15 seconds, stops after 10 minutes, caps open streams at 200 per instance, and never sends the source, the tests, the version or internal error text.
- **A surprise that is now documented:** it streams *state*, not *history*. If a submission is judged faster than half a second it can jump from `queued` straight to the verdict. My first end-to-end test assumed `judging` would always show up, and failed once because the tiny sample problem is judged in about 300 ms. I fixed the test (it now submits a correct but slower solution), not the behaviour; clients must show whatever state they last saw.
- **Alternatives considered:** an in-memory broadcast (only reaches users connected to the instance that heard the event). [ADR 0012](../adr/0012-live-status-sse-and-reaper.md).

### The reaper (`feat/5-reaper`)
- **What:** a background loop that re-queues a submission that was saved but never queued.
- **Why:** the API saves the row, then queues the job. If it dies between the two, the row says `queued` forever with no job (the gap noted at the end of Phase 4).
- **How it works:** a new column `enqueued_at` is set after a successful queue; rows that are old, still `queued` and have no `enqueued_at` are the orphans. `store.ReapUnqueued` selects them in one transaction with `FOR UPDATE SKIP LOCKED` (two API instances never grab the same row), queues each and marks it; a crash before the commit just rolls back. The worst case is one duplicate job, which is harmless because a runner skips a job that already has a verdict and verdict writes are idempotent.
- **Cost trade-off:** it sweeps once at startup (that is when a crash is noticed) and then only every 15 minutes, because a more frequent database query would keep the Neon database awake and may cost money.

### The end-to-end test (`test/5-e2e-redaction`)
- **What:** `make test-live-e2e` (about 45 seconds). It proves the three things above in order: the live stream, the redaction and the reaper.
- **Why:** unit tests prove pieces; this proves they fit, and that the exit criteria hold.
- **How it works:** the runner it starts has no problems folder and no database address, so its tests can only come from the bucket. The redaction part first proves its own detector works (it plants a secret and requires it to be found), then submits an echoing program, a compile error carrying the marker, and a wrong answer, and searches every response for the marker, pieces of every hidden test, the source and a version number. It deletes the rows it created.
- **I also tried to break it on purpose:** exposing the test-set version in the JSON made the test fail with the right message; two other deliberate breakages made the matching unit tests fail. A test that has never failed proves little.

## How it works now, step by step
A correct Python solution for `sample-sum`, RustFS, Redis, the API and one runner running:
1. At startup the API loads `problems/`, saves the problem rows in Postgres, packs each problem and uploads its bundle to the bucket, and sweeps for orphans.
2. `POST /submissions`: the API checks the input, saves a row with the problem's current test-set version, queues the job (it now includes that version), marks the row as queued, and answers `202` with an id.
3. You open `curl -N /submissions/<id>/events`; the first event says `queued`.
4. A runner takes the job and writes a `judging` note to the status stream, then asks the bucket for the bundle of that version (using its cache if the fingerprint matches), checks the tests hash to the same version, and judges in the sandbox.
5. The API's status watcher sees the note and sets the row to `judging`; your stream, polling every half second, sends `status: judging`.
6. The runner publishes the verdict; the API ingest stores it once (Phase 4), the row becomes `judged`, and your stream sends `verdict` with runtime, memory and `passed/total`, then closes.
7. Nothing in any of those messages contains the source, the hidden tests, the version or error text.

## Key concepts
- **Object storage / S3:** a service that stores files ("objects") in "buckets" and hands them back by name over HTTP. LeetForce needs it so runners on other machines can get test data.
- **Bundle:** one compressed file with a problem's definition and tests. Packing is deterministic so the same content always makes the same bytes.
- **SHA-256 fingerprint (hash):** a short value computed from a file's bytes; any change changes it. Used to name the runner's cache folders.
- **SSE (Server-Sent Events):** a response that stays open and sends text events; simpler than WebSockets when only the server talks.
- **State vs history:** the stream says where a submission is now, not every step it took.
- **`FOR UPDATE SKIP LOCKED`:** a Postgres way to say "lock the rows I pick and let other workers pick different ones", so two reapers do not collide.
- **Best effort:** a message that is allowed to get lost because losing it only makes the display slightly late, never wrong.
- **Mutation check:** breaking the code on purpose to confirm a test fails.

## Try it yourself
On the EC2 host (`ssh leetforce-dev`; start the instance first if it is stopped and update `HostName` in `C:\Users\karth\.ssh\config`). All secrets are already in `~/Leetforce/.env`.
```bash
cd ~/Leetforce && git fetch && git checkout phase/5-live-status-storage && git pull
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
set -a; . ./.env; set +a                    # without this the database and storage tests are skipped, not run
docker compose --env-file .env up -d s3      # RustFS; curl -s -o /dev/null -w '%{http_code}\n' 127.0.0.1:9000/health  -> 200
make migrate-status                          # versions 1, 2 and 3 applied (Neon already has them)
make fmt lint test                           # 0 issues, all ok
make test-live-e2e                           # about 45 s; ends with PASS: queued -> judging -> verdict over SSE ...

# by hand, to watch the stream yourself (needs bin/api and bin/runner built: make build-api build-runner):
export LEETFORCE_QUEUE_PREFIX=mydemo LEETFORCE_API_ADDR=127.0.0.1:18080 LEETFORCE_PROBLEMS_DIR=$PWD/problems
bin/api &                                    # JSON log lines: "problem bundles published", "api listening"
python3 - <<'PY' > /tmp/body.json
import json; print(json.dumps({"problem":"sample-sum","language":"python","source":open("problems/sample-sum/solutions/python/ac.py").read()}))
PY
ID=$(curl -s -X POST -H 'Content-Type: application/json' -d @/tmp/body.json 127.0.0.1:18080/submissions | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -sN 127.0.0.1:18080/submissions/$ID/events &          # shows: event: status / data: {"status":"queued"}
sudo -n --preserve-env LEETFORCE_PROBLEMS_DIR=/nonexistent bin/runner &   # no problems folder: tests come from the bucket
# the stream then prints judging and the verdict. Clean up:
sudo -n pkill -TERM -x runner; pkill -TERM -x api; pkill curl; bin/lfq destroy
```
Afterwards: `pgrep nsjail` prints nothing. The manual run leaves a row or two in the real Neon tables; `make test-live-e2e` deletes its own.

## Trade-offs and risks
- **`judging` can be skipped** for very fast judgements, by design. The Phase 8 website must render the last state it saw.
- **Database polling per stream:** every open stream reads Postgres twice a second; the cap of 200 per instance bounds it. If it shows up in load, a Redis-based push is the next step (Phase 11).
- **RustFS is young software.** If it misbehaves, swapping to Garage or real S3 is an environment change.
- **One storage password for both the API and the runner** for now; read-only runner credentials arrive with infrastructure as code (Phase 12).
- **Costs not measured:** the status loop adds roughly 17,000 Redis commands a day on Upstash, and the reaper wakes Neon about every 15 minutes. I have not seen either bill, and the Neon plan is still unchecked.
- **A guard test was narrowed** (the two interface-only database packages). It is a security rule, so it deserves your explicit agreement (decision B).
- **Test leftovers** in the real database: 1 problem, 31 submissions, 30 verdicts from earlier runs; `make test-api-e2e` still writes 2 rows per run without cleaning up.
- We would revisit these if a bill or a quota shows a surprise, or when real users arrive (Phase 9).

## Review questions
Understanding:
1. Why does each job carry the `test_set_version`, and what does the runner do if the bundle it downloads turns out to hold a different version? What could go wrong if the runner simply read the `problems/` folder?
2. Why is the runner's cache folder named after the bundle's fingerprint (hash) and not just the test-set version?
3. A runner crashes right after writing the `judging` note, or the note is lost. What does the user see, and why is that acceptable? What stops a late `judging` note from overwriting a verdict that is already stored?
4. The live stream can jump from `queued` straight to the verdict. Why is that not a bug, and what did it teach us about how to write the end-to-end test?
5. The API crashed after saving a row but before queuing its job, and two API instances run the reaper at the same time after restart. Walk through how the job ends up queued once (or harmlessly twice): which layers prevent a double verdict?

Decisions for you:
- **A.** RustFS instead of MinIO for local storage (MinIO no longer ships images). Keep RustFS, or would you prefer Garage, or to wait and use real S3?
- **B.** The runner's "no database" test now allows exactly two interface-only standard-library packages (`database/sql/driver`, `database/sql/internal`) because the S3 client needs them. Do you accept that narrowing, or should I replace the S3 library with our own minimal client to keep the guard strict?
- **C.** Cost checks: can you look at your Neon plan limits and Upstash usage, and is a 15-minute reaper interval (one database wake-up per interval) fine for you?

## Review Q&A
**Understanding questions 1 to 5: not answered; the review was skipped at the owner's request** (2026-10-02: the owner replied "skip"). They can serve as the recap question at the start of the next session.

**Decisions A, B and C: not answered.** Claude's recommendations stand as the defaults and stay open to change: A, RustFS 1.0.0 for local storage; B, the runner no-database guard narrowed to allow `database/sql/driver` and `database/sql/internal`; C, reaper every 15 minutes with the Neon plan and Upstash usage still unchecked.

## Open decisions
- A, B and C above are unanswered and on Claude's defaults (A and B matter from Phase 6; C before real traffic).
- Carried over: the Neon plan and limits (unchecked); runner privilege model (Phase 6); Phase 2 decisions A (problem format; Phases 7 and 8) and C (Go and Java compile speed; Phases 6 and 13) on their defaults; `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Handoff
- **State:** Phase 5 is merged into `main` and tagged `phase-5-done` and `M2` (`phase-5-start` marks the start); the branch `phase/5-live-status-storage` is kept. Everything is pushed to `origin`. The EC2 instance `leetforce-dev` is still running (billable; stopping it is the owner's call); its checkout is on `test/5-e2e-redaction`, so run `git fetch && git checkout main && git pull` there first; Docker is installed and the RustFS container (`leetforce-s3-1`) is running; a stash on the host holds stale Phase 4 copies. No API or runner is left running. Neon is at goose version 3 with test leftovers (1 problem, 31 submissions, 30 verdicts).
- **Next phase:** 6 - Sandbox hardening. Goal: raise isolation confidence: evaluate gVisor vs nsjail with measurements (ADR), tune seccomp, grow the adversarial suite; the suite must pass on the chosen sandbox. Think about before the session: how much complexity and slowdown you accept for gVisor on the small x86 host, and the runner privilege model (runners run as root today).
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-5-summary.md, then start Phase 6 (Sandbox hardening). Ask me the recap question and show me the session plan before writing any code.
  ```

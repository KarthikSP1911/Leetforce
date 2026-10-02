# Phase 3: Queue and runner

**Branch:** `phase/3-queue-runner`
**Range:** `phase-3-start..phase-3-done`
**Dates:** 2026-10-02 to 2026-10-02
**Milestone:** none
**Working log:** [phase-3-log.md](phase-3-log.md) (every command, result and mistake)

## Summary
Jobs now travel from a producer through Redis Streams (Upstash) to runner processes. A runner pulls a job, judges it with the Phase 2 engine in the Phase 1 sandbox, reports the verdict to a results stream and acknowledges the job. If a runner dies mid-job, another runner reclaims the job after an idle window and exactly one verdict is recorded. The runner module has no database dependency, and a test enforces it. Phase 4's API will enqueue into and read from these streams.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Killing a runner mid-job results in the job being reclaimed and judged once | ✅ | `make test-crash` on the dev host, Upstash, real sandbox: runner A `kill -9` during a Go compile, runner B reclaimed (delivery 2, `reclaimed: true`), exactly one verdict `AC` from `runner-b`, still one after waiting 8 s more, B stopped cleanly on SIGTERM. Passed on commit `a04444d` and again on the merged head `a43f833`. Unit level: `TestAbandonedJobIsReclaimedByAnotherConsumer`, `TestPublishIsIdempotentPerSubmission`, `TestLostClaimCancelsJudgingAndDiscardsResult`, `TestRedeliveredJobWithRecordedVerdictIsNotJudgedAgain` |
| No DB dependency in `runner/go.mod` | ✅ | `go test ./runner/` → `TestRunnerHasNoDatabaseDependency` PASS (checks `go.mod` and `go list -deps`); shown to fail when a package the runner uses is forbidden |
| Build item: `runner/` module | ✅ | `runner/go.mod`, `runner/cmd/runner`, `runner/internal/agent`; `make build-runner` |
| Build item: Redis Streams consumer groups on Upstash (`LEETFORCE_REDIS_URL`) | ✅ | `queue/queue.go`; `go test ./queue/` passes on local Redis (1.3 s) and on Upstash (39 s, 10 of 10) |
| Build item: `XAUTOCLAIM` for crashed runners | ✅ | `Queue.Receive`; reclaim and poison-job tests |
| Build item: runner talks only to Redis and the API | ⚠️ partly | The runner talks only to Redis. The API does not exist until Phase 4, so verdicts go to the `results` stream, which the API will read (ADR 0008) |
| Build item: Docker Compose for Redis/MinIO | ⚠️ written, not run | `docker-compose.yml` exists; Docker is not installed on the dev host and Docker Desktop was stopped on the Windows machine, so `docker compose config` was never run |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/3-compose` | Compose file for Redis and MinIO, env example, `make dev`/`down` | 2 (1 build, 1 docs) |
| `feat/3-queue-package` | `queue/` module: stream, group, reclaim, heartbeat, dead letter, idempotent publish | 3 (1 feat, 1 test, 1 docs) |
| `feat/3-runner-module` | `runner/` module, agent loop, `Published`, `Destroy`, unsigned memory | 4 (2 feat, 1 fix, 1 docs) |
| `feat/3-lfq-tool` | `lfq` tool, queue prefix env, crash-reclaim test and its two script fixes | 4 (1 feat, 1 test, 2 fix) |
| `test/3-no-db-dependency` | build-level check that the runner has no database dependency | 1 |
| `docs/3-adr-flow` | ADR 0008, FLOW.md, log for units 4 and 5 | 1 |

Also on the phase branch directly: `b7cf4e8` (open the phase log and PROGRESS), plus this report and the summary.

## File-by-file changes

Generated with `git diff --name-status phase-3-start..HEAD`.

### Added
| File | Purpose |
|---|---|
| `docker-compose.yml` | Local Redis 7 (AOF) and MinIO, both bound to 127.0.0.1; the MinIO password must come from `.env` |
| `queue/go.mod` | Module `leetforce/queue`; only dependency go-redis v9.22.0 |
| `queue/go.sum` | Checksums for the above (generated) |
| `queue/queue.go` | The queue: `Setup`, `Enqueue`, `Receive` (autoclaim then read, dead-letters poison jobs), `Ack`, `Touch` (owner-only heartbeat, Lua), `Publish` (idempotent, Lua), `Published`, `Results`, `DeadLetters`, `Destroy` |
| `queue/queue_test.go` | 10 tests against a real Redis; idle windows scale to the measured round trip |
| `queue/cmd/lfq/main.go` | `lfq enqueue`, `results`, `destroy` (refuses the default prefix) |
| `queue/cmd/lfq/main_test.go` | usage errors and the refusal to destroy the default prefix |
| `runner/go.mod` | Module `leetforce/runner`, no third-party dependency of its own |
| `runner/cmd/runner/main.go` | Runner process: env config, root check, signal handling, JSON logs |
| `runner/internal/agent/agent.go` | The loop: receive, skip if verdict exists, heartbeat, judge, publish, ack; failure policy |
| `runner/internal/agent/agent_test.go` | 9 test functions (one has 4 sub-cases) with a fake judger and real Redis (verdict mapping, hidden data, IE cases, heartbeat, lost claim, `Run`) |
| `runner/nodb_test.go` | Fails if a database package is in the runner's `go.mod` or dependency graph |
| `scripts/test-crash-reclaim.sh` | The exit test: kill runner A mid-job, runner B reclaims, one verdict |
| `docs/adr/0008-queue-reclaim-and-runner-privileges.md` | ADR: queue design, reclaim, heartbeat, idempotent verdicts, root runner |
| `docs/phases/phase-3-log.md` | Working log |
| `docs/phases/phase-3.md` | This report |
| `docs/phases/phase-3-summary.md` | Plain-language summary and review record |

### Modified
| File | What changed | Why |
|---|---|---|
| `.env.example` | Added `LEETFORCE_TEST_REDIS_URL` and the MinIO variables | Document the new settings without values or secrets |
| `Makefile` | `GO_MODULES` now `judge queue runner`; targets `dev`, `down`, `build-runner` (builds `bin/runner` and `bin/lfq`), `test-crash` | Lint and test the new modules; run the Phase 3 exit test |
| `go.work` | Added `./queue` and `./runner` | Modules resolve each other locally (ADR 0002) |
| `scripts/setup-dev-host.sh` | Installs `redis-server` with the other packages | Queue tests need a real Redis on the host; it had been installed by hand |
| `docs/FLOW.md` | Phase 3 ticked; new "Phase 3 (as built)" section | Flow doc must match what exists |
| `docs/PROGRESS.md` | Phase 3 row and status | Phase tracker |

### Deleted
| File | Reason |
|---|---|
| `runner/.gitkeep` | The directory now has real files |

### Renamed / moved
None.

## Key code changes
1. **Owner-only heartbeat** (`queue/queue.go`, `Touch`). A Lua script checks that the entry's current owner is the caller before resetting its idle time. A plain `XCLAIM` with min-idle 0 would let a slow runner steal a job back from the runner that legitimately took it over.
   ```lua
   local p = redis.call('XPENDING', KEYS[1], ARGV[1], ARGV[2], ARGV[2], 1)
   if #p == 0 or p[1][2] ~= ARGV[3] then return 0 end
   redis.call('XCLAIM', KEYS[1], ARGV[1], ARGV[3], 0, ARGV[2], 'JUSTID')
   return 1
   ```
2. **Exactly-once verdict** (`Publish`). One Lua script sets a marker with `SET NX EX` and appends to the results stream; a second publish for the same submission returns false and writes nothing. Ack happens after Publish, and `Published` lets a redelivered job stop before judging.
3. **Poison-job guard** (`Receive` and `deliver`). A job delivered more than `MaxDeliveries` times, or one that cannot be decoded, goes to `<prefix>:jobs:dead` instead of being tried forever.
4. **Failure policy** (`agent.Process`). Host error: leave the job pending so the queue redelivers it. Bad job (invalid slug, unknown problem or language, oversized source) or last attempt: publish `IE` with no error text. Lost claim: cancel the judging and publish nothing. The problem slug is checked against a regular expression before it is used in a path.
5. **Crash test** (`scripts/test-crash-reclaim.sh`). Runs two real runner processes as root against Upstash, kills one with SIGKILL while its sandboxed Go compile is running, and checks the single verdict.

## Decisions
- [ADR 0008](../adr/0008-queue-reclaim-and-runner-privileges.md): Redis Streams with `XAUTOCLAIM`, owner-only heartbeat, idempotent verdicts through a results stream (no API needed yet); the runner runs as root for now (Phase 6 revisits).
- Taken by Claude on the owner's delegation: Upstash for the real URL, local Redis for tests; Phase 2 decisions A, B and C keep their defaults.

## Tests
- `queue/queue_test.go` (10): enqueue/receive/ack, validation, jobs enqueued before the group exists, reclaim of an abandoned job (and not before `MinIdle`), heartbeat keeps a job, `Touch` on an unknown entry, poison job to dead letter, undecodable job, idempotent publish plus `Published`, publish validation.
- `queue/cmd/lfq/main_test.go` (2 test functions: 5 usage cases, and 2 prefixes for the refusal): usage errors, refusal to destroy the default prefix.
- `runner/internal/agent/agent_test.go` (9 test functions, one with 4 sub-cases): verdict and result mapping, WA result carries no failing test, CE carries the compiler text, bad jobs get `IE` and are acked, host failure then `IE` on the last attempt, redelivery with a recorded verdict is not judged, heartbeats while judging, lost claim cancels judging, `Run` stops on cancel.
- `runner/nodb_test.go` (1): no database dependency.
- `scripts/test-crash-reclaim.sh`: the end-to-end exit test.
- Commands (on the dev host, from `~/Leetforce`): `make fmt lint test` (needs the local Redis; queue and agent tests skip without one), `make test-crash` (needs `LEETFORCE_REDIS_URL` in `.env`, sudo and nsjail).
- Not re-run: `make test-sandbox` and `make test-adversarial`; nothing under `judge/sandbox/` changed in this phase.

## Known issues and deferred work
- **Leftover job directories:** a runner killed mid-job leaves `/var/tmp/leetforce-job-*` behind (one per killed run; I deleted them by hand). Needs a startup sweep of old directories or a per-runner work root. Phase 6 (hardening) or Phase 13 (host provisioning).
- **Dead-lettered jobs have no verdict:** a job that crashes runners every time ends in `<prefix>:jobs:dead` without a result. The API must watch that stream and mark such submissions `IE`. Phase 4.
- **No systemd unit:** the runner is started with `sudo` by hand or by the test script. Provisioning (Packer, Ansible) is Phase 13.
- **Runner runs as root.** Privilege separation is a Phase 6 decision (ADR 0008).
- **Compose file unvalidated:** `docker compose config` was never run (no Docker available). Check it the first time Docker is available (Phase 5 brings MinIO into use).
- **Problems are read from a local directory** (`LEETFORCE_PROBLEMS_DIR`), not from object storage. Phase 5 (MinIO/S3).
- **Upstash cost and latency:** about 0.5 to 1 s per command from the host; command usage of idle runners is unmeasured (Phase 13).
- **The Upstash token was pasted in chat.** It is stored only in git-ignored `.env` files, but rotating it in the Upstash console after the phase is advisable (owner's action).
- Carried over: Phase 2 decisions A, B, C (defaults stay); `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Stats
- Commits: 18 (excluding merges; 16 before this report and the summary)
- Files: 17 added, 6 modified, 1 deleted
- Lines: +2047 / -10 (`git diff --shortstat phase-3-start..HEAD` at the summary commit; the later commits that record this and the review add a few lines of documentation)

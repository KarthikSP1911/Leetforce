# 0008. Redis Streams queue with reclaim, heartbeat and idempotent verdicts; the runner runs as root for now

**Status:** accepted (Phase 3). The owner delegated these choices ("you decide which is good"); they are Claude's recommendations and can be changed.

## Context
Jobs must reach a fleet of runners, and a runner can die at any moment (crash, `kill -9`, spot reclaim) while holding a job. The exit criterion is that such a job is reclaimed and judged once. Phase 3 also has to decide how the runner reports verdicts before the API exists (Phase 4), and how it gets the root rights nsjail needs (carried over from Phase 1).

## Decision
**Queue (`queue/`, module `leetforce/queue`, shared by API and runners):**
- One stream `<prefix>:jobs`, consumer group `runners`, created from ID 0 so jobs added before the first runner starts are kept.
- `Receive` first runs `XAUTOCLAIM` (jobs pending longer than `MinIdle`, default 30 s), then `XREADGROUP`. A job is acknowledged with `XACK` + `XDEL`.
- **Heartbeat:** while judging, the runner calls `Touch` every `MinIdle/3`. `Touch` is a Lua script: it resets the idle time only if the caller is still the owner, otherwise `ErrLost`. The runner then cancels the judging and discards the result, so a slow job is not stolen and a runner that lost its claim cannot overwrite the new owner's work.
- **Poison jobs:** a job delivered more than `MaxDeliveries` (default 3) times goes to `<prefix>:jobs:dead` instead of being retried, so a job that keeps killing runners cannot loop forever. A job that cannot be decoded is dead-lettered at once.
- **Verdicts:** `Publish` is a Lua script (`SET marker NX EX 7d` then `XADD <prefix>:results`), so the verdict for a submission is recorded at most once. `Published` lets a runner acknowledge a redelivered job whose verdict already exists without judging it again. Ack comes after Publish, so a crash between them is repaired by the redelivery.
- **Reporting before the API exists:** the runner writes to the `results` stream. In Phase 4 the API reads that stream with its own consumer group and writes the verdict to Postgres idempotently by submission ID. The runner needs only Redis, so it has no database or API client.
- The result carries verdict, runtime, memory, test-set version, passed and total test counts, the runner ID and (only for CE) the compiler message. It has no failing-test name, input, expected output or stderr.
- Runner failures: a host error leaves the job pending (redelivery); a bad job (invalid slug, unknown problem or language, oversized source) or a failure on the last attempt publishes verdict `IE` (platform error) with no error text; the cause goes to the runner's log only.

**Runner privileges:** the runner runs as **root** (it exits with a message otherwise). On the dev host it is started with `sudo`; a systemd unit comes with host provisioning (Packer and Ansible, Phase 13). It needs root because nsjail and the cgroup writes need it (ADR 0004). A privilege-separated design (a small root helper for the sandbox, the rest unprivileged) is deferred to Phase 6 with the gVisor evaluation.

## Alternatives
- **Postgres or another table as the queue:** the runner must not touch the database, and polling a table gives no reclaim primitive.
- **Plain Redis lists (`BRPOPLPUSH`) with a visibility timeout I build myself:** more code for the same thing; streams give pending lists, idle times and `XAUTOCLAIM` built in.
- **Runner reports over HTTP to the API:** the API does not exist yet, and a stub would be thrown away. The stream is testable now, and decouples runner from API availability.
- **No heartbeat, just a long `MinIdle`:** a long job would still be stolen once it exceeded the window, and a crash would take that long to recover. The heartbeat allows a short window (fast recovery) with unbounded job length.
- **`XCLAIM` instead of Lua for `Touch`:** `XCLAIM` with min-idle 0 steals the entry from its owner; the Lua check makes it owner-only.

## Consequences
- Measured: killing a runner during a Go compile (`make test-crash`, Upstash, `MinIdle` 6 s): the second runner started judging the job 5.9 s after it started (delivery 2, `reclaimed: true`), reported AC after 19.5 s of judging, and exactly one verdict exists, also after waiting 8 s more.
- Upstash round trips from the dev host take about 0.5 to 1 s, so `MinIdle` must be many round trips long. The default of 30 s with a 10 s heartbeat is safe; the test helpers scale their short windows to the measured round trip.
- Delivery is at-least-once, the verdict is exactly-once. A job that crashes the runner every time ends in the dead-letter stream **without a verdict**; the API must watch that stream and mark such submissions `IE` (Phase 4).
- A killed runner leaves its job directory (`/var/tmp/leetforce-job-*`) behind; nothing sweeps it yet (known issue in the phase report).
- Running as root widens the blast radius of a runner bug. It is accepted for the dev host and revisited in Phase 6; the runner must not run on a shared machine before then.
- Upstash bills per command. An idle runner polls with a blocking read (`Block` 5 s) plus an autoclaim per loop, a few commands per 5 s; this should be measured before the fleet grows (Phase 13).

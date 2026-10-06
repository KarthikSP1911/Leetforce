# 0028. Keep idle Redis polling inside a hosted command budget

Status: ACCEPTED (owner reported the Upstash free tier running out, after Phase 16)

## Context
Upstash bills per command, and a blocking read that times out with no data still counts as one.
Every long-lived process polled Redis all day even with no submissions (counts from the code, not
from the Upstash usage graph):

| Loop | Commands | Idle cost per process |
|---|---|---|
| Runner `Receive` | `XAUTOCLAIM` + `XREADGROUP BLOCK 5s`, 2 per ~5 s | ~31k a day |
| API verdict ingest | the same 2 per ~5 s, plus 2 per 30 s for dead letters | ~37k a day |
| API status watcher | `XREAD BLOCK 5s`, 1 per 5 s | ~17k a day |
| Queue-depth sampler | a 5-command pipeline every 60 s | ~7k a day |

One API and one runner left up cost about 90k commands a day, about 2.7M a month. The free plan's
limit was never checked (open item B2 in `docs/launch-checklist.md`); it is believed to be about
500k a month, so the budget ran out in under a week. Extra runners, the e2e scripts and the load
test against the real instance added more.

## Decision
- `queue.Config.ReclaimEvery` (env `LEETFORCE_JOB_RECLAIM_EVERY`, default 2 x `MinIdle` = 60 s) is
  how often a consumer looks for abandoned entries and the longest it waits for new ones.
  `Receive` and the API's `nextEntry` skip `XAUTOCLAIM` when they looked less than `ReclaimEvery` ago
  and cap the blocking read at `ReclaimEvery` (`queue/poll.go`).
- Default `Block` is 60 s in the runner, the API verdict loop and the API status loop (was 5 s). A new
  entry wakes a blocked read at once, so latency to start a job does not change.
- Dead-letter check: every 5 minutes (was 30 s). Queue-depth sampler: every 5 minutes (was 60 s,
  `LEETFORCE_METRICS_QUEUE_EVERY`).
- Blocking reads go through `untilDone`, which returns when ctx is cancelled. go-redis does not
  interrupt a read the server is holding, so with a 60 s block a SIGTERM would otherwise wait up to
  60 s (`api` waits on `<-ingestDone`). A read abandoned that way finishes in the background; an
  entry it still receives stays pending and is reclaimed after `MinIdle`.

## Alternatives
- Only raise `Block`: cheapest to write, but a crashed runner's job would wait for the next loop
  with no bound tied to `MinIdle`. Capping the block at `ReclaimEvery` keeps the recovery bound explicit.
- A reclaim interval of `MinIdle / 2` (15 s): faster recovery, but about 4x the idle commands. Set
  `LEETFORCE_JOB_RECLAIM_EVERY=15s` to get it back.
- Paid plan: Upstash pay-as-you-go would absorb the old load; not chosen without the owner's say.
- A different queue (a Postgres table, a cloud queue): not evaluated here; Redis Streams stays.

## Consequences
- Idle cost per process: runner ~2.8k a day, verdict ingest ~2.8k plus ~0.6k for dead letters,
  status watcher ~1.4k, sampler ~1.4k. One API and one runner: about 9k a day, about 0.27M a month
  (computed from the intervals, not measured).
- A crashed runner's job is picked up within `MinIdle + ReclaimEvery` (90 s by default, was about
  35 s). `scripts/test-crash-reclaim.sh` sets `MinIdle` 6 s, so its bound is 18 s; it waits up to 120 s.
- The reclaim gate only changes anything when a loop runs more often than `ReclaimEvery`, for
  example a runner finishing jobs back to back. An idle loop always takes `ReclaimEvery` plus a round trip.
- Shutdown of the API and runner is not delayed by the longer block (unit test
  `TestUntilDoneReturnsOnCancel`).
- Not measured: command counts against the Upstash usage page; do that after a day of running.
- Supersedes the polling-cost notes in ADR 0008, 0009 and 0012 and the 60 s sampler figures in
  ADR 0019 and `docs/cost-review.md`.

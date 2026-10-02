# 0012. Live status: a best-effort status stream, an SSE endpoint that polls the database, and a reaper for unqueued submissions

**Status:** accepted (Phase 5). The owner delegated the choices ("do as u wish"); they are Claude's recommendations and can be changed.

## Context
Phase 5's exit criterion is to watch Queued, then Judging, then a verdict over SSE. Only the runner knows when judging starts, and runners never touch the database or call the API (ADR 0008). Separately, Phase 4 left a gap: `POST /submissions` inserts the row and then queues the job, so a crash between the two leaves a `queued` row with no job.

## Decision
**Judging event.** The runner appends `{submission_id, state: "judging", runner_id}` to `<prefix>:status` (capped at about 5000 entries) before it judges; a failed publish is logged and the job continues. The API's `StatusWatcher` reads that stream with plain `XREAD` from a tracked ID (starting at the newest entry) and runs `UPDATE submissions SET status = 'judging' WHERE id = $1 AND status = 'queued'`. No consumer group, nothing to acknowledge, nothing to get stuck pending: one Redis command per 5 s poll, and every API instance applies every event, which is safe because the update only moves a queued submission forward (a late event after the verdict changes nothing, tested). The status is best effort: a lost event leaves a submission `queued` until its verdict arrives.

**SSE endpoint.** `GET /submissions/:id/events` reads the submission from Postgres every 500 ms and emits `status` events on change (`queued`, `judging`) and then one `verdict` event (the same view as `GET /submissions/:id`), after which it closes. The first event is the current state, so a late or reconnecting client needs no `Last-Event-ID`. Also: keep-alive comment every 15 s, `timeout` event after 10 minutes, `error` event after 10 consecutive failed reads (generic text, the cause is logged), 503 with `Retry-After` beyond 200 open streams per instance, `X-Accel-Buffering: no`.

**The stream reports state, not history.** A judgement shorter than one poll can go from `queued` straight to the verdict. That is intended and clients must handle it (the web client in Phase 8 renders whatever state it sees last). It showed up in practice: the end-to-end test failed once because the fast `sample-sum` Python solution spent only a few hundred milliseconds in `judging`; the test now submits a solution that sleeps 0.6 s per test so `judging` lasts a few seconds.

**Reaper.** A nullable `submissions.enqueued_at` is set after a successful enqueue. Rows that are `queued`, older than a grace period (2 min) and still `NULL` are re-queued, carrying the test-set version they were accepted against. The sweep is one transaction with `SELECT ... FOR UPDATE SKIP LOCKED`, so two instances never take the same row; the mark is written in the same transaction as the enqueue loop, and a crash before the commit rolls it back (worst case one duplicate job, harmless: a runner skips a job whose verdict exists and the verdict write is idempotent). It sweeps once at startup and then every 15 minutes (`LEETFORCE_REAPER_INTERVAL`, `LEETFORCE_REAPER_GRACE`).

## Alternatives
- **In-process pub/sub hub for SSE:** lower latency, but a hub only reaches clients on the instance that processed the event; with several API instances it needs Redis pub/sub anyway, and the database stays the source of truth. Polling is simple, correct everywhere and bounded by the stream cap. Revisit if the 500 ms poll shows up in database load (Phase 11).
- **Status event over a consumer group** (like results): gives retries, but a status that arrives late is worthless, and pending entries need a janitor. Plain `XREAD` is cheaper and has no failure state.
- **The runner POSTs the status to the API:** needs credentials and an up API on every runner; breaks "runners talk only to Redis".
- **Reaper rule "queued and older than N" without `enqueued_at`:** would re-queue every job that is waiting while runners are down, piling up duplicates; `enqueued_at` identifies exactly the crash window.
- **Reaper polling every 30 s:** reacts faster, but a Neon query every 30 s keeps the database from suspending when idle (cost). The failure it covers is an API crash, which is noticed when the API restarts, hence the startup sweep.

## Consequences
- Verified: `make test-live-e2e` (44 s): queued, judging and AC 5/5 in order over a real `curl -N` stream; an orphaned row is re-queued after an API restart and judged AC; mutation checks: removing the `status = 'queued'` guard fails `TestMarkJudging`, removing `enqueued_at IS NULL` fails the reaper tests, exposing the test-set version in the JSON fails the end-to-end test.
- Redaction for Submit: the stream and all responses carry only state and the verdict view. The end-to-end test submits programs that echo the hidden input and a marker to stdout, stderr and the compiler output and finds none of it, nor the source or version, in any response; its detector has a self-test. Run (custom or sample input) is Phase 8; it will be the only path that shows failing-case details.
- Upstash: the status loop adds one command per 5 s while idle (about 17,000 a day) on top of the Phase 4 loops; not measured against the bill.
- Neon: the reaper wakes the database about every 15 minutes; the owner's Neon plan and limits are still unchecked.
- Per-user stream limits come with authentication (Phase 9); today the cap is per instance.
- A `judged` submission with no verdict row cannot occur (one statement writes both); if it did, the stream would keep polling until its time limit.

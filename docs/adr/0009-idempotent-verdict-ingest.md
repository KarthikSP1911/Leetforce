# 0009. Verdicts reach Postgres through a results-stream consumer, written idempotently in one SQL statement

**Status:** accepted (Phase 4). The owner delegated the choices ("remaining all okay" to the session plan); they are Claude's recommendations and can be changed.

## Context
A runner reports a verdict to the Redis `results` stream (ADR 0008) and never touches the database. The API must turn that into a durable row, and the exit criterion is that **duplicate verdict posts do not change state**. Duplicates are expected, not exotic: the queue is at-least-once (a job can be judged twice after a crash), the API can crash after storing and before acknowledging, and two API instances may read the same entry after a reclaim. A job that kills every runner is dead-lettered with no verdict at all (Phase 3 known gap).

## Decision
- **Transport:** the API reads `<prefix>:results` and `<prefix>:jobs:dead` with its own consumer group `api` (`queue/ingest.go`, separate from the runners' group on the jobs stream). Entries are acknowledged but **not deleted**, so `lfq results` and the crash test still see every verdict. An entry left unacknowledged for `MinIdle` is reclaimed with `XAUTOCLAIM`, so a crashed API instance loses nothing.
- **Idempotency lives in the database, as one statement** (`store.RecordVerdict`): `WITH ins AS (INSERT INTO verdicts ... SELECT ... FROM submissions WHERE id = $1::uuid ON CONFLICT (submission_id) DO NOTHING RETURNING submission_id) UPDATE submissions SET status = 'judged' WHERE id IN (SELECT submission_id FROM ins)`. The primary key on `verdicts.submission_id` makes the first verdict win; the status flips only for the call that really inserted; both happen atomically in one statement, so there is no window where a verdict exists with the submission still `queued`. First write wins even if a later one disagrees (for example WA after AC); a rejudge (Phase 10) will be a deliberate, separate operation, not a duplicate post.
- **Failure policy in the loop** (`api/internal/ingest`): a duplicate or unknown submission is acknowledged and ignored; a transient database error is not acknowledged (redelivered after `MinIdle`); a permanent error (Postgres class 22 or 23: a value the schema rejects), an undecodable entry or a non-UUID submission id is logged and acknowledged, so one bad entry cannot block the stream.
- **Dead letters become `IE`:** every 30 s the API drains the dead-letter stream and writes an `IE` verdict (runner `dead-letter`) for each job, so a poison job still ends with a verdict. An empty `test_set_version` in a verdict (internal errors) falls back to the submission's own.
- **What is stored:** verdict, runtime, memory, passed and total counts, the test-set version it was judged against, the runner id. Not the compiler output (Phase 2 decision B kept at its default: Submit shows only the `CE` label), not test data, not stderr. The submission row separately records the test-set version at accept time.

## Alternatives
- **Runner POSTs the verdict to the API over HTTP:** the runner would need API credentials and the API would have to be up; the stream decouples them and the runner stays Redis-only.
- **Check-then-insert in application code** (`SELECT` then `INSERT`): racy between two instances; the constraint is the only reliable guard.
- **`ON CONFLICT DO UPDATE` (last write wins):** lets a stale or hostile duplicate overwrite a verdict. Shown to fail both tests (below).
- **Delete the results entries after storing:** would break `lfq results` and the Phase 3 crash test, and gives no replay for debugging. The cost is an unbounded stream (see Consequences).
- **A separate service for ingest:** one more thing to deploy; the loop is small and lives in the API process. It can be split out if ingest load ever competes with request handling.

## Consequences
- Verified: `make test-api-e2e` (API, Upstash, runner with nsjail, Neon; about 51 s) shows AC stored with 5 of 5 tests, a conflicting WA injected into the stream leaving the stored submission byte-identical, and a dead-lettered job becoming `IE`. Mutation check: changing `DO NOTHING` to `DO UPDATE` made `TestRecordVerdictIsIdempotent` fail ("duplicate AC record = true, want false") and the end-to-end test fail (verdict flipped AC to WA).
- Polling cost on Upstash: the results poll is `XAUTOCLAIM` + `XREADGROUP BLOCK 5s` per 5 s (roughly 35,000 commands a day while idle) plus about 2 commands per 30 s for dead letters. Unmeasured against the bill; `Config.Block` and `Config.DeadEvery` are the knobs.
- The `results` stream grows without bound because entries are kept; a trim policy (for example `XTRIM MINID` after a retention period) is deferred, to be decided with observability (Phase 11).
- A crash between inserting the submission and enqueueing the job leaves a `queued` row with no job; a reaper for stale queued rows is deferred to Phase 5 with live status.
- Without authentication (Phase 9) anyone who can reach the API can read any submission by id; ids are random UUIDs and responses carry no source, test data or stderr.

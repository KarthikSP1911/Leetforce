-- +goose Up
-- +goose StatementBegin

-- enqueued_at is set once the submission's job is on the queue. A queued
-- submission that still has it NULL after a grace period was stored but never
-- queued (the API died between the two steps), and the reaper re-queues it.
ALTER TABLE submissions ADD COLUMN enqueued_at timestamptz;

-- Rows from before this migration were queued by the old code path.
UPDATE submissions SET enqueued_at = created_at;

-- The reaper's lookup touches only the rare unqueued rows.
CREATE INDEX submissions_unqueued_idx ON submissions (created_at)
    WHERE enqueued_at IS NULL AND status = 'queued';

-- +goose StatementEnd

-- +goose Down
DROP INDEX submissions_unqueued_idx;
ALTER TABLE submissions DROP COLUMN enqueued_at;

-- +goose Up
-- +goose StatementBegin

-- A submission is 'judging' from the moment a runner takes the job until its
-- verdict is stored. The runner reports it over Redis and the API writes it
-- here, so the status stream (SSE) can read one source of truth.
ALTER TABLE submissions DROP CONSTRAINT submissions_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_status_check
    CHECK (status IN ('queued', 'judging', 'judged'));

-- +goose StatementEnd

-- +goose Down
UPDATE submissions SET status = 'queued' WHERE status = 'judging';
ALTER TABLE submissions DROP CONSTRAINT submissions_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_status_check
    CHECK (status IN ('queued', 'judged'));

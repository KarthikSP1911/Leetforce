-- +goose Up
-- +goose StatementBegin

-- client_id is an anonymous browser id sent by the web app, used only to list
-- "my submissions" before accounts exist (Phase 9 replaces it with a user id).
-- It is not a credential: anyone who knows an id can list that browser's
-- submissions, which carry only verdict, runtime, memory and language.
ALTER TABLE submissions ADD COLUMN client_id text;

CREATE INDEX submissions_client_idx ON submissions (client_id, problem_slug, created_at DESC)
    WHERE client_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
DROP INDEX submissions_client_idx;
ALTER TABLE submissions DROP COLUMN client_id;

-- +goose Up
-- +goose StatementBegin

-- The global ranking scans accepted verdicts only; a partial index keeps that
-- scan small as rejected verdicts pile up. Rankings themselves are not stored:
-- they are recomputed from submissions and verdicts (ADR 0023).
CREATE INDEX verdicts_accepted_idx ON verdicts (submission_id) WHERE verdict = 'AC';

-- +goose StatementEnd

-- +goose Down
DROP INDEX verdicts_accepted_idx;

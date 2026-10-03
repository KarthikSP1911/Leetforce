-- +goose Up
-- +goose StatementBegin

-- A contest is a timed window over a fixed set of problems. Its status
-- (upcoming, running, ended) is never stored: it is derived from starts_at and
-- ends_at, so it cannot drift from the clock.
CREATE TABLE contests (
    id         uuid PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    title      text NOT NULL,
    starts_at  timestamptz NOT NULL,
    ends_at    timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);

CREATE TABLE contest_problems (
    contest_id   uuid NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    problem_slug text NOT NULL REFERENCES problems (slug),
    label        text NOT NULL,
    position     int  NOT NULL,
    points       int  NOT NULL DEFAULT 100,
    PRIMARY KEY (contest_id, problem_slug),
    UNIQUE (contest_id, label)
);
CREATE INDEX contest_problems_problem_idx ON contest_problems (problem_slug);

CREATE TABLE contest_participants (
    contest_id    uuid NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    registered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (contest_id, user_id)
);

-- Set only for submissions made inside a contest; only those count for scoring.
ALTER TABLE submissions ADD COLUMN contest_id uuid REFERENCES contests (id);
CREATE INDEX submissions_contest_idx ON submissions (contest_id, created_at)
    WHERE contest_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
DROP INDEX submissions_contest_idx;
ALTER TABLE submissions DROP COLUMN contest_id;
DROP TABLE contest_participants;
DROP TABLE contest_problems;
DROP TABLE contests;

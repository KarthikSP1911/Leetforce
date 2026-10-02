-- +goose Up
-- +goose StatementBegin

-- Problems are loaded from problems/ by the API; test_set_version is the
-- version of the hidden tests currently in force for the problem.
CREATE TABLE problems (
    slug             text PRIMARY KEY,
    title            text NOT NULL,
    difficulty       text NOT NULL CHECK (difficulty IN ('easy', 'medium', 'hard')),
    tags             text[] NOT NULL DEFAULT '{}',
    test_set_version text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- A submission records the test-set version it was accepted against so the
-- problem can be rejudged after a test fix. Its verdict lives in verdicts.
CREATE TABLE submissions (
    id               uuid PRIMARY KEY,
    problem_slug     text NOT NULL REFERENCES problems (slug),
    language         text NOT NULL CHECK (language IN ('python', 'cpp', 'java', 'go')),
    source           text NOT NULL,
    status           text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'judged')),
    test_set_version text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submissions_problem_created_idx ON submissions (problem_slug, created_at DESC);

-- One verdict per submission: the primary key makes a duplicate write a
-- no-op (INSERT ... ON CONFLICT DO NOTHING). No test input, expected output
-- or stderr is stored; Submit never returns them.
CREATE TABLE verdicts (
    submission_id    uuid PRIMARY KEY REFERENCES submissions (id) ON DELETE CASCADE,
    verdict          text NOT NULL CHECK (verdict IN ('AC', 'WA', 'TLE', 'MLE', 'RE', 'CE', 'OLE', 'IE')),
    runtime_ms       bigint NOT NULL DEFAULT 0 CHECK (runtime_ms >= 0),
    memory_kb        bigint NOT NULL DEFAULT 0 CHECK (memory_kb >= 0),
    test_set_version text NOT NULL,
    passed           integer NOT NULL DEFAULT 0 CHECK (passed >= 0),
    total            integer NOT NULL DEFAULT 0 CHECK (total >= 0),
    runner_id        text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
DROP TABLE verdicts;
DROP TABLE submissions;
DROP TABLE problems;

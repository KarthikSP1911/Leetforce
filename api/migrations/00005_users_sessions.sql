-- +goose Up
-- +goose StatementBegin

-- Accounts. Email and username are unique case-insensitively. The password is
-- stored only as a bcrypt hash.
CREATE TABLE users (
    id            uuid PRIMARY KEY,
    email         text NOT NULL,
    username      text NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
CREATE UNIQUE INDEX users_username_key ON users (lower(username));

-- Sessions are opaque random tokens. Only the SHA-256 of the token is stored,
-- so a database leak does not hand out working sessions.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- user_id replaces the anonymous client_id of migration 00004. Older rows keep
-- their client_id and have no user.
ALTER TABLE submissions ADD COLUMN user_id uuid REFERENCES users (id);
CREATE INDEX submissions_user_idx ON submissions (user_id, problem_slug, created_at DESC)
    WHERE user_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
DROP INDEX submissions_user_idx;
ALTER TABLE submissions DROP COLUMN user_id;
DROP TABLE sessions;
DROP TABLE users;

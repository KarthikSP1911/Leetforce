# 0010. Neon access: goose migrations, a pooler-safe pgx pool, and tests in throwaway schemas

**Status:** accepted (Phase 4). Decisions on the owner's delegation ("remaining all okay"); can be changed.

## Context
The API stores problems, submissions and verdicts in Neon Postgres. Neon gives two endpoints: a direct one and a pooled one (`-pooler`, pgbouncer in transaction mode). Neon suspends an idle compute. The owner supplied one connection string (the pooled endpoint, database `neondb`) and no separate test database or branch.

## Decision
- **Migrations:** [goose](https://github.com/pressly/goose), plain SQL files in `api/migrations/`, run by `make migrate-up | migrate-down | migrate-status`. The URL is `LEETFORCE_MIGRATE_DATABASE_URL` if set (intended for Neon's direct endpoint), else `DATABASE_URL`, read from the git-ignored `.env`. The first migration, applied from an empty database and rolled back and re-applied successfully, creates `problems`, `submissions` and `verdicts` (CHECK constraints for difficulty, language, status and the eight verdicts).
- **Pool** (`api/internal/store`): `MaxConnIdleTime` 30 s (idle connections are closed long before Neon suspends the compute), `MaxConnLifetime` 30 min, `MaxConns` 10, and `QueryExecModeCacheDescribe`, because named prepared statements are unreliable behind pgbouncer in transaction mode.
- **Tests never touch real tables:** each database test creates a schema `t_<random>`, applies the real migration files into it, and drops it afterwards (`store/testdb_test.go`). It connects to the direct endpoint (the pooled host with `-pooler` removed, or `LEETFORCE_MIGRATE_DATABASE_URL`), sets `search_path` after connecting, and **aborts if `current_schema()` is not the throwaway schema**. The tests skip without a database.
- **Secrets:** `DATABASE_URL` lives only in `.env` (git-ignored, mode 600 on the host), listed blank in `.env.example`, and in SSM in the cloud (Phase 13). The `&` in the URL means it must be quoted in `.env`.

## Alternatives
- **golang-migrate:** equivalent for our needs; goose won on simpler SQL-file annotations and a status command. Switching costs little (the files are plain SQL).
- **A separate Neon branch (or project) for tests:** cleaner isolation, but needs the owner to create it. The throwaway schema gives most of the isolation now; a test branch can replace it later without changing the tests (only the URL).
- **Pooled endpoint everywhere:** worked for goose and for the API in practice (migrations and queries ran through `-pooler`), so no direct host was needed. The test helper still uses the direct endpoint, because a per-connection `search_path` is not reliable through pgbouncer.
- **An ORM or sqlc:** more machinery than six queries need; hand-written SQL with pgx keeps the one subtle statement (the verdict CTE) visible.

## Consequences
- Mistake found and fixed during the phase: the first test helper set `search_path` as a startup parameter, which the server ignored, so the migration ran against the real `public` schema and failed with "relation already exists" before changing anything. It now uses `AfterConnect` plus the `current_schema()` guard, so this class of mistake aborts instead of writing.
- Latency measured from the EC2 dev host to Neon: `/readyz` (database ping plus Redis ping) answered in about 330 ms; a migration step takes 0.3 to 0.5 s. Neon is in the Singapore region; the EC2 host's region was not compared.
- Test runs cost Neon compute time and a few round trips each (a schema create, the migration, a drop: about 2 s per test).
- Cost: the owner's Neon plan and its compute-hour and storage limits were not checked in this phase, so no cost-table row was added. Please confirm the plan; tests and the API wake the compute and use a little storage.
- The owner pasted the connection string in chat. Rotating the `neondb_owner` password is advisable, and the new value goes only into the `.env` files.

# Phase 3 working log: Queue and runner

**Branch:** `phase/3-queue-runner`
**Range:** `phase-3-start..phase-3-done`
**Status:** in progress

## Units of work
- [x] `feat/3-compose`: Compose file for Redis and MinIO, `.env.example`, Redis on the dev host for tests
- [ ] `feat/3-queue-package`: job and result types, stream, consumer group, ack, `XAUTOCLAIM`
- [ ] `feat/3-runner-module`: `runner/` module and agent loop calling `engine.Judge`
- [ ] `feat/3-runner-reporting`: verdicts to a `results` stream, idempotent by submission ID
- [ ] `test/3-crash-reclaim`: kill a runner mid-job, job reclaimed and judged once; no DB in `runner/go.mod`

## Decisions (2026-10-02)
- Owner asked for the recap question and session plan; the recap question was not answered. The owner replied "upstash url i will provide you later, remaining you decide which is good", so Claude chose the defaults below. Who decided: Claude, on the owner's delegation.
- Upstash URL: owner supplies it later. Until then queue code is developed and tested against a local Redis (dev host); `LEETFORCE_REDIS_URL` stays out of git.
- Reporting before the API exists: runner `XADD`s the verdict to a `results` stream, idempotent by submission ID; the API consumes it in Phase 4.
- Runner privileges: root under a systemd unit on the dev host for now; real privilege model deferred to Phase 6 (ADR in this phase).
- Decision C (Go/Java compile speed) deferred; Phase 2 decisions A and B keep their defaults.

## Session log

### Start of session (2026-10-02)
1. Claude read `CLAUDE.md`, `docs/PROGRESS.md`, `phase-2-summary.md`. Repo matched: branch `main` at `7579541`, tags `phase-0/1/2-start|done`, only `web/AGENTS.md` and `web/CLAUDE.md` untracked.
2. Claude (repo): `git checkout -b phase/3-queue-runner`, `git tag phase-3-start`. Set `PROGRESS.md` to in progress.
3. Claude (read-only checks): Docker Desktop is not running on the Windows machine; the dev host (`ssh leetforce-dev`) has Go 1.27.1 but no Docker and no Redis. The host checkout is on `phase/2-judge-engine`.

### Unit 1: `feat/3-compose` (2026-10-02)
1. Claude (host): `sudo -n apt-get install -y redis-server`. Result: Redis 7.0.15 running, listening on 127.0.0.1:6379 and ::1 only (`redis-cli ping` returns PONG). It supports `XAUTOCLAIM` (Redis 6.2+). Used for queue tests until the Upstash URL arrives. Not yet in `scripts/setup-dev-host.sh` (added in unit 2).
2. Claude (repo): `docker-compose.yml` (redis:7-alpine with AOF, minio; both bound to 127.0.0.1; MinIO password required from `.env`), `.env.example` (`LEETFORCE_TEST_REDIS_URL`, MinIO vars), `make dev` and `make down`.
3. Mistake and fix: the first commit of these files was rejected by commitlint (a body line over 72 characters), and my next `git add docs` then swept the still-staged compose files into a `docs` commit. Nothing was pushed, so I rebuilt the branch from `b7cf4e8` with the compose files in their own `build(infra)` commit (the wrong commit is dropped; lesson: check that a commit succeeded before staging anything else).
4. Not verified: `docker compose config` was not run because Docker is not installed on the host and Docker Desktop was stopped locally. The file is unvalidated until Docker is available.

## File and path index
- `docker-compose.yml`, `.env.example`, `Makefile` (`dev`, `down`): local Redis and MinIO
- `docs/phases/phase-3-log.md`: this log
- `docs/PROGRESS.md`: phase 3 in progress

# Phase 5 working log: Live status and storage

**Branch:** `phase/5-live-status-storage`
**Range:** `phase-5-start..phase-5-done`
**Status:** in progress

## Units of work
- [ ] `feat/5-dev-host-docker`: Docker on the dev host, MinIO from `docker-compose.yml`, `trivy image`
- [ ] `feat/5-test-storage`: problem tests in MinIO keyed by test-set version; API and runner read them from there
- [ ] `feat/5-judging-state`: runner publishes a Judging event; API ingests it
- [ ] `feat/5-sse-status`: `GET /submissions/:id/events` (SSE)
- [ ] `feat/5-reaper`: re-enqueue rows stored but never queued
- [ ] `test/5-e2e-redaction`: SSE end-to-end and hidden-data leak tests
- [ ] `docs/5-adr-report`: ADR, `docs/FLOW.md`, report, summary

## Decisions (2026-10-02)
- Recap question: not answered; the owner replied "do as u wish", so the defaults of the session plan apply. Who decided: Claude, on the owner's delegation.
- MinIO in Docker on the EC2 dev host (carried from the Phase 4 review). Judging event transport: a separate `status` Redis stream ingested by the API (runners stay off the DB and HTTP). Both on the session-plan defaults.

## Session log

### Start of session (2026-10-02)
1. Claude read `CLAUDE.md`, `docs/PROGRESS.md`, `phase-4-summary.md` and the Phase 5 section of `docs/PLAN.md`. Repo matched: branch `main` at `c3dbf06`, tags `phase-0..4-start|done`, only `web/AGENTS.md` and `web/CLAUDE.md` untracked.
2. Claude (repo): `git checkout -b phase/5-live-status-storage`, `git tag phase-5-start`.
3. Claude (read-only): `ssh leetforce-dev` works; `docker` is not installed; the host checkout is on `feat/4-migrations`.

### Unit 1: `feat/5-dev-host-docker` (2026-10-02)
1. Claude (host): installed Docker from the official Docker apt repository (key in `/etc/apt/keyrings/docker.asc`, source `/etc/apt/sources.list.d/docker.list`, packages `docker-ce docker-ce-cli containerd.io docker-compose-plugin`); `docker 29.8.2`, `compose v5.5.1`, service `docker` active; user `ubuntu` added to group `docker` (takes effect at next login). Same steps are in `scripts/setup-dev-host.sh` (new `docker` block; the block alone was run on the host). Disk after: 2.5 GB free of 14 GB.
2. Finding (changes an earlier decision): `docker compose pull minio` failed with "pull access denied". MinIO no longer publishes public container images (Docker Hub `minio/minio` and quay.io are gone; checked with `docker manifest inspect`, including the old `RELEASE.2025-04-22T22-12-26Z` tag). So the Phase 4 decision "MinIO in Docker" cannot be carried out as written. Who decided: Claude, on the owner's delegation ("do as u wish"); the code only uses the S3 API, so the local server is replaceable and production is real S3.
3. Claude (host): evaluated `rustfs/rustfs` (Apache-2.0, S3 compatible, accepts MINIO_ROOT_* and RUSTFS_* credentials). Available: `dxflrs/garage:v1.0.1` and `rustfs/rustfs`. Chose RustFS pinned to `1.0.0` (GA, 2026-09-16): one container, no layout bootstrap, about 150 MiB RAM on the 1 GB host. Garage was the alternative (needs a config file and layout commands). `trivy image rustfs/rustfs:latest` (same digest `8cc98017...` as `1.0.0`): 0 HIGH/CRITICAL (Trivy 0.75.0, alpine 3.24.1 base; Trivy does not scan the Rust binary's crates). `trivy config docker-compose.yml`: reports "not scanned" (Trivy has no Compose checker), recorded as such.
4. Claude (repo): `docker-compose.yml` service `minio` replaced by `s3` (`rustfs/rustfs:1.0.0`, ports bound to 127.0.0.1, volume `s3-data`); `.env.example` now documents `LEETFORCE_S3_ENDPOINT|ACCESS_KEY|SECRET_KEY|BUCKET|USE_TLS` instead of `LEETFORCE_MINIO_*`; `Makefile` `dev` comment updated.
5. Claude (host): copied `docker-compose.yml` with scp (the host checkout is on an old branch); replaced the `LEETFORCE_MINIO_*` lines of `~/Leetforce/.env` with `LEETFORCE_S3_*` (secret generated on the host with `openssl rand -hex 16`, mode 600, never shown); `docker compose --env-file .env up -d s3`; `curl 127.0.0.1:9000/health` returned 200.
6. Mistake and fix: the first attempt wrote `LEETFORCE_MINIO_*` to the host `.env` before the image failed to pull; those lines were removed in step 5 and the failed containers removed.

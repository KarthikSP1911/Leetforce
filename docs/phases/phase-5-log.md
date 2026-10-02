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

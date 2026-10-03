#!/usr/bin/env bash
# Phase 15 exit test: contest standings and the global ranking stay correct
# while many verdicts are ingested concurrently (same and different users,
# every verdict delivered twice) with readers going through the cache.
# Runs one Go test with the race detector on a throwaway Postgres schema built
# from the real migrations (needs the Phase 14 migrations, so run it after the
# merge). Needs DATABASE_URL (environment or ./.env) and Go; no Redis, no sandbox.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"

cd api
go test -race -count="${COUNT:-3}" -tags leaderboard_concurrent -run TestLeaderboardConcurrentIngest -v ./internal/store/

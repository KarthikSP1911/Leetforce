#!/usr/bin/env bash
# Phase 15 exit test: contest standings and the global ranking stay correct
# while many verdicts are ingested concurrently (same and different users,
# every verdict delivered twice) with readers going through the cache.
# Runs one Go test with the race detector on a throwaway Postgres schema built
# from the real migrations (needs the Phase 14 migrations, so run it after the
# merge). Needs DATABASE_URL (environment or ./.env) and Go; no Redis, no sandbox.
# While Phase 14 is not merged, the leaderboard_stub tag supplies its contract.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"

TAGS="leaderboard_concurrent"
# After Phase 16 removes the stub, set LEADERBOARD_STUB=0 (or delete this line).
if [ "${LEADERBOARD_STUB:-1}" = "1" ]; then TAGS="$TAGS,leaderboard_stub"; fi

cd api
go test -race -count="${COUNT:-3}" -tags "$TAGS" -run TestLeaderboardConcurrentIngest -v ./internal/store/

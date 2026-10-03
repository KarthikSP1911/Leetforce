#!/usr/bin/env bash
# Phase 14 exit test: a full mock contest end to end. Starts the API and a
# runner on a throwaway Redis key prefix, seeds the contest
# (scripts/seed-mock-contest.sh), drives it through the API
# (scripts/mock-contest-e2e.py: register, submit AC/WA/CE, visibility, 409s,
# scores from contest.Score), then removes the mock contest and users.
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, psql, curl),
# DATABASE_URL (with migration 00006 applied: make migrate-up) and
# LEETFORCE_REDIS_URL (environment or ./.env). Writes to the real database.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"

export LEETFORCE_QUEUE_PREFIX="lfcontest-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
export LEETFORCE_API_ADDR="127.0.0.1:${E2E_PORT:-18084}"
export LEETFORCE_API="http://$LEETFORCE_API_ADDR"
unset LEETFORCE_S3_ENDPOINT # tests come from the problems directory
WORK="$(mktemp -d)"
API_PID="" RUNNER_PID=""

fail() {
  echo "FAIL: $*" >&2
  echo "--- api log" >&2; tail -n 30 "$WORK/api.log" >&2 || true
  echo "--- runner log" >&2; tail -n 15 "$WORK/runner.log" >&2 || true
  exit 1
}

cleanup() {
  [ -n "$API_PID" ] && kill -TERM "$API_PID" 2>/dev/null || true
  [ -n "$RUNNER_PID" ] && sudo -n kill -KILL "$RUNNER_PID" 2>/dev/null || true
  bin/lfq destroy >/dev/null 2>&1 || true
  scripts/seed-mock-contest.sh --clean >/dev/null 2>&1 || echo "warning: could not remove the mock contest" >&2
  sudo -n rm -rf "$WORK"
}
trap cleanup EXIT

make build-api build-runner >/dev/null
(cd api && go build -o ../bin/contestscore ./cmd/contestscore)

bin/api >"$WORK/api.log" 2>&1 &
API_PID=$!
for _ in $(seq 120); do curl -fsS "$LEETFORCE_API/readyz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "$LEETFORCE_API/readyz" >/dev/null 2>&1 || fail "the API did not become ready"

sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
  bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
[ -s "$WORK/runner.pid" ] || fail "the runner did not start"
RUNNER_PID="$(cat "$WORK/runner.pid")"

scripts/seed-mock-contest.sh || fail "seeding the mock contest failed"
python3 scripts/mock-contest-e2e.py || fail "mock-contest-e2e.py reported failures"
echo "PASS: a full mock contest ran end to end (register, AC/WA/CE, visibility, scoring)"

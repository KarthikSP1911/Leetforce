#!/usr/bin/env bash
# Phase 9 exit test: accounts, sessions, login required for Run and Submit,
# per-user and per-IP limits on the real Redis counters, solved status, plus the
# Run/Submit/SSE loop as a signed-in user (scripts/phase9-e2e.py does the checks).
#
# Starts the API (default limits) and a runner on a throwaway Redis key prefix,
# runs the checks, then deletes the throwaway account and its submissions.
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, psql),
# DATABASE_URL and LEETFORCE_REDIS_URL (environment or ./.env).
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"

export LEETFORCE_QUEUE_PREFIX="lfauth-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
export LEETFORCE_API_ADDR="127.0.0.1:${E2E_PORT:-18083}"
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
  # phase9-e2e.py prints "account used: <name>"
  local name; name="$(sed -n 's/^account used: //p' "$WORK/e2e.out" 2>/dev/null | head -1)"
  if [[ "$name" =~ ^e2e[0-9a-f]+$ ]]; then
    psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE username = '$name'); DELETE FROM users WHERE username = '$name'" >/dev/null 2>&1 || echo "warning: could not delete test account $name" >&2
  fi
  sudo -n rm -rf "$WORK"
}
trap cleanup EXIT

make build-api build-runner >/dev/null

bin/api >"$WORK/api.log" 2>&1 &
API_PID=$!
for _ in $(seq 120); do curl -fsS "$LEETFORCE_API/readyz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "$LEETFORCE_API/readyz" >/dev/null 2>&1 || fail "the API did not become ready"

sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
  bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
[ -s "$WORK/runner.pid" ] || fail "the runner did not start"
RUNNER_PID="$(cat "$WORK/runner.pid")"

python3 scripts/phase9-e2e.py 2>&1 | tee "$WORK/e2e.out" || fail "phase9-e2e.py reported failures"
echo "PASS: accounts, login required, limits, solved status and the Run/Submit loop as a signed-in user"

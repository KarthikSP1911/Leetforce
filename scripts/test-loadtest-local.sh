#!/usr/bin/env bash
# Phase 16 exit test: a modest load test against the local stack (dev host).
# Starts the API and a runner on a throwaway Redis key prefix with the rate
# limits raised (one IP, many users), runs tools/loadtest in mixed mode and then
# in contest mode against the mock contest, and deletes every account it created
# (email @loadtest.invalid) with their submissions, plus the mock contest.
#   LOAD_USERS=6 LOAD_ITER=2 scripts/test-loadtest-local.sh
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, psql, curl),
# DATABASE_URL with migrations applied, LEETFORCE_REDIS_URL. Writes to the real database.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"
USERS="${LOAD_USERS:-6}"
ITER="${LOAD_ITER:-2}"

export LEETFORCE_QUEUE_PREFIX="lfload-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
export LEETFORCE_API_ADDR="127.0.0.1:${E2E_PORT:-18085}"
export LEETFORCE_API="http://$LEETFORCE_API_ADDR"
export LEETFORCE_LIMIT_AUTH_IP=1000 LEETFORCE_LIMIT_SUBMIT_USER=1000 LEETFORCE_LIMIT_SUBMIT_IP=10000 \
  LEETFORCE_LIMIT_RUN_USER=1000 LEETFORCE_LIMIT_RUN_IP=10000 LEETFORCE_LIMIT_LOGIN_ACCOUNT=1000
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
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -qAt -c \
    "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@loadtest.invalid'); DELETE FROM users WHERE email LIKE '%@loadtest.invalid'" \
    >/dev/null 2>&1 || echo "warning: could not remove the load test users" >&2
  sudo -n rm -rf "$WORK"
}
trap cleanup EXIT

make build-api build-runner build-judge >/dev/null
bin/api >"$WORK/api.log" 2>&1 &
API_PID=$!
for _ in $(seq 120); do curl -fsS "$LEETFORCE_API/readyz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "$LEETFORCE_API/readyz" >/dev/null 2>&1 || fail "the API did not become ready"

sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
  bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
[ -s "$WORK/runner.pid" ] || fail "the runner did not start"
RUNNER_PID="$(cat "$WORK/runner.pid")"

export LEETFORCE_LOADTEST_BASE_URL="$LEETFORCE_API"
echo "== mixed mode: $USERS users x $ITER iterations"
(cd tools/loadtest && go run . -mode mixed -users "$USERS" -iterations "$ITER" -ramp 3s -json "$WORK/mixed.json") \
  || fail "mixed load test failed"

scripts/seed-mock-contest.sh || fail "seeding the mock contest failed"
echo "== contest mode: $USERS users x $ITER iterations"
(cd tools/loadtest && go run . -mode contest -contest mock-contest -users "$USERS" -iterations "$ITER" -ramp 3s -json "$WORK/contest.json") \
  || fail "contest load test failed"

python3 - "$WORK/mixed.json" "$WORK/contest.json" <<'PY' || fail "verdicts were missing: a submission did not reach a verdict"
import json, sys
for p in sys.argv[1:]:
    r = json.load(open(p))
    acc, got = r.get("submissions", 0), r.get("verdicts", 0)
    print(p.rsplit("/", 1)[-1], "accepted", acc, "with verdict", got)
    if acc == 0 or got != acc:
        sys.exit(1)
PY
echo "PASS: load test finished and every accepted submission reached a verdict"

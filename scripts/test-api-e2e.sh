#!/usr/bin/env bash
# Phase 4 end-to-end test: API -> Redis -> runner -> Redis -> API -> Postgres.
#
# 1. Submit a correct Python solution through the API; wait for the verdict (AC).
# 2. Inject a conflicting verdict (WA, another runner) for the same submission
#    straight into the results stream; once the API has consumed it, the stored
#    verdict must be unchanged (idempotent verdict write).
# 3. With the runner stopped, submit again and inject a dead-lettered job for
#    it; the API must record an IE verdict (a poison job still ends with a verdict).
#
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, redis-cli),
# DATABASE_URL and LEETFORCE_REDIS_URL (environment or ./.env). It uses a
# throwaway Redis key prefix. It does write two rows to the real submissions
# table in Postgres (there is no cleanup yet). Takes about two minutes.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"

export LEETFORCE_QUEUE_PREFIX="lfe2e-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
export LEETFORCE_API_ADDR="127.0.0.1:${E2E_PORT:-18081}"
BASE="http://$LEETFORCE_API_ADDR"
P="$LEETFORCE_QUEUE_PREFIX"
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
  rm -rf "$WORK"
}
trap cleanup EXIT

wait_for() { # wait_for <seconds> <description> <command...>
  local secs="$1" what="$2"; shift 2
  for _ in $(seq $((secs * 2))); do "$@" && return 0; sleep 0.5; done
  fail "timed out waiting for $what"
}

# field <json> <dotted.path>: prints the value, or nothing if absent.
field() {
  python3 -c '
import json, sys
v = json.loads(sys.argv[1])
for k in sys.argv[2].split("."):
    v = v.get(k) if isinstance(v, dict) else None
print("" if v is None else v)' "$1" "$2"
}
get_sub() { curl -fsS "$BASE/submissions/$1"; }
submit() { # submit <language> <source-file>: prints the new submission id
  local body
  body="$(python3 -c '
import json, sys
print(json.dumps({"problem": "sample-sum", "language": sys.argv[1], "source": open(sys.argv[2]).read()}))' "$1" "$2")"
  field "$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$body" "$BASE/submissions")" id
}
is_judged() { [ "$(field "$(get_sub "$1")" status)" = "judged" ]; }
group_field() { redis-cli -u "$LEETFORCE_REDIS_URL" --raw XINFO GROUPS "$1" | awk -v k="$2" '$0==k{getline; print; exit}'; }
api_caught_up() { # the API group has read $1 entries and has none pending
  [ "$(group_field "$P:results" entries-read)" = "$1" ] && [ "$(group_field "$P:results" pending)" = "0" ]
}

make build-api build-runner >/dev/null

echo "1/3 start the API and a runner"
bin/api >"$WORK/api.log" 2>&1 &
API_PID=$!
ready() { curl -fsS "$BASE/readyz" >/dev/null 2>&1; }
wait_for 60 "the API to be ready" ready
sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
  bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
[ -s "$WORK/runner.pid" ] || fail "the runner did not start"
RUNNER_PID="$(cat "$WORK/runner.pid")"

echo "2/3 submit a correct solution; expect AC through the whole chain"
ID="$(submit python problems/sample-sum/solutions/python/ac.py)"
[ -n "$ID" ] || fail "POST /submissions returned no id"
echo "    submission $ID"
[ "$(field "$(get_sub "$ID")" test_set_version)" = "" ] || fail "the API leaked the test-set version"
judged() { is_judged "$ID"; }
wait_for 120 "the verdict" judged
FIRST="$(get_sub "$ID")"
[ "$(field "$FIRST" verdict.verdict)" = "AC" ] || fail "expected AC, got: $FIRST"
[ "$(field "$FIRST" verdict.passed)" = "$(field "$FIRST" verdict.total)" ] || fail "not all tests passed: $FIRST"
echo "    stored: $FIRST"

echo "    inject a conflicting WA verdict for the same submission"
redis-cli -u "$LEETFORCE_REDIS_URL" XADD "$P:results" '*' result \
  "{\"submission_id\":\"$ID\",\"verdict\":\"WA\",\"runtime_ms\":1,\"memory_kb\":1,\"test_set_version\":\"bogus\",\"passed\":0,\"total\":5,\"runner_id\":\"impostor\"}" >/dev/null
caught_up() { api_caught_up 2; }
wait_for 60 "the API to consume the duplicate" caught_up
SECOND="$(get_sub "$ID")"
[ "$SECOND" = "$FIRST" ] || fail "a duplicate verdict changed state:
before: $FIRST
after:  $SECOND"
echo "    unchanged after the duplicate"

echo "3/3 stop the runner, simulate a dead-lettered job; expect IE"
sudo -n kill -KILL "$RUNNER_PID" 2>/dev/null || true
RUNNER_PID=""
ID2="$(submit python problems/sample-sum/solutions/python/ac.py)"
echo "    submission $ID2"
redis-cli -u "$LEETFORCE_REDIS_URL" XADD "$P:jobs:dead" '*' \
  job "{\"submission_id\":\"$ID2\",\"problem\":\"sample-sum\",\"language\":\"python\",\"source\":\"x\"}" \
  entry 0-0 reason "max deliveries exceeded" deliveries 4 >/dev/null
judged2() { is_judged "$ID2"; }
wait_for 90 "the IE verdict" judged2
DEAD="$(get_sub "$ID2")"
[ "$(field "$DEAD" verdict.verdict)" = "IE" ] || fail "expected IE, got: $DEAD"
echo "    stored: $DEAD"

echo "PASS: AC stored through API, runner and ingest; a duplicate verdict changed nothing; a dead-lettered job became IE"

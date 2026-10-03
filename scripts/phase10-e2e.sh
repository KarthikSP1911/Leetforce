#!/usr/bin/env bash
# Phase 10 exit test: fixing a test set triggers a rejudge of affected submissions.
#
# 1. Copy sample-sum to a throwaway problem (rejudge-demo) in a temp directory
#    and start the API and a runner on it. A correct solution is judged AC.
# 2. Stop the API, "fix" the tests in the copy (a wrong expected output, so the
#    test-set version changes and the old solution no longer passes), restart
#    the API. Its startup sync sees the version change and rejudges.
# 3. The same submission must end WA, judged against the new version: one
#    verdict row, test_set_version moved, verdict stamped with the new version.
# 4. Restarting the API once more changes nothing (the rejudge is idempotent),
#    and `bin/rejudge -dry-run` finds nothing stale.
#
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, psql),
# DATABASE_URL and LEETFORCE_REDIS_URL (environment or ./.env). It uses a
# throwaway Redis key prefix and its own problem slug, and deletes the rows it
# creates (submissions, problem, throwaway account). Object storage is not used.
# Takes about two minutes.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"
command -v psql >/dev/null || { echo "psql is required" >&2; exit 2; }
unset LEETFORCE_S3_ENDPOINT # runner and API both read the temp problems directory

SLUG="rejudge-demo"
WORK="$(mktemp -d)"
export LEETFORCE_QUEUE_PREFIX="lfp10-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$WORK/problems"
export LEETFORCE_API_ADDR="127.0.0.1:${E2E_PORT:-18082}"
BASE="http://$LEETFORCE_API_ADDR"
API_PID="" RUNNER_PID=""
JAR="$WORK/cookies"; E2E_USER="p10e$$"

fail() {
  echo "FAIL: $*" >&2
  echo "--- api log" >&2; tail -n 30 "$WORK/api.log" >&2 || true
  echo "--- runner log" >&2; tail -n 15 "$WORK/runner.log" >&2 || true
  exit 1
}

sql() { psql "$DATABASE_URL" -qAt -c "$1"; }

cleanup() {
  [ -n "$API_PID" ] && kill -TERM "$API_PID" 2>/dev/null || true
  [ -n "$RUNNER_PID" ] && sudo -n kill -KILL "$RUNNER_PID" 2>/dev/null || true
  bin/lfq destroy >/dev/null 2>&1 || true
  sql "DELETE FROM submissions WHERE problem_slug = '$SLUG' OR user_id IN (SELECT id FROM users WHERE username = '$E2E_USER'); DELETE FROM problems WHERE slug = '$SLUG'; DELETE FROM users WHERE username = '$E2E_USER'" >/dev/null 2>&1 \
    || echo "warning: could not delete the rows for $SLUG and $E2E_USER" >&2
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
ensure_login() {
  [ -s "$JAR" ] && return 0
  curl -fsS -c "$JAR" -X POST -H 'Content-Type: application/json' \
    -d "{\"email\":\"$E2E_USER@example.com\",\"username\":\"$E2E_USER\",\"password\":\"e2e-password-1\"}" \
    "$BASE/auth/signup" >/dev/null || fail "sign-up failed"
}
get_sub() { curl -fsS "$BASE/submissions/$1"; }
submit() { # submit <source-file>: prints the new submission id
  ensure_login
  local body
  body="$(python3 -c '
import json, sys
print(json.dumps({"problem": sys.argv[1], "language": "python", "source": open(sys.argv[2]).read()}))' "$SLUG" "$1")"
  field "$(curl -fsS -b "$JAR" -X POST -H 'Content-Type: application/json' -d "$body" "$BASE/submissions")" id
}
ready() { curl -fsS "$BASE/readyz" >/dev/null 2>&1; }
start_api() {
  bin/api >>"$WORK/api.log" 2>&1 &
  API_PID=$!
  wait_for 60 "the API to be ready" ready
}
stop_api() {
  kill -TERM "$API_PID" 2>/dev/null || true
  wait "$API_PID" 2>/dev/null || true
  API_PID=""
}
verdict_is() { # verdict_is <id> <verdict>: judged with that verdict
  local s; s="$(get_sub "$1")"
  [ "$(field "$s" status)" = "judged" ] && [ "$(field "$s" verdict.verdict)" = "$2" ]
}
db_version() { sql "SELECT test_set_version FROM submissions WHERE id = '$1'"; }
verdict_version() { sql "SELECT test_set_version FROM verdicts WHERE submission_id = '$1'"; }
verdict_rows() { sql "SELECT count(*) FROM verdicts WHERE submission_id = '$1'"; }

make build-api build-runner >/dev/null
(cd api && go build -o ../bin/rejudge ./cmd/rejudge)

mkdir -p "$LEETFORCE_PROBLEMS_DIR"
cp -r problems/sample-sum "$LEETFORCE_PROBLEMS_DIR/$SLUG"
sed -i "s/^slug: .*/slug: $SLUG/; s/^title: .*/title: Rejudge Demo/" "$LEETFORCE_PROBLEMS_DIR/$SLUG/problem.yaml"
SOLUTION="problems/sample-sum/solutions/python/ac.py"

echo "1/4 start the API and a runner; a correct solution is judged AC"
start_api
sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
  bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
[ -s "$WORK/runner.pid" ] || fail "the runner did not start"
RUNNER_PID="$(cat "$WORK/runner.pid")"

ID="$(submit "$SOLUTION")"
[ -n "$ID" ] || fail "POST /submissions returned no id"
echo "    submission $ID"
ac() { verdict_is "$ID" AC; }
wait_for 120 "the first verdict (AC)" ac
V1="$(db_version "$ID")"
[ "$(verdict_version "$ID")" = "$V1" ] || fail "verdict version differs from the submission version at v1"
echo "    AC at test-set version $V1"

echo "2/4 fix the tests (version changes) and restart the API"
stop_api
printf '999\n' > "$LEETFORCE_PROBLEMS_DIR/$SLUG/tests/01.out"
start_api

echo "3/4 expect the submission to be rejudged against the new version"
wa() { verdict_is "$ID" WA; }
wait_for 120 "the rejudged verdict (WA)" wa
V2="$(db_version "$ID")"
[ "$V2" != "$V1" ] || fail "submission test_set_version did not change ($V1)"
[ "$(verdict_version "$ID")" = "$V2" ] || fail "stored verdict is not stamped with the new version $V2"
[ "$(verdict_rows "$ID")" = "1" ] || fail "expected exactly one verdict row"
[ "$(sql "SELECT test_set_version FROM problems WHERE slug = '$SLUG'")" = "$V2" ] || fail "problem version is not $V2"
echo "    WA at test-set version $V2 (was $V1), one verdict row"

echo "4/4 a second restart and the rejudge tool find nothing to do"
BEFORE="$(sql "SELECT status || ' ' || updated_at FROM submissions WHERE id = '$ID'")"
stop_api
start_api
sleep 3
AFTER="$(sql "SELECT status || ' ' || updated_at FROM submissions WHERE id = '$ID'")"
[ "$BEFORE" = "$AFTER" ] || fail "an unchanged restart touched the submission: '$BEFORE' -> '$AFTER'"
verdict_is "$ID" WA || fail "verdict changed on an unchanged restart"
OUT="$(bin/rejudge -dry-run "$SLUG")"
echo "    $OUT"
case "$OUT" in *": 0 submissions"*) ;; *) fail "rejudge -dry-run reported stale submissions: $OUT" ;; esac

echo "PASS: a fixed test set re-queued the submission, the new verdict replaced the old one once, and repeating changed nothing"

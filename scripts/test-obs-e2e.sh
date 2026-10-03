#!/usr/bin/env bash
# Phase 11 exit test: the leetforce_ metrics follow a live submission flow.
# Starts the API and a runner on a throwaway Redis prefix, submits 12 mixed
# solutions through the API and checks that the counters, histograms and queue
# gauges moved by exactly that much; then kills the runner, submits 2 more and
# checks the queue gauges show them waiting and ageing while the runner's
# /metrics port is gone (what the RunnerDown alert watches).
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, psql),
# DATABASE_URL and LEETFORCE_REDIS_URL (environment or ./.env). Writes 14
# submissions to the real database and deletes them with the account.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"

export LEETFORCE_QUEUE_PREFIX="lfobs-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
API_PORT="${OBS_API_PORT:-18084}" API_METRICS="${OBS_API_METRICS_PORT:-19102}" RUNNER_METRICS="${OBS_RUNNER_METRICS_PORT:-19101}"
export LEETFORCE_API_ADDR="127.0.0.1:$API_PORT"
export LEETFORCE_METRICS_ADDR="127.0.0.1:$API_METRICS"
export LEETFORCE_METRICS_QUEUE_EVERY=1s
export LEETFORCE_LIMIT_SUBMIT_USER=-1 LEETFORCE_LIMIT_SUBMIT_IP=-1
unset LEETFORCE_S3_ENDPOINT # tests come from the problems directory
WORK="$(mktemp -d)"
API_PID="" RUNNER_PID=""

fail() {
  echo "FAIL: $*" >&2
  echo "--- api log" >&2; tail -n 20 "$WORK/api.log" >&2 || true
  echo "--- runner log" >&2; tail -n 10 "$WORK/runner.log" >&2 || true
  exit 1
}
cleanup() {
  [ -n "$API_PID" ] && kill -TERM "$API_PID" 2>/dev/null || true
  [ -n "$RUNNER_PID" ] && sudo -n kill -KILL "$RUNNER_PID" 2>/dev/null || true
  bin/lfq destroy >/dev/null 2>&1 || true
  local name; name="$(sed -n 's/^account used: //p' "$WORK"/load*.out 2>/dev/null | head -1)"
  if [[ "$name" =~ ^obs[0-9a-f]+$ ]]; then
    psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE username = '$name'); DELETE FROM users WHERE username = '$name'" >/dev/null 2>&1 || echo "warning: could not delete test account $name" >&2
  fi
  sudo -n rm -rf "$WORK"
}
trap cleanup EXIT

# metric <port> <series-regex>: sum of the matching samples (0 when none).
metric() {
  curl -fsS "http://127.0.0.1:$1/metrics" | awk -v re="$2" '$0 ~ re && $1 !~ /^#/ { s += $NF } END { printf "%d\n", s }'
}
wait_for() { # wait_for <seconds> <description> <command...>
  local t=$1 what=$2; shift 2
  for _ in $(seq $((t * 2))); do "$@" >/dev/null 2>&1 && return 0; sleep 0.5; done
  fail "timed out waiting for $what"
}
check() { # check <name> <got> <want>
  [ "$2" = "$3" ] || fail "$1: got $2, want $3"
  echo "PASS $1 = $2"
}

make build-api build-runner >/dev/null
bin/api >"$WORK/api.log" 2>&1 &
API_PID=$!
ready() { curl -fsS "http://$LEETFORCE_API_ADDR/readyz"; }
wait_for 60 "the API to be ready" ready

sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
  env LEETFORCE_METRICS_ADDR="127.0.0.1:$RUNNER_METRICS" \
  bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
[ -s "$WORK/runner.pid" ] || fail "the runner did not start"
RUNNER_PID="$(cat "$WORK/runner.pid")"
runner_up() { curl -fsS "http://127.0.0.1:$RUNNER_METRICS/metrics"; }
wait_for 20 "the runner metrics port" runner_up

# 1. a burst of 12 submissions: the queue gauges must show work in flight
python3 scripts/obs-load.py --api "http://$LEETFORCE_API_ADDR" --count 12 --rate 8 --nowait >"$WORK/load1.out" &
LOAD_PID=$!
peak=0
for _ in $(seq 60); do
  depth=$(( $(metric "$API_METRICS" '^leetforce_queue_waiting ') + $(metric "$API_METRICS" '^leetforce_queue_pending ') ))
  [ "$depth" -gt "$peak" ] && peak=$depth
  [ "$peak" -gt 0 ] && break
  sleep 0.5
done
wait "$LOAD_PID" || fail "obs-load.py failed"
[ "$peak" -gt 0 ] || fail "queue depth never rose above 0 during the burst"
echo "PASS queue depth peaked at $peak during the burst"

verdicts() { [ "$(metric "$API_METRICS" '^leetforce_verdicts_total\{')" -ge 12 ]; }
wait_for 180 "12 verdicts" verdicts

check "submissions created" "$(metric "$API_METRICS" '^leetforce_submissions_created_total\{')" 12
check "verdicts stored" "$(metric "$API_METRICS" '^leetforce_verdicts_total\{')" 12
[ "$(metric "$API_METRICS" '^leetforce_verdicts_total\{verdict="AC"\}')" -gt 0 ] || fail "no AC verdict counted"
check "POST /submissions 202" "$(metric "$API_METRICS" '^leetforce_http_requests_total\{method="POST",route="/submissions",status="202"\}')" 12
check "runner jobs" "$(metric "$RUNNER_METRICS" '^leetforce_runner_jobs_total\{kind="submission"')" 12
check "runner judge histogram count" "$(metric "$RUNNER_METRICS" '^leetforce_runner_judge_duration_seconds_count\{')" 12
check "runner in flight" "$(metric "$RUNNER_METRICS" '^leetforce_runner_jobs_in_flight ')" 0
settled() { [ "$(( $(metric "$API_METRICS" '^leetforce_queue_waiting ') + $(metric "$API_METRICS" '^leetforce_queue_pending ') ))" -eq 0 ]; }
wait_for 20 "the queue gauges to drain" settled
echo "PASS queue gauges drained to 0"

# 2. runner lost: its port disappears and new jobs wait and age
sudo -n kill -KILL "$RUNNER_PID"; RUNNER_PID=""
if curl -fsS --max-time 3 "http://127.0.0.1:$RUNNER_METRICS/metrics" >/dev/null 2>&1; then fail "runner metrics port still answers after the kill"; fi
echo "PASS runner metrics port is down (up==0 for the RunnerDown alert)"
python3 scripts/obs-load.py --api "http://$LEETFORCE_API_ADDR" --count 2 --rate 0 --nowait --seed 3 >"$WORK/load2.out"
waiting() { [ "$(metric "$API_METRICS" '^leetforce_queue_waiting ')" -eq 2 ]; }
wait_for 20 "2 waiting jobs" waiting
sleep 4
[ "$(metric "$API_METRICS" '^leetforce_queue_oldest_job_age_seconds ')" -ge 3 ] || fail "oldest job age did not grow while no runner was alive"
echo "PASS queue shows 2 waiting jobs and a growing oldest-job age"
[ "$(metric "$API_METRICS" '^leetforce_queue_sample_success ')" -eq 1 ] || fail "queue sampling reports failure"
echo "PASS: metrics follow submit, judge, verdict and runner loss"

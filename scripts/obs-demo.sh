#!/usr/bin/env bash
# Phase 11 demo: runs the API and a runner on the dev host with the DEFAULT
# metrics ports (9102 API, 9101 runner), logs in ~/obs, and drives a steady
# stream of mixed submissions, so the Grafana dashboards show a live flow.
#   scripts/obs-demo.sh [seconds=300] [rate-per-second=0.4] [runner-kill-at-second=0]
# A third argument stops the runner that many seconds in (and restarts it 60
# seconds later) so you can watch the queue grow and the alerts fire and clear.
# Uses a throwaway Redis prefix and the real database; the account and its
# submissions are deleted at the end. Needs the dev host (see test-obs-e2e.sh).
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"
SECONDS_TOTAL="${1:-300}" RATE="${2:-0.4}" KILL_AT="${3:-0}"

export LEETFORCE_QUEUE_PREFIX="lfdemo-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
export LEETFORCE_API_ADDR="127.0.0.1:18085"
export LEETFORCE_METRICS_QUEUE_EVERY=5s
export LEETFORCE_LIMIT_SUBMIT_USER=-1 LEETFORCE_LIMIT_SUBMIT_IP=-1
unset LEETFORCE_METRICS_ADDR LEETFORCE_S3_ENDPOINT
OBS="$HOME/obs"; mkdir -p "$OBS"; : >"$OBS/api.log"; sudo -n bash -c ": > $OBS/runner.log"
WORK="$(mktemp -d)"
API_PID="" RUNNER_PID=""

start_runner() {
  rm -f "$WORK/runner.pid"
  sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_PROBLEMS_DIR \
    bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >>"$OBS/runner.log" 2>&1 &
  for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
  RUNNER_PID="$(cat "$WORK/runner.pid")"
}
cleanup() {
  [ -n "$API_PID" ] && kill -TERM "$API_PID" 2>/dev/null || true
  [ -n "$RUNNER_PID" ] && sudo -n kill -KILL "$RUNNER_PID" 2>/dev/null || true
  bin/lfq destroy >/dev/null 2>&1 || true
  local name; name="$(sed -n 's/^account used: //p' "$WORK/load.out" 2>/dev/null | head -1)"
  if [[ "$name" =~ ^obs[0-9a-f]+$ ]]; then
    psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE username = '$name'); DELETE FROM users WHERE username = '$name'" >/dev/null 2>&1 || echo "warning: could not delete demo account $name" >&2
  fi
  sudo -n rm -rf "$WORK"
}
trap cleanup EXIT

make build-api build-runner >/dev/null
bin/api >>"$OBS/api.log" 2>&1 &
API_PID=$!
for _ in $(seq 120); do curl -fsS "http://$LEETFORCE_API_ADDR/readyz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "http://$LEETFORCE_API_ADDR/readyz" >/dev/null || { echo "the API did not become ready" >&2; exit 1; }
start_runner
echo "API metrics on 127.0.0.1:9102, runner metrics on 127.0.0.1:9101, logs in $OBS; load for ${SECONDS_TOTAL}s at ${RATE}/s" >&2

if [ "$KILL_AT" -gt 0 ]; then
  (
    sleep "$KILL_AT"; echo "demo: stopping the runner now" >&2
    sudo -n kill -KILL "$RUNNER_PID"
    sleep 60; echo "demo: restarting the runner" >&2
    start_runner
  ) &
fi
python3 scripts/obs-load.py --api "http://$LEETFORCE_API_ADDR" --seconds "$SECONDS_TOTAL" --rate "$RATE" | tee "$WORK/load.out"
wait || true

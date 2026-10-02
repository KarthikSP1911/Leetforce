#!/usr/bin/env bash
# Phase 3 exit criterion: kill a runner mid-job and check that another runner
# reclaims the job and exactly one verdict is recorded.
#
# Needs the dev host (Linux, nsjail, passwordless sudo, Go) and
# LEETFORCE_REDIS_URL (from the environment or ./.env). It uses a throwaway key
# prefix, so it never touches real queue data. Takes about a minute: the job is a
# Go solution, whose cold compile in the sandbox takes 10 to 20 seconds and gives
# a wide window to kill runner A while it is judging.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"

export LEETFORCE_QUEUE_PREFIX="lfcrash-$$-$(date +%s)"
export LEETFORCE_JOB_MIN_IDLE="${LEETFORCE_JOB_MIN_IDLE:-6s}"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
SUB="crash-test-$$"
SRC="problems/sample-sum/solutions/go/ac.go"
WORK="$(mktemp -d)"
PIDS=()

fail() { echo "FAIL: $*" >&2; echo "--- runner A log"; cat "$WORK/a.log" >&2 || true; echo "--- runner B log"; cat "$WORK/b.log" >&2 || true; exit 1; }

cleanup() {
  for p in "${PIDS[@]:-}"; do
    [ -n "$p" ] && sudo -n kill -KILL "$p" 2>/dev/null || true
  done
  bin/lfq destroy >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

make build-runner >/dev/null

# start_runner <id> <log>: runs bin/runner as root; the shell is replaced by the
# runner (exec) so the recorded PID is the runner's own PID.
start_runner() {
  local id="$1" log="$2" pidfile="$WORK/$1.pid"
  sudo -n bash -c 'echo $$ > "$1"; exec env LEETFORCE_RUNNER_ID="$2" bin/runner' _ "$pidfile" "$id" >"$log" 2>&1 &
  for _ in $(seq 50); do [ -s "$pidfile" ] && break; sleep 0.1; done
  [ -s "$pidfile" ] || fail "runner $id did not start"
  PIDS+=("$(cat "$pidfile")")
}

wait_for() { # wait_for <seconds> <description> <command...>
  local secs="$1" what="$2"; shift 2
  for _ in $(seq $((secs * 2))); do "$@" && return 0; sleep 0.5; done
  fail "timed out waiting for $what"
}

result_count() { bin/lfq results | grep -c "\"submission_id\":\"$SUB\"" || true; }

echo "1. start runner A and enqueue $SRC (id $SUB)"
start_runner runner-a "$WORK/a.log"
bin/lfq enqueue -id "$SUB" sample-sum go "$SRC"

echo "2. wait until runner A is judging, then kill -9 it"
wait_for 60 "runner A to start judging" grep -q '"msg":"judging"' "$WORK/a.log"
sudo -n kill -KILL "$(cat "$WORK/runner-a.pid")"
echo "   runner A killed mid-job"
sleep 1
[ "$(result_count)" = 0 ] || fail "a verdict was recorded before any runner finished"

echo "3. start runner B; after MinIdle ($LEETFORCE_JOB_MIN_IDLE) it must reclaim and judge the job"
start_runner runner-b "$WORK/b.log"
wait_for 120 "runner B to publish a verdict" test "$(result_count)" -ge 1

echo "4. check: exactly one verdict, AC, from runner B, job was a reclaim"
sleep 8  # a duplicate verdict, if the queue allowed one, would appear by now
[ "$(result_count)" = 1 ] || fail "expected exactly 1 verdict, got $(result_count)"
RESULT="$(bin/lfq results | grep "\"submission_id\":\"$SUB\"")"
echo "   $RESULT"
echo "$RESULT" | grep -q '"verdict":"AC"' || fail "verdict is not AC"
echo "$RESULT" | grep -q '"runner_id":"runner-b"' || fail "verdict was not reported by runner B"
grep -q '"reclaimed":true' "$WORK/b.log" || fail "runner B log does not show a reclaimed job"

echo "5. stop runner B with SIGTERM; it must exit cleanly"
PID_B="$(cat "$WORK/runner-b.pid")"
sudo -n kill -TERM "$PID_B"
wait_for 30 "runner B to exit" bash -c "! sudo -n kill -0 $PID_B 2>/dev/null"
grep -q '"msg":"runner stopped"' "$WORK/b.log" || fail "runner B did not log a clean stop"

echo "PASS: runner killed mid-job; job reclaimed and judged once"

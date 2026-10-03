#!/usr/bin/env bash
# Phase 13 exit test: the system runs in the cloud and survives a runner loss.
#
# HOW TO RUN (needs the deployed cloud stack; terminates one real EC2 runner):
#   export LEETFORCE_API_URL=http://<control-host>      # reachable from here
#   export AWS_PROFILE=leetforce                        # region defaults to ap-south-1
#   export LEETFORCE_CONFIRM_TERMINATE=yes              # you accept losing one runner host
#   export DATABASE_URL=...   # optional: enables the 1-row-per-submission check and
#                             # test-account cleanup (psql); without it the account stays
#   make test-runner-loss
# Optional: LEETFORCE_TEST_PROBLEM (default sample-sum), LEETFORCE_SUBMISSIONS (default 8,
#   max 10 because of the 10/min per-user submit limit), LEETFORCE_TIMEOUT_SEC (default
#   600), LEETFORCE_REPLACEMENT_TIMEOUT_SEC (default 420), LEETFORCE_EXPECT_REPLACEMENT
#   (yes|no|auto; auto = yes only when an Auto Scaling group tagged Role=runner exists),
#   LEETFORCE_KILL_INSTANCE (instance id to terminate instead of the first running runner).
#
# Why this workload: the test must kill a runner while it holds a job. The API does not
# say which runner holds which job, so we make every runner busy: the submissions cycle
# through slow-to-judge cases (Java and Go compiles take seconds, a C++ build and a TLE
# solution are not instant) of the problem's own reference solutions
# (problems/<slug>/solutions/<lang>/{ac,wa,tle}.*). Nothing sleeps, so every solution
# stays inside the problem's normal time-limit logic and its verdict is the known one
# (ac -> AC, wa -> WA, tle -> TLE). As soon as one submission is "judging" we terminate a
# runner; its in-flight job is then reclaimed from the Redis pending list (XAUTOCLAIM)
# by a survivor. The runner we kill is not guaranteed to hold a job (limitation: no
# job-to-runner mapping is exposed); with N busy submissions on 2+ runners it almost
# always does, and the assertions (every submission gets exactly one correct verdict)
# hold either way.
#
# Infra today (infra/aws/main.tf): runners are plain aws_instance resources named
# leetforce-runner-<n> (Role=runner), not an Auto Scaling group, so no replacement
# appears on its own. Step 6 therefore only asserts a replacement when an ASG exists (or
# LEETFORCE_EXPECT_REPLACEMENT=yes); otherwise it asserts the survivors keep the stack
# working and prints a note.
set -euo pipefail

usage() {
  sed -n '2,/^set -euo/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//' >&2
  exit 2
}

if [ "${1:-}" = "-h" ]; then usage; fi
if [ -z "${LEETFORCE_API_URL:-}" ]; then echo "LEETFORCE_API_URL is required" >&2; usage; fi
API="${LEETFORCE_API_URL%/}"
export AWS_DEFAULT_REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-ap-south-1}}"
if [ -z "${AWS_PROFILE:-}" ]; then echo "AWS_PROFILE is required (e.g. leetforce)" >&2; usage; fi
if [ "${LEETFORCE_CONFIRM_TERMINATE:-}" != "yes" ]; then
  echo "This test terminates one running runner EC2 instance (billable infrastructure)." >&2
  echo "Set LEETFORCE_CONFIRM_TERMINATE=yes to proceed." >&2
  exit 2
fi
for tool in curl python3 aws; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing tool: $tool" >&2; exit 2; }
done

cd "$(dirname "$0")/.."
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env # DATABASE_URL only; never printed
  set +a
fi

PROBLEM="${LEETFORCE_TEST_PROBLEM:-sample-sum}"
N="${LEETFORCE_SUBMISSIONS:-8}"
TIMEOUT="${LEETFORCE_TIMEOUT_SEC:-600}"
REPL_TIMEOUT="${LEETFORCE_REPLACEMENT_TIMEOUT_SEC:-420}"
EXPECT_REPL="${LEETFORCE_EXPECT_REPLACEMENT:-auto}"
case "$N" in '' | *[!0-9]*) echo "LEETFORCE_SUBMISSIONS must be a number" >&2; exit 2 ;; esac
if [ "$N" -lt 2 ] || [ "$N" -gt 10 ]; then echo "LEETFORCE_SUBMISSIONS must be 2..10" >&2; exit 2; fi
[ -d "problems/$PROBLEM/solutions" ] || { echo "no reference solutions under problems/$PROBLEM/solutions" >&2; exit 2; }

WORK="$(mktemp -d)"
JAR="$WORK/cookies"
USERNAME="e2e$(python3 -c 'import uuid; print(uuid.uuid4().hex[:10])')"
PASSWORD="e2e-password-$(python3 -c 'import uuid; print(uuid.uuid4().hex[:8])')"
FAILS=0

fail() { echo "FAIL: $*" >&2; FAILS=$((FAILS + 1)); }
note() { echo "-- $*"; }

cleanup() {
  if [ -n "${DATABASE_URL:-}" ] && command -v psql >/dev/null 2>&1; then
    psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE username = '$USERNAME'); DELETE FROM users WHERE username = '$USERNAME'" >/dev/null 2>&1 \
      || echo "warning: could not delete test account $USERNAME" >&2
  else
    echo "note: test account $USERNAME and its submissions were left in the database (no DATABASE_URL/psql)"
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# jfield <json> <dotted.path>: print a field, empty if missing.
jfield() {
  python3 -c '
import json, sys
try:
    v = json.loads(sys.argv[1])
    for k in sys.argv[2].split("."):
        v = v[k]
    print(v)
except Exception:
    pass' "$1" "$2"
}

running_runners() {
  aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=leetforce-runner*" "Name=instance-state-name,Values=running" \
    --query 'Reservations[].Instances[].InstanceId' --output text | tr '\t' '\n' | sed '/^$/d' | sort
}

# --- 1. health
note "1. health"
curl -fsS --max-time 15 "$API/healthz" >/dev/null || { echo "FAIL: $API/healthz" >&2; exit 1; }
curl -fsS --max-time 15 "$API/readyz" >/dev/null || { echo "FAIL: $API/readyz (API not ready)" >&2; exit 1; }

# --- 2. throwaway account
note "2. account $USERNAME"
BODY="$(python3 -c 'import json,sys; print(json.dumps({"email": sys.argv[1] + "@example.com", "username": sys.argv[1], "password": sys.argv[2]}))' "$USERNAME" "$PASSWORD")"
code="$(curl -sS --max-time 30 -o /dev/null -w '%{http_code}' -c "$JAR" -H 'Content-Type: application/json' -d "$BODY" "$API/auth/signup")"
[ "$code" = "201" ] || { echo "FAIL: signup returned $code" >&2; exit 1; }

# --- 3. runners
note "3. runners"
mapfile -t RUNNERS < <(running_runners)
BEFORE="${#RUNNERS[@]}"
echo "running runners: $BEFORE (${RUNNERS[*]:-none})"
if [ "$BEFORE" -lt 2 ]; then echo "FAIL: need at least 2 running runners (tag Name=leetforce-runner*), found $BEFORE" >&2; exit 1; fi
ASG="$(aws autoscaling describe-auto-scaling-groups --query "AutoScalingGroups[?Tags[?Key=='Role' && Value=='runner']].[AutoScalingGroupName,DesiredCapacity]" --output text 2>/dev/null || true)"
if [ -n "$ASG" ]; then
  echo "runner ASG (name desired): $ASG"
  if [ "$EXPECT_REPL" = auto ]; then EXPECT_REPL=yes; fi
else
  echo "no Auto Scaling group tagged Role=runner: runners are plain instances, no automatic replacement"
  if [ "$EXPECT_REPL" = auto ]; then EXPECT_REPL=no; fi
fi
VICTIM="${LEETFORCE_KILL_INSTANCE:-${RUNNERS[0]}}"

# --- 4. submit N solutions, then kill a runner once one is judging
note "4. submit $N solutions to $PROBLEM"
# Slow-to-judge first so all runners are busy: java, go, cpp, python tle, python wa.
PLAN=(java:ac go:ac cpp:ac python:tle python:wa java:wa go:wa cpp:tle java:tle go:tle)
declare -A EXT=([python]=py [cpp]=cpp [java]=java [go]=go)
IDS=()
WANT=()
for ((i = 0; i < N; i++)); do
  lang="${PLAN[i]%%:*}"
  kind="${PLAN[i]##*:}"
  file="problems/$PROBLEM/solutions/$lang/$kind.${EXT[$lang]}"
  [ -f "$file" ] || { echo "FAIL: missing reference solution $file" >&2; exit 1; }
  body="$(python3 -c 'import json,sys; print(json.dumps({"problem": sys.argv[1], "language": sys.argv[2], "source": open(sys.argv[3]).read()}))' "$PROBLEM" "$lang" "$file")"
  resp="$(curl -sS --max-time 30 -b "$JAR" -w '\n%{http_code}' -H 'Content-Type: application/json' -d "$body" "$API/submissions")"
  code="${resp##*$'\n'}"
  if [ "$code" != "201" ] && [ "$code" != "202" ]; then echo "FAIL: submit $lang/$kind returned $code" >&2; exit 1; fi
  id="$(jfield "${resp%$'\n'*}" id)"
  [ -n "$id" ] || { echo "FAIL: no id in submit response" >&2; exit 1; }
  IDS+=("$id")
  case "$kind" in ac) WANT+=(AC) ;; wa) WANT+=(WA) ;; tle) WANT+=(TLE) ;; esac
done

note "waiting for a submission to reach judging"
deadline=$((SECONDS + 120))
judging=no
while [ "$SECONDS" -lt "$deadline" ] && [ "$judging" = no ]; do
  for id in "${IDS[@]}"; do
    st="$(jfield "$(curl -sS --max-time 15 "$API/submissions/$id")" status)"
    if [ "$st" = judging ]; then
      judging=yes
      break
    fi
  done
  if [ "$judging" = no ]; then sleep 0.3; fi
done
if [ "$judging" = no ]; then echo "warning: no submission seen in 'judging' (all may have finished); terminating anyway" >&2; fi

echo "TERMINATING runner instance: $VICTIM"
aws ec2 terminate-instances --instance-ids "$VICTIM" --query 'TerminatingInstances[].CurrentState.Name' --output text
KILLED_AT=$SECONDS

# --- 5. every submission reaches exactly one correct verdict
note "5. waiting up to ${TIMEOUT}s for all verdicts"
declare -A GOT
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
  pending=0
  for id in "${IDS[@]}"; do
    [ -n "${GOT[$id]:-}" ] && continue
    j="$(curl -sS --max-time 15 "$API/submissions/$id" || true)"
    if [ "$(jfield "$j" status)" = judged ]; then GOT[$id]="$(jfield "$j" verdict.verdict)"; else pending=$((pending + 1)); fi
  done
  if [ "$pending" -eq 0 ]; then break; fi
  sleep 3
done
for i in "${!IDS[@]}"; do
  id="${IDS[i]}"
  if [ -z "${GOT[$id]:-}" ]; then
    fail "submission ${id:0:8} never reached a verdict (lost)"
  elif [ "${GOT[$id]}" != "${WANT[i]}" ]; then
    fail "submission ${id:0:8} verdict ${GOT[$id]}, expected ${WANT[i]}"
  fi
done
# No duplicates: the verdict must not change after a grace period, and (with psql) the
# verdicts table (PK submission_id) has exactly one row per submission.
sleep 20
for id in "${IDS[@]}"; do
  [ -n "${GOT[$id]:-}" ] || continue
  again="$(jfield "$(curl -sS --max-time 15 "$API/submissions/$id")" verdict.verdict)"
  [ "$again" = "${GOT[$id]}" ] || fail "submission ${id:0:8} verdict changed from ${GOT[$id]} to ${again:-none}"
done
if [ -n "${DATABASE_URL:-}" ] && command -v psql >/dev/null 2>&1; then
  idlist="$(printf "'%s'," "${IDS[@]}")"
  rows="$(psql "$DATABASE_URL" -qAt -c "SELECT count(*) FROM verdicts WHERE submission_id IN (${idlist%,})" 2>/dev/null || echo "?")"
  [ "$rows" = "$N" ] || fail "verdict rows for the $N submissions: $rows (want $N)"
else
  echo "note: no DATABASE_URL/psql, verdict row count not checked directly"
fi

# --- 6. replacement / survivors
note "6. runner count after the loss"
if [ "$EXPECT_REPL" = yes ]; then
  deadline=$((SECONDS + REPL_TIMEOUT))
  after=0
  while [ "$SECONDS" -lt "$deadline" ]; do
    after="$(running_runners | wc -l | tr -d ' ')"
    if [ "$after" -ge "$BEFORE" ]; then break; fi
    sleep 10
  done
  [ "$after" -ge "$BEFORE" ] || fail "runner count is $after after ${REPL_TIMEOUT}s, expected the original $BEFORE (replacement did not come up)"
else
  after="$(running_runners | wc -l | tr -d ' ')"
  echo "no replacement expected (no ASG): $after runner(s) running, was $BEFORE; survivors served the load"
  [ "$after" -ge 1 ] || fail "no running runners left"
fi

# --- 7. summary
echo "== summary: problem=$PROBLEM submissions=$N terminated=$VICTIM runners_before=$BEFORE runners_after=${after:-?} verdicts_after_kill_s=$((SECONDS - KILLED_AT))"
if [ "$FAILS" -ne 0 ]; then
  echo "FAIL: $FAILS check(s) failed" >&2
  exit 1
fi
echo "PASS: all $N submissions got exactly one correct verdict after a runner was terminated"

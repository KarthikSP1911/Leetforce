#!/usr/bin/env bash
# Phase 5 end-to-end test: live status, object storage, redaction, reaper.
#
# 1. Submit a correct (slowish) solution while no runner is up; follow
#    GET /submissions/:id/events with curl; start a runner that has NO problems
#    directory (it can only read tests from the S3 bucket). The stream must show
#    status queued, status judging, verdict AC (5/5), in that order.
# 2. Submit three hostile programs that echo the hidden test input and a secret
#    marker to stdout and stderr (runtime error) or into the compiler output
#    (compile error), plus a wrong answer. Collect every response the API gave
#    (POST, GET, the SSE streams, /problems) and check that none contains hidden
#    test input or expected output, the marker, the source or the test-set
#    version. A self-test shows the detector would notice a leak.
# 3. Create an orphan (a row stored but never queued, as after an API crash),
#    restart the API; the reaper must re-queue it and the runner must judge it.
#
# Needs the dev host (Linux, nsjail, passwordless sudo, Go, python3, psql),
# DATABASE_URL, LEETFORCE_REDIS_URL and the LEETFORCE_S3_* settings (environment
# or ./.env), and RustFS running (make dev). It uses a throwaway Redis key
# prefix, writes a few rows to the real submissions table and deletes the rows
# it created when it ends. The runner never receives DATABASE_URL.
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL (or put it in .env)}"
: "${LEETFORCE_S3_ENDPOINT:?set LEETFORCE_S3_ENDPOINT (RustFS from make dev) in .env}"

export LEETFORCE_QUEUE_PREFIX="lflive-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems" # the API's catalog; the runner gets a bogus one below
export LEETFORCE_API_ADDR="127.0.0.1:${E2E_PORT:-18082}"
BASE="http://$LEETFORCE_API_ADDR"
WORK="$(mktemp -d)"
CACHE="$WORK/problem-cache"
API_PID="" RUNNER_PID="" SSE_PID=""
IDS=()

fail() {
  echo "FAIL: $*" >&2
  echo "--- api log" >&2; tail -n 30 "$WORK/api.log" >&2 || true
  echo "--- runner log" >&2; tail -n 15 "$WORK/runner.log" >&2 || true
  exit 1
}

cleanup() {
  [ -n "$SSE_PID" ] && kill "$SSE_PID" 2>/dev/null || true
  [ -n "$API_PID" ] && kill -TERM "$API_PID" 2>/dev/null || true
  [ -n "$RUNNER_PID" ] && sudo -n kill -KILL "$RUNNER_PID" 2>/dev/null || true
  bin/lfq destroy >/dev/null 2>&1 || true
  if [ "${#IDS[@]}" -gt 0 ]; then # remove only the rows this run created (verdicts cascade)
    local list; list="$(printf "'%s'," "${IDS[@]}")"
    psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE id IN (${list%,})" >/dev/null 2>&1 || echo "warning: could not delete test rows: ${IDS[*]}" >&2
  fi
  if [ -s "$JAR" ]; then # the throwaway account, after its submissions
    psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE username = '$E2E_USER'); DELETE FROM users WHERE username = '$E2E_USER'" >/dev/null 2>&1 || echo "warning: could not delete test account $E2E_USER" >&2
  fi
  sudo -n rm -rf "$WORK"
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
JAR="$WORK/cookies"; E2E_USER="e2e$$"
ensure_login() { # sign up a throwaway account once; the session cookie is kept in $JAR
  [ -s "$JAR" ] && return 0
  curl -fsS -c "$JAR" -X POST -H 'Content-Type: application/json' \
    -d "{\"email\":\"$E2E_USER@example.com\",\"username\":\"$E2E_USER\",\"password\":\"e2e-password-1\"}" \
    "$BASE/auth/signup" >/dev/null || fail "sign-up failed"
}
get_sub() { curl -fsS "$BASE/submissions/$1"; }
submit_file() { # submit_file <language> <source-file>: prints the new id; the response goes to the evidence file
  local body resp
  ensure_login
  body="$(python3 -c '
import json, sys
print(json.dumps({"problem": "sample-sum", "language": sys.argv[1], "source": open(sys.argv[2]).read()}))' "$1" "$2")"
  resp="$(curl -fsS -b "$JAR" -X POST -H 'Content-Type: application/json' -d "$body" "$BASE/submissions")"
  echo "$resp" >>"$EVIDENCE"
  local id; id="$(field "$resp" id)"
  [ -n "$id" ] || fail "POST /submissions returned no id: $resp"
  IDS+=("$id")
  echo "$id"
}
start_api() { # start_api [ENV=VALUE ...]
  env "$@" bin/api >>"$WORK/api.log" 2>&1 &
  API_PID=$!
  ready() { curl -fsS "$BASE/readyz" >/dev/null 2>&1; }
  wait_for 60 "the API to be ready" ready
}
start_runner() {
  # No DATABASE_URL here, and a problems directory that does not exist: the
  # runner can only get tests from the S3 bucket (and it caches them in $CACHE).
  sudo -n --preserve-env=LEETFORCE_REDIS_URL,LEETFORCE_QUEUE_PREFIX,LEETFORCE_S3_ENDPOINT,LEETFORCE_S3_ACCESS_KEY,LEETFORCE_S3_SECRET_KEY,LEETFORCE_S3_BUCKET,LEETFORCE_S3_USE_TLS \
    env LEETFORCE_PROBLEMS_DIR=/nonexistent LEETFORCE_PROBLEM_CACHE="$CACHE" \
    bash -c 'echo $$ > "$1"; exec bin/runner' _ "$WORK/runner.pid" >"$WORK/runner.log" 2>&1 &
  for _ in $(seq 50); do [ -s "$WORK/runner.pid" ] && break; sleep 0.1; done
  [ -s "$WORK/runner.pid" ] || fail "the runner did not start"
  RUNNER_PID="$(cat "$WORK/runner.pid")"
}
event_names() { grep '^event: ' "$1" | sed 's/^event: //' | paste -sd, -; }

EVIDENCE="$WORK/evidence.txt"; : >"$EVIDENCE"
make build-api build-runner >/dev/null

# The secrets: distinctive pieces of the hidden test files (raw and JSON-escaped),
# a marker only our hostile programs know, and the test-set version.
MARK="LFSECRET$(date +%s%N)"
python3 - "$PWD/problems/sample-sum" "$MARK" >"$WORK/banned.json" <<'PY'
import glob, json, os, sys
root, mark = sys.argv[1], sys.argv[2]
spec = open(os.path.join(root, "problem.yaml")).read()
samples = next(l for l in spec.splitlines() if l.startswith("samples:"))
sample_names = {s.strip() for s in samples.split("[")[1].split("]")[0].split(",")}
banned = {mark}
for path in sorted(glob.glob(os.path.join(root, "tests", "*"))):
    name = os.path.basename(path).rsplit(".", 1)[0]
    if name in sample_names:
        continue
    text = open(path).read().strip()
    for piece in {text[:48].strip(), text[-48:].strip()}:
        if len(piece) >= 10:
            banned.add(piece)
print(json.dumps(sorted(banned)))
PY
python3 - "$WORK/banned.json" >"$WORK/banned.txt" <<'PY'
import json, sys
for b in json.load(open(sys.argv[1])):
    print(b.replace("\n", "\\n"))            # as a raw string with a visible newline
    print(json.dumps(b)[1:-1])               # as it would appear inside JSON
PY
leaks() { # leaks <file>: prints any banned string found in the file
  while IFS= read -r b; do
    grep -qF -- "$b" "$1" && echo "LEAK: $b"
  done <"$WORK/banned.txt"
  return 0
}
# Self-test: the detector must notice a leak, or "no leaks found" proves nothing.
printf '{"verdict":{"note":"%s"}}\n' "$MARK" >"$WORK/selftest.txt"
[ -n "$(leaks "$WORK/selftest.txt")" ] || fail "the leak detector missed a planted marker"
first_hidden="$(head -n 3 "$WORK/banned.txt" | tail -n 1)"
echo "$first_hidden" >"$WORK/selftest.txt"
[ -n "$(leaks "$WORK/selftest.txt")" ] || fail "the leak detector missed a planted hidden-test piece"
echo "    leak detector self-test ok ($(wc -l <"$WORK/banned.txt") strings watched)"

echo "1/3 live status: submit with no runner, follow the SSE stream, then start a runner"
start_api
# A correct solution that sleeps 0.6 s per test (limit 1 s, and sleeping uses no
# CPU), so "judging" lasts a few seconds. The stream reports state, not history,
# so a judgement faster than one poll (500 ms) can legitimately go straight from
# queued to the verdict; this keeps the test deterministic.
{ echo "import time"; echo "time.sleep(0.6)"; cat problems/sample-sum/solutions/python/ac.py; } >"$WORK/slow-ac.py"
ID="$(submit_file python "$WORK/slow-ac.py")"
echo "    submission $ID"
curl -sN --max-time 120 "$BASE/submissions/$ID/events" >"$WORK/sse-ac.txt" &
SSE_PID=$!
wait_for 20 "the stream to show queued" grep -q '"queued"' "$WORK/sse-ac.txt"
[ "$(field "$(get_sub "$ID")" status)" = "queued" ] || fail "expected queued while no runner is up"
echo "    stream shows queued; starting a runner with no problems directory"
start_runner
wait "$SSE_PID" || fail "the SSE stream did not end cleanly (curl exit $?)"
SSE_PID=""
cat "$WORK/sse-ac.txt" >>"$EVIDENCE"
[ "$(event_names "$WORK/sse-ac.txt")" = "status,status,verdict" ] || fail "expected events status,status,verdict, got: $(event_names "$WORK/sse-ac.txt")
$(cat "$WORK/sse-ac.txt")"
grep -q 'data: {"status":"queued"}' "$WORK/sse-ac.txt" || fail "first status was not queued"
grep -q 'data: {"status":"judging"}' "$WORK/sse-ac.txt" || fail "second status was not judging"
VERDICT_LINE="$(grep -A1 '^event: verdict' "$WORK/sse-ac.txt" | grep '^data: ' | sed 's/^data: //')"
[ "$(field "$VERDICT_LINE" verdict.verdict)" = "AC" ] || fail "expected AC, got: $VERDICT_LINE"
[ "$(field "$VERDICT_LINE" verdict.passed)" = "5" ] && [ "$(field "$VERDICT_LINE" verdict.total)" = "5" ] || fail "expected 5/5, got: $VERDICT_LINE"
echo "    stream: $(event_names "$WORK/sse-ac.txt"); $VERDICT_LINE"
sudo -n test -d "$CACHE" && [ -n "$(sudo -n ls "$CACHE")" ] || fail "the runner cache is empty: tests did not come from the bucket"
[ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/submissions/00000000-0000-4000-8000-000000000000/events")" = "404" ] || fail "events for an unknown submission must be 404"

echo "2/3 redaction: hostile programs echo the hidden input and a marker; nothing may come back"
cat >"$WORK/snoop.py" <<PY
import sys
data = sys.stdin.read()
sys.stdout.write("$MARK " + data)
sys.stderr.write("$MARK " + data)
sys.exit(3)
PY
cat >"$WORK/ce.cpp" <<CPP
#error $MARK hidden-test-leak
int main() { return 0; }
CPP
declare -A WANT=([snoop]=RE [ce]=CE [wa]=WA)
declare -A SUBS
SUBS[snoop]="$(submit_file python "$WORK/snoop.py")"
SUBS[ce]="$(submit_file cpp "$WORK/ce.cpp")"
SUBS[wa]="$(submit_file python problems/sample-sum/solutions/python/wa.py)"
for k in snoop ce wa; do
  id="${SUBS[$k]}"
  curl -sN --max-time 180 "$BASE/submissions/$id/events" >"$WORK/sse-$k.txt" || fail "stream for $k failed"
  cat "$WORK/sse-$k.txt" >>"$EVIDENCE"
  get_sub "$id" >>"$EVIDENCE"
  v="$(field "$(get_sub "$id")" verdict.verdict)"
  [ "$v" = "${WANT[$k]}" ] || fail "$k: expected ${WANT[$k]}, got $v"
  [ "$(field "$(get_sub "$id")" status)" = "judged" ] || fail "$k: not judged"
  echo "    $k: ${WANT[$k]} (stream: $(event_names "$WORK/sse-$k.txt"))"
done
curl -fsS "$BASE/problems" >>"$EVIDENCE"
curl -fsS "$BASE/problems/sample-sum" >"$WORK/problem-detail.txt"
cat "$WORK/problem-detail.txt" >>"$EVIDENCE"
# Not vacuous: the problem page does show the samples, so the capture works.
grep -q '"input"' "$WORK/problem-detail.txt" || fail "the problem page returned no samples"
found="$(leaks "$EVIDENCE")"
[ -z "$found" ] || fail "hidden data reached a response:
$found"
grep -qE 'ts-[0-9a-f]{16}' "$EVIDENCE" && fail "a test-set version reached a response"
grep -qF 'source' "$EVIDENCE" && fail "the word source appears in a response (the submitted source must not be returned)"
echo "    $(wc -c <"$EVIDENCE") bytes of responses checked: no hidden test data, marker, source or version"

echo "3/3 reaper: a row stored but never queued is re-queued after an API restart"
kill -TERM "$API_PID"; wait "$API_PID" 2>/dev/null || true; API_PID=""
ORPHAN="$(python3 -c 'import uuid; print(uuid.uuid4())')"
IDS+=("$ORPHAN")
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -qAt >/dev/null <<SQL
INSERT INTO submissions (id, problem_slug, language, source, test_set_version)
SELECT '$ORPHAN', slug, 'python', \$src\$$(cat problems/sample-sum/solutions/python/ac.py)\$src\$, test_set_version
FROM problems WHERE slug = 'sample-sum';
SQL
[ "$(psql "$DATABASE_URL" -qAt -c "SELECT enqueued_at IS NULL FROM submissions WHERE id = '$ORPHAN'")" = "t" ] || fail "the orphan row was not created unqueued"
echo "    orphan $ORPHAN stored with no job"
sleep 2
start_api LEETFORCE_REAPER_GRACE=1s
orphan_judged() { [ "$(field "$(get_sub "$ORPHAN")" status)" = "judged" ]; }
wait_for 90 "the orphan to be judged after the restart" orphan_judged
O="$(get_sub "$ORPHAN")"
[ "$(field "$O" verdict.verdict)" = "AC" ] || fail "expected AC for the orphan, got: $O"
grep -q 're-queued submissions' "$WORK/api.log" || fail "the API log has no reaper message"
[ "$(psql "$DATABASE_URL" -qAt -c "SELECT enqueued_at IS NOT NULL FROM submissions WHERE id = '$ORPHAN'")" = "t" ] || fail "the orphan was not marked enqueued"
echo "    reaper re-queued it, the runner judged it: $O"

echo "PASS: queued -> judging -> verdict over SSE with tests from the bucket; no hidden data in any response; an orphaned submission was re-queued and judged"

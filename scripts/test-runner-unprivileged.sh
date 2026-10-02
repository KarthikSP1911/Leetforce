#!/usr/bin/env bash
# Phase 6 unit 5 check: the runner as the hardened systemd unit (user lfrunner,
# no capabilities) judges real programs in nsjail. Needs the dev host, sudo and
# LEETFORCE_REDIS_URL (env or ./.env). Installs the unit (scripts/runner) and
# uses a throwaway queue prefix; stops the service and removes its env file at
# the end. Run: scripts/test-runner-unprivileged.sh
set -euo pipefail
cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${LEETFORCE_REDIS_URL:?set LEETFORCE_REDIS_URL}"
export LEETFORCE_QUEUE_PREFIX="lfpriv-$$-$(date +%s)"
export LEETFORCE_PROBLEMS_DIR="$PWD/problems"
fail() { echo "FAIL: $*" >&2; sudo -n journalctl -u leetforce-runner --no-pager -n 6 -o cat >&2 || true; exit 1; }
cleanup() {
  sudo -n systemctl stop leetforce-runner 2>/dev/null || true
  sudo -n rm -rf /etc/leetforce/runner.env /opt/leetforce/problems
  bin/lfq destroy >/dev/null 2>&1 || true
}
trap cleanup EXIT

make build-runner >/dev/null
sudo -n scripts/runner/install-runner.sh bin/runner >/dev/null
sudo -n rm -rf /opt/leetforce/problems && sudo -n cp -r problems /opt/leetforce/problems
sudo -n bash -c 'umask 027; cat > /etc/leetforce/runner.env' <<ENV
LEETFORCE_REDIS_URL=$LEETFORCE_REDIS_URL
LEETFORCE_QUEUE_PREFIX=$LEETFORCE_QUEUE_PREFIX
LEETFORCE_PROBLEMS_DIR=/opt/leetforce/problems
LEETFORCE_RUNNER_ID=lfrunner-test
ENV
sudo -n chgrp lfrunner /etc/leetforce/runner.env
sudo -n systemctl restart leetforce-runner
sleep 2
systemctl is-active --quiet leetforce-runner || fail "service not active"
PID="$(systemctl show -p MainPID --value leetforce-runner)"
echo "runner pid $PID uid $(sudo -n awk '/^Uid/{print $2}' /proc/$PID/status)"
sudo -n grep -E '^(Uid|CapEff|CapBnd|CapAmb|NoNewPrivs)' /proc/$PID/status
[ "$(sudo -n awk '/^Uid/{print $2}' /proc/$PID/status)" != 0 ] || fail "runner runs as root"
[ "$(sudo -n awk '/^CapEff/{print $2}' /proc/$PID/status)" = 0000000000000000 ] || fail "runner has capabilities"

n=0
check() { # check <lang> <file> <want-verdict>
  n=$((n+1)); local id="priv-$$-$n"
  bin/lfq enqueue -id "$id" sample-sum "$1" "problems/sample-sum/solutions/$1/$2" >/dev/null
  for _ in $(seq 240); do
    r="$(bin/lfq results | grep "\"submission_id\":\"$id\"" || true)"; [ -n "$r" ] && break; sleep 0.5
  done
  [ -n "${r:-}" ] || fail "$1/$2: no verdict"
  echo "$r" | grep -q "\"verdict\":\"$3\"" || fail "$1/$2: want $3, got $r"
  echo "ok $1/$2 -> $3"; r=""
}
for l in python cpp java go; do check "$l" "ac.${l/python/py}" AC; done
check python wa.py WA; check python tle.py TLE; check python mle.py MLE; check python re.py RE; check cpp ce.cpp CE
echo "PASS: unprivileged runner judged all cases"

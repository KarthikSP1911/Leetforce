#!/usr/bin/env bash
# Install the unprivileged LeetForce runner on a host (idempotent; run as root).
# Usage: sudo scripts/runner/install-runner.sh [path/to/runner-binary]
# Prerequisites: nsjail at /usr/local/bin/nsjail, systemd >= 254 (DelegateSubgroup),
# cgroup v2. Put LEETFORCE_* settings in /etc/leetforce/runner.env (mode 0640).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
bin="${1:-bin/runner}"
[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
[ -x "$bin" ] || { echo "runner binary $bin not found (make build-runner)" >&2; exit 1; }
[ -x /usr/local/bin/nsjail ] || { echo "nsjail missing at /usr/local/bin/nsjail" >&2; exit 1; }

id lfrunner >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin lfrunner
install -d -m 0755 /opt/leetforce/bin
install -m 0755 "$bin" /opt/leetforce/bin/runner
install -d -m 0750 -o root -g lfrunner /etc/leetforce
[ -e /etc/leetforce/runner.env ] || { install -m 0640 -o root -g lfrunner /dev/null /etc/leetforce/runner.env; echo "created empty /etc/leetforce/runner.env; add LEETFORCE_REDIS_URL etc." >&2; }

if [ -d /etc/apparmor.d ] && command -v apparmor_parser >/dev/null; then
  install -m 0644 "$here/usr.local.bin.nsjail" /etc/apparmor.d/usr.local.bin.nsjail
  apparmor_parser -r /etc/apparmor.d/usr.local.bin.nsjail
fi
install -m 0644 "$here/leetforce-runner.service" /etc/systemd/system/leetforce-runner.service
systemctl daemon-reload
echo "installed. Start with: systemctl enable --now leetforce-runner"

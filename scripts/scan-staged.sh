#!/usr/bin/env bash
# Trivy scan of exactly what is staged (the index), run on the dev host, because
# the working tree holds git-ignored .env files with real secrets.
#   scripts/scan-staged.sh        secret scan (before every commit)
#   scripts/scan-staged.sh full   vuln + secret + misconfig, HIGH and CRITICAL (before merging a unit or phase)
# Needs `ssh leetforce-dev` (override with LEETFORCE_DEV_HOST) and Trivy on the host.
set -euo pipefail
if [ "${1:-}" = "full" ]; then
  flags="--scanners vuln,secret,misconfig --severity HIGH,CRITICAL"
else
  flags="--scanners secret"
fi
git archive "$(git write-tree)" | ssh "${LEETFORCE_DEV_HOST:-leetforce-dev}" \
  "rm -rf /tmp/scan && mkdir /tmp/scan && tar -x -C /tmp/scan && cd /tmp/scan && trivy --version | head -1 && trivy fs $flags --exit-code 1 --quiet ."
echo "trivy scan of the staged tree (${1:-secret}): clean"

#!/usr/bin/env bash
# Secret scan of exactly what is staged (the index), run with Trivy on the dev
# host, because the working tree holds git-ignored .env files with real secrets.
# Usage: scripts/scan-staged.sh   (needs `ssh leetforce-dev` and Trivy on the host)
set -euo pipefail
git archive "$(git write-tree)" | ssh "${LEETFORCE_DEV_HOST:-leetforce-dev}" \
  'rm -rf /tmp/scan && mkdir /tmp/scan && tar -x -C /tmp/scan && cd /tmp/scan && trivy fs --scanners secret --exit-code 1 --quiet .'
echo "trivy secret scan of the staged tree: clean"

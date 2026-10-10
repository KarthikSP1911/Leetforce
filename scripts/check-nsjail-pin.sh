#!/usr/bin/env bash
# Fail when the nsjail commit in runner/Dockerfile differs from the one the Ansible role pins
# (ansible/roles/runner_host/defaults/main.yml). Both must stay equal: the adversarial suite
# is run against that one build (ADR 0029). Static and offline. WRITTEN, NOT RUN (Phase 17).
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
docker_commit="$(sed -n 's/^ARG NSJAIL_COMMIT=\([0-9a-f]\{40\}\)$/\1/p' "$here/runner/Dockerfile")"
ansible_commit="$(sed -n 's/^runner_host_nsjail_commit: \([0-9a-f]\{40\}\)$/\1/p' "$here/ansible/roles/runner_host/defaults/main.yml")"
if [ -z "$docker_commit" ] || [ -z "$ansible_commit" ]; then
  echo "check-nsjail-pin: could not read a 40-character commit from both files" >&2
  exit 2
fi
if [ "$docker_commit" != "$ansible_commit" ]; then
  echo "check-nsjail-pin: runner/Dockerfile pins $docker_commit but the Ansible role pins $ansible_commit" >&2
  exit 1
fi
echo "check-nsjail-pin: both pin $docker_commit"

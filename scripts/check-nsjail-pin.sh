#!/usr/bin/env bash
# Fail when two places that must agree have drifted apart (Phase 17, ADR 0029). Static and
# offline. WRITTEN, NOT RUN.
#   1. the nsjail commit in runner/Dockerfile and in the Ansible role runner_host: the adversarial
#      suite is run against one nsjail build, and the container must ship that build;
#   2. the k3s version in the Ansible roles k3s_server and k3s_agent and in the Terraform
#      variable runner_node_k3s_version: agents must match the control host.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

pick() { # pick <file> <sed expression with one capture group>
  sed -n "$2" "$1" | head -n1
}

fail=0
same() { # same <what> <value> <value> [<value>...]
  local what="$1" first="$2"
  shift 2
  if [ -z "$first" ]; then
    echo "check-nsjail-pin: could not read $what from the first file" >&2
    fail=1
    return
  fi
  local v
  for v in "$@"; do
    if [ "$v" != "$first" ]; then
      echo "check-nsjail-pin: $what differs: '$first' vs '$v'" >&2
      fail=1
      return
    fi
  done
  echo "check-nsjail-pin: $what agrees ($first)"
}

same "nsjail commit" \
  "$(pick "$here/runner/Dockerfile" 's/^ARG NSJAIL_COMMIT=\([0-9a-f]\{40\}\)$/\1/p')" \
  "$(pick "$here/ansible/roles/runner_host/defaults/main.yml" 's/^runner_host_nsjail_commit: \([0-9a-f]\{40\}\)$/\1/p')"

same "k3s version" \
  "$(pick "$here/ansible/roles/k3s_server/defaults/main.yml" 's/^k3s_server_version: "\([^"]*\)"$/\1/p')" \
  "$(pick "$here/ansible/roles/k3s_agent/defaults/main.yml" 's/^k3s_agent_version: "\([^"]*\)"$/\1/p')" \
  "$(pick "$here/infra/aws/variables.tf" 's/^ *default *= *"\(v[0-9][^"]*+k3s[0-9]*\)"$/\1/p')"

exit "$fail"

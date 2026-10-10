#!/usr/bin/env bash
# Copy the k3s agent join token from the control host into SSM Parameter Store, where the
# k3s agent nodes read it at first boot (Phase 17, ADR 0029). WRITTEN, NEVER RUN.
# Run on the owner's machine with AWS_PROFILE set and SSH access to the control host.
# Dry run by default: it prints the parameter NAME and never the token. Pass --apply to write.
#
#   scripts/k3s/push-agent-token.sh ubuntu@<control host> [--apply]
#
# The token never appears on a command line: ssh writes it to a mode 0600 temp file and the
# AWS CLI reads the value from that file.
set -euo pipefail
prefix="${LEETFORCE_SSM_PREFIX:-/leetforce}"
region="${AWS_REGION:-ap-south-1}"
name="$prefix/runner/K3S_AGENT_TOKEN"
target="${1:-}"
apply=0
[ "${2:-}" = "--apply" ] && apply=1
[ -n "$target" ] || { echo "usage: push-agent-token.sh <user@control-host> [--apply]" >&2; exit 2; }

if [ "$apply" != 1 ]; then
  echo "would read the k3s agent token from $target and write $name (dry run: pass --apply)"
  exit 0
fi

umask 077
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# agent-token joins agents only; it cannot start a server. k3s creates it next to the server token.
ssh "$target" sudo cat /var/lib/rancher/k3s/server/agent-token >"$work/token"
[ -s "$work/token" ] || { echo "push-agent-token: the token file on $target is empty or missing" >&2; exit 1; }
aws ssm put-parameter --region "$region" --name "$name" --type SecureString --overwrite \
  --value "file://$work/token" >/dev/null
echo "wrote $name"

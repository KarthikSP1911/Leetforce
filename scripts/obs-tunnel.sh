#!/usr/bin/env bash
# Phase 11: forwards the dev host's /metrics ports to this machine so the
# Prometheus container can scrape them (observability/prometheus/prometheus.yml).
#   local 9102 -> host 127.0.0.1:9102   API    (LEETFORCE_METRICS_ADDR default)
#   local 9101 -> host 127.0.0.1:9101   runner (LEETFORCE_METRICS_ADDR default)
# The listeners on the host are bound to localhost, so this tunnel is the only
# way to reach them; nothing new is opened in the security group.
# Run it in its own terminal and leave it open: scripts/obs-tunnel.sh [ssh-host]
set -euo pipefail
HOST="${1:-leetforce-dev}"
echo "forwarding 127.0.0.1:9101 (runner) and 127.0.0.1:9102 (API) from $HOST; Ctrl+C stops" >&2
exec ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
  -L 0.0.0.0:9101:127.0.0.1:9101 -L 0.0.0.0:9102:127.0.0.1:9102 "$HOST"

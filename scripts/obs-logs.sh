#!/usr/bin/env bash
# Phase 11: copies the API and runner JSON logs from the dev host into
# observability/logs/ so Alloy can ship them to Loki. The demo script
# (scripts/obs-demo.sh) writes them to ~/obs on the host. Run in its own
# terminal: scripts/obs-logs.sh [ssh-host]
set -euo pipefail
cd "$(dirname "$0")/.."
HOST="${1:-leetforce-dev}"
mkdir -p observability/logs
echo "tailing $HOST:~/obs/{api,runner}.log into observability/logs/; Ctrl+C stops" >&2
ssh "$HOST" 'mkdir -p ~/obs && touch ~/obs/api.log ~/obs/runner.log && tail -n 0 -F ~/obs/api.log' >>observability/logs/api.log &
A=$!
trap 'kill $A 2>/dev/null || true' EXIT
ssh "$HOST" 'sudo -n tail -n 0 -F ~/obs/runner.log' >>observability/logs/runner.log

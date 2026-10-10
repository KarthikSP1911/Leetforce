#!/usr/bin/env bash
# Start the LeetForce public demo on the dev host: API, runner, web, Cloudflare quick tunnel.
# Layout under ~/demo: bin/{api,runner,cloudflared}, web/ (next build + prod node_modules), node/ (Node 22).
# Secrets come from ~/Leetforce/.env, never printed.
set -euo pipefail
DEMO="$HOME/demo"; LOGS="$DEMO/logs"; mkdir -p "$LOGS"
set -a; . "$HOME/Leetforce/.env"; set +a
export LEETFORCE_PROBLEMS_DIR="$HOME/Leetforce/problems"
export LEETFORCE_S3_ENDPOINT=""   # host .env still points at a local RustFS; read problems from disk instead (README step 1)
export LEETFORCE_REDIS_URL="redis://127.0.0.1:6379/0"   # hosted Upstash hit its 500k/month limit; API and runner share this host
export LEETFORCE_API_ADDR="127.0.0.1:8080"
export LEETFORCE_TRUSTED_PROXIES="127.0.0.1"
export LEETFORCE_API_URL="http://127.0.0.1:8080"
export PATH="$DEMO/node/bin:$PATH"

start() { # start <name> <cmd...>: detached, log to $LOGS/<name>.log, pid to $LOGS/<name>.pid
  local name="$1"; shift
  setsid nohup "$@" >"$LOGS/$name.log" 2>&1 < /dev/null &
  echo $! >"$LOGS/$name.pid"
}
cd "$DEMO"
start api bin/api
for _ in $(seq 60); do curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1 && break; sleep 1; done
curl -fsS http://127.0.0.1:8080/readyz >/dev/null || { echo "API not ready; see $LOGS/api.log"; exit 1; }
start runner sudo -E bin/runner
(cd web && start web node node_modules/next/dist/bin/next start -H 127.0.0.1 -p 3000) 
for _ in $(seq 30); do curl -fsS http://127.0.0.1:3000/ >/dev/null 2>&1 && break; sleep 1; done
start tunnel bin/cloudflared tunnel --no-autoupdate --url http://127.0.0.1:3000
for _ in $(seq 30); do grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' "$LOGS/tunnel.log" 2>/dev/null | head -1 | grep . && break; sleep 1; done

# ADR 0019: Observability stack and metric design

Status: accepted (Phase 11). Decision 1 was made by the owner in chat; 2 and 3 are Claude defaults the owner did not change.

## Context
Phases 1 to 10 gave us a working submit-to-verdict flow with no way to see it. Phase 11 needs Prometheus metrics with the `leetforce_` prefix, Grafana dashboards, Loki logs and alerts for queue depth and runner health, and the exit criterion is that the dashboards show a live submission flow.

Constraints found at the start of the session:
- The dev host has 911 MB RAM (about 430 MB used) and 578 MB of free disk (96% full). The four images alone are about 1.5 GB.
- Runners must not connect to the database (CLAUDE.md), and the API and runners only reach each other through Redis.
- Redis is hosted (Upstash) and has a monthly command budget, so polling it often costs real quota.
- Nothing runs permanently on the dev host: the API and runner are started by test scripts.

## Decision
1. **The stack runs on the owner's Windows machine in Docker Compose** (`observability/docker-compose.yml`), bound to localhost. Prometheus scrapes the dev host through an SSH tunnel (`scripts/obs-tunnel.sh`); the metrics listeners on the host are bound to `127.0.0.1`, so no security-group rule is added and nothing new is billable.
2. **Metrics are pulled** from a separate listener in each process (API `127.0.0.1:9102`, runner `127.0.0.1:9101`, both configurable, `off` disables), never from the public API port. Each component has its own private registry (`api/internal/metrics`, `runner/internal/metrics`) so tests cannot register a metric twice.
3. **Queue depth is sampled by the API**, not by runners or an exporter: `queue.Stats` reads the stream length, the pending count, the oldest entry and the dead-letter length in one pipeline, and a goroutine publishes them as gauges every 60 seconds by default (`LEETFORCE_METRICS_QUEUE_EVERY`). Waiting is `XLEN - pending` because acknowledged jobs are deleted from the stream (`XLEN` plus `XDEL` on ack), which also means `XINFO GROUPS` lag is unreliable. Age is measured against the Redis `TIME`, so a skewed API host cannot fake a stuck queue.
4. **Runner health comes from Prometheus `up` and from the runner's own `last_poll_timestamp`.** `up == 0` is a dead or unreachable runner; a stale poll time is a runner whose job loop is stuck.
5. **Labels are bounded sets** (method, route template, status, verdict, language, scope, stage). Unknown verdict or language values map to `other`. Submission, user and problem ids are never labels; they stay in the logs.
6. **Logs**: the services already write JSON with `slog`. Alloy tails log files and sends them to Loki with only `service` and `level` as labels. `scripts/obs-logs.sh` copies the host's logs into `observability/logs/`, because Alloy on Windows cannot read the remote disk.
7. **Alerts are Prometheus rules with no notifier.** Firing alerts show on the Prometheus Alerts page and on the dashboard. Rules are unit tested with `promtool` (`make test-alerts`).

## Alternatives considered
- **Stack on the dev host:** does not fit in disk or RAM; growing the volume is billable and still leaves RAM tight.
- **Pushing metrics (Pushgateway, or runners posting to the API):** would let runners live behind NAT later, but adds a component and makes a dead runner look the same as an idle one. Revisit in Phase 13 when runners run in k3s (scraping pods is native there).
- **Exposing `/metrics` on the API port:** simpler, but public. A separate localhost listener costs ten lines.
- **A queue exporter (redis_exporter):** generic, but it cannot tell waiting from pending after our `XDEL`, and it is one more process to run.
- **Alertmanager and a notifier (Slack, email):** needs a destination and a secret; the owner can add it later without changing the rules.
- **Promtail:** end of life; Alloy replaces it.

## Consequences
- Queue gauges cost 5 Redis commands per sample: about 216k commands a month at 60 seconds, 1.3M at 10 seconds (the default is now 5 minutes, about 43k a month, ADR 0028). The e2e test and the demo set 1 to 5 seconds on a throwaway prefix for a few minutes only. Record this against the Upstash plan (unchecked since Phase 4).
- Dashboards need three things running together: the SSH tunnel, `scripts/obs-logs.sh` and the compose stack. Nothing alerts you if you forget the tunnel except `APIDown` and `RunnerDown`, which look the same as a real outage.
- A second runner on one host cannot bind port 9101; it logs the error and keeps judging, so metrics exist for one runner per host until a per-runner port is configured (Phase 13).
- Provisioning the metrics ports and log directory with Ansible, and running the stack in the cloud, are deferred to Phase 12 and 13.

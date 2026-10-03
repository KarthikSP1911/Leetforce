# Phase 11: Observability

**Branch:** `phase/11-observability`
**Range:** `phase-11-start..phase-11-done`
**Dates:** 2026-10-03 (one session)
**Milestone:** none
**Log:** [phase-11-log.md](phase-11-log.md) · **ADR:** [0019](../adr/0019-observability.md)

## Summary
The API and the runner now expose `leetforce_` Prometheus metrics on localhost-only listeners, the API publishes queue depth as gauges, and a Prometheus, Grafana, Loki and Alloy stack (run on the owner's PC, not the 1 GB dev host) shows the submission flow on a provisioned dashboard with ten alert rules. A live test proves the counters, histogram and queue gauges follow a real submit-to-verdict flow and a lost runner.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Prometheus metrics with the `leetforce_` prefix | ✅ | `make test-obs-e2e` on the dev host: submissions created = 12, verdicts stored = 12, POST /submissions 202 = 12, runner jobs = 12, judge histogram count = 12 |
| Dashboards show a live submission flow | ✅ (queries); ⚠ (eyes) | `scripts/obs-demo.sh 200 0.5 80` with the tunnel: Prometheus targets api, runner, prometheus all `up`; verdicts by type (AC 9, WA 1, MLE 1 at the first look), queue waiting 17 with oldest job 31 s. The dashboard JSON loaded into Grafana (`/api/search` lists `lf-flow`). Not yet confirmed: the owner looking at the rendered panels |
| Loki logs | ✅ | Loki query `{service=~"api|runner"}` returned both streams with `service` and `level` labels |
| Queue-depth and runner-health alerts | ✅ rules; ⚠ firing | `promtool check rules` (10 rules) and `promtool test rules` SUCCESS for RunnerDown, QueueStuck, QueueBacklog, InternalErrorVerdicts. In the live demo `RunnerDown` and `QueueBacklog` reached pending; the runner came back before `RunnerDown` completed its 1 minute (the demo outage is 60 s), so firing was shown by the promtool test, not live |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/11-api-metrics` | API metrics, `queue.Stats`, queue sampler | 1 |
| `feat/11-runner-metrics` | runner metrics and listener | 1 |
| `feat/11-stack` | Prometheus, Grafana, Loki, Alloy, dashboard, alerts, scripts | 1 |
| (test commit on the phase branch) | load driver and live-flow test | 1 |
| `fix/11-lint-and-modes` | `noctx` lint, executable bits | 1 |
| `fix/11-runner-no-sql` | keep `database/sql` out of the runner | 1 |
| `fix/11-e2e-metric-helper` | `grep -E` in the test helper | 1 |
| `fix/11-grafana-port` | Grafana on 3001 | 1 |

## File-by-file changes
Generated from `git diff --name-status phase-11-start..HEAD` after the last commit (47 files: 26 added, 21 modified).

### Added
| File | Purpose |
|---|---|
| `queue/stats.go`, `queue/stats_test.go` | `Queue.Stats`: waiting, pending, oldest age (Redis clock), dead letters in one pipeline; test needs real Redis |
| `api/internal/metrics/metrics.go`, `metrics_test.go` | API registry, all `leetforce_` series, queue sampler, handler |
| `runner/internal/metrics/metrics.go` | Runner registry (`leetforce_runner_`), listener helper |
| `runner/internal/agent/metrics_test.go` | verdict counter, in-flight gauge, reclaim counter (need real Redis) |
| `runner/go.sum` | checksums for the new Prometheus dependency |
| `observability/docker-compose.yml` | the four services, localhost-only ports (Grafana 3001) |
| `observability/prometheus/prometheus.yml`, `alerts.yml` | scrape targets (tunnel ends) and 10 alert rules |
| `observability/tests/alerts_test.yml` | promtool tests for four alerts |
| `observability/loki/loki.yml`, `observability/alloy/config.alloy` | Loki storage and 7-day retention; log tailing and labels |
| `observability/grafana/provisioning/datasources/datasources.yml`, `.../dashboards/dashboards.yml` | datasources `lf-prometheus`, `lf-loki`; dashboard provider |
| `observability/grafana/dashboards/submission-flow.json` | 25-panel dashboard (queue, flow, verdicts, judge time, runners, API, alerts, logs) |
| `observability/logs/.gitkeep` | keeps the log copy directory in git (contents ignored) |
| `scripts/obs-load.py`, `scripts/test-obs-e2e.sh` | load driver and the exit test |
| `scripts/obs-demo.sh`, `scripts/obs-tunnel.sh`, `scripts/obs-logs.sh` | live demo on the host, SSH tunnel (localhost-bound), log copy |
| `docs/adr/0019-observability.md` | decision record |
| `docs/phases/phase-11-log.md`, `phase-11.md`, `phase-11-summary.md` | running log with commands and why, this report, plain-language summary |

### Modified
| File | What changed | Why |
|---|---|---|
| `api/cmd/api/main.go` | separate metrics listener, `LEETFORCE_METRICS_ADDR`, `LEETFORCE_METRICS_QUEUE_EVERY`, sampler start | `/metrics` must not be on the public port |
| `api/internal/server/server.go` | request counter and latency by route template | bounded labels |
| `api/internal/server/limits.go`, `events.go`, `submissions.go`, `runs.go` | 429 by scope, SSE gauge, created counters | flow visibility |
| `api/internal/ingest/ingest.go` | verdict and write-outcome counters | verdict mix, IE detection |
| `api/internal/reaper/reaper.go`, `api/internal/rejudge/rejudge.go` | re-queue counters | Phase 10 flows become visible |
| `api/internal/server/server_test.go` | route-template counting test | covers the middleware |
| `api/go.mod`, `api/go.sum`, `runner/go.mod` | `prometheus/client_golang v1.24.1` | metrics library |
| `runner/cmd/runner/main.go`, `runner/internal/agent/agent.go`, `run.go` | listener and hooks | runner health and judge time |
| `Makefile` | `test-obs-e2e`, `test-alerts`, `dev-obs`, `down-obs` | commands for the phase |
| `.gitignore`, `.env.example` | ignore copied logs; `LEETFORCE_GRAFANA_PASSWORD` name | no secrets, no log files in git |
| `docs/FLOW.md`, `docs/PROGRESS.md` | Phase 11 flow as built; tracker row and status | docs rule |

### Deleted / Renamed
None.

## Key code changes
- **Queue depth without trusting `XINFO` lag** (`queue/stats.go`): `Ack` deletes entries, so group lag becomes unreliable; waiting is `XLEN - pending` and the first entry gives the oldest age, measured against Redis `TIME`.
- **Separate localhost metrics listener** (`api/cmd/api/main.go`, `runner/internal/metrics`): the public API port never serves `/metrics`.
- **Runner stays database-free** (`runner/internal/metrics/metrics.go`): `client_golang/prometheus/collectors` imports `database/sql`; the root-package collectors are used instead so `runner/nodb_test.go` keeps passing unchanged.
- **Bounded labels**: unknown verdicts and languages become `other`; routes use the template; no submission or user ids in labels.

## Decisions
- [ADR 0019](../adr/0019-observability.md): stack on the owner's PC (dev host has 571 MB disk free), pull metrics through an SSH tunnel, API samples the queue, Alloy for logs, Prometheus rules with no notifier.

## Tests
- `go test ./...` in `api` (metrics sampler, handler, route counting), `queue` (`TestStatsCountsWaitingPendingAndDead`, needs Redis), `runner` (metrics tests, `TestRunnerHasNoDatabaseDependency`).
- `make test-alerts` (promtool check + 4 rule tests; needs Docker), `make test-obs-e2e` (dev host).
- Note: on the dev host `make test` skips Redis tests unless `LEETFORCE_TEST_REDIS_URL` is set; the queue stats test was run with it pointed at the Upstash URL (unique prefix, cleaned up by the test helper).

## Known issues and deferred work
- The first run of `make test-obs-e2e` failed at "2 waiting jobs" (timed out after 20 s). The cause was not identified; the helper was changed (grep instead of awk -v) and the second run passed all 11 checks. A flake in that step is possible; if it recurs, capture the API `/metrics` output when it times out.
- Runner and API metrics need the SSH tunnel; nothing warns you the tunnel is closed except `APIDown` and `RunnerDown`.
- One runner per host can bind port 9101; extra runners log the failure and keep judging (Phase 13).
- Redis command budget: 5 commands per queue sample (about 216k a month at 60 s). The Upstash plan limit is still unchecked.
- Ansible provisioning of the metrics ports and log directory is Phase 12; running the stack in the cloud is Phase 13.
- No notifier (email, Slack): add one in Alertmanager if wanted.
- `make test-adversarial` not run: no sandbox code changed.
- Trivy full scan before the merge to `main`: clean (see the log).
- The review was skipped by the owner ("go"): understanding questions unanswered; decision A answered (no notifier); decision B stays at 60 s.
- The Phase 9 and 10 pages and the Phase 11 dashboard are not yet seen by the owner in a browser.

## Stats
- Commits: 11 (excluding merges; the 8 listed under branches plus two docs commits and the tunnel fix made directly on the phase branch, and this final record)
- Files: 26 added, 21 modified, 0 deleted
- Lines: about +2854 / -14 (before this final record)

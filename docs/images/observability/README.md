# Observability screenshots

Taken on 2026-10-10 from the local Compose stack (`observability/`) while a load test (`tools/loadtest`, 6 users, 3 minutes, 616 submissions, 0 errors) ran against the API on the dev host. The load tool sends a trivial program, so every verdict is WA. Details: `docs/phases/phase-16-log.md`, section "Observability evidence".

| File | Shows |
|---|---|
| `01-prometheus-targets.jpg` | Prometheus scrape targets: API and runner UP |
| `02-prometheus-alert-rules.jpg` | Alert rules (all inactive) |
| `03-grafana-dashboard-queue.jpg` | Dashboard, Queue: waiting, being judged, oldest job, dead letters, runners up |
| `04-grafana-dashboard-submission-flow.jpg` | Dashboard, Submission flow: submissions and verdicts per minute, judge time |
| `05-grafana-dashboard-runners.jpg` | Dashboard, Runners: jobs in flight, queue poll age, reclaimed jobs |
| `06-grafana-dashboard-api-and-alerts.jpg` | Dashboard, API: requests per second, p95 latency by route; alerts |
| `07-grafana-dashboard-errors-and-warnings.jpg` | Dashboard, errors and warnings from Loki (none during the run) |
| `08-loki-logs-query.jpg` | Loki Explore query `{service="runner"} \| json` |
| `09-loki-runner-logs.jpg` | Runner JSON logs: judging and verdict reported lines |
| `10-tempo-trace-list.jpg` | Tempo TraceQL results: API route traces |
| `11-tempo-trace-submission.jpg` | One trace: `POST /submissions` > `queue.enqueue` > `runner.process` > `judge` |

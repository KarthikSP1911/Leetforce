# Phase 11 summary: Observability

## TL;DR
- The API and the runner now publish numbers about themselves (requests, submissions, verdicts, judge time, queue depth, runner health) that Prometheus collects every 15 seconds.
- A dashboard in Grafana shows the submission flow live, Loki keeps the logs searchable, and ten alert rules say when the queue is stuck, a runner is gone, or the platform itself is failing.
- A test (`make test-obs-e2e`) submits real solutions and checks that the numbers moved by exactly the right amount, including after a runner is killed.

## Where this phase fits
```
 browser --> API (P4, P9) --> Redis Streams queue (P3) --> runner (P3) --> sandbox (P1, P2, P6) --> verdict
                |                     |                        |                                      |
                |  [P11 NEW] request, submission, 429, SSE counters         [P11 NEW] jobs, judge time, reclaimed
                |  [P11 NEW] queue sampler: waiting, pending, oldest age, dead letters
                v                     v                        v
        /metrics :9102  -------- SSH tunnel -------->  Prometheus --> Grafana dashboard  [P11 NEW]
        /metrics :9101 (runner)                           |--> alert rules (10)         [P11 NEW]
        JSON logs --> scripts/obs-logs.sh --> Alloy --> Loki --> Grafana logs panel      [P11 NEW]
 Still to come: infrastructure as code (P12), cloud (P13), contests (P14-15)
```
- It depends on every earlier phase (it measures them) and mainly on Phase 3 (the queue) and Phase 10 (rejudges and reaper re-queues now show up as numbers).
- It unblocks Phase 13: running in the cloud without these numbers would be flying blind, and the Phase 12 Terraform and Ansible work can now provision the metrics ports and log directory.

## What I built and why
### API metrics (`feat/11-api-metrics`)
- **What:** `api/internal/metrics` holds the `leetforce_` counters, histograms and gauges; they are served on their own port (`127.0.0.1:9102`).
- **Why:** nothing could tell you how many submissions arrived, how they ended, or whether users were being rate limited.
- **How it works:** a middleware counts every request under its route template (not the raw URL, so one problem does not create a thousand series); the places where a submission is created, a verdict is stored, a 429 is returned, a stream opens, or a rejudge happens each add one to a counter. `queue/stats.go` reads the queue in one round trip and a goroutine turns it into gauges every minute.
- **Alternatives considered:** putting `/metrics` on the public port (public, so no); a generic Redis exporter (cannot tell waiting from in-progress after our delete-on-ack). See ADR 0019.

### Runner metrics (`feat/11-runner-metrics`)
- **What:** `runner/internal/metrics`: jobs by verdict, judge time by language, jobs in flight, reclaimed and lost jobs, host failures, and the time of the last queue poll.
- **Why:** to tell a healthy runner from a dead one (`up` is 0) and from a stuck one (last poll is old).
- **How it works:** hooks in `agent.go` and `run.go`; its own listener on `127.0.0.1:9101`. The runner still talks only to Redis and the API; Prometheus pulls from it.
- **Gotcha I hit:** the usual Prometheus helper package imports `database/sql`, and a test forbids that in the runner. I used the older constructors instead rather than weaken the test.

### The stack (`feat/11-stack`)
- **What:** `observability/` with Docker Compose for Prometheus, Grafana, Loki and Alloy, plus the config, the dashboard JSON and the alert rules.
- **Why on your PC:** the dev host has 911 MB of RAM and about 570 MB of disk free; the images alone need about 1.5 GB.
- **How it works:** `scripts/obs-tunnel.sh` forwards the host's two metrics ports to your PC; Prometheus scrapes them. `scripts/obs-logs.sh` copies the host's log files to `observability/logs`, and Alloy sends them to Loki. Grafana loads its data sources and the dashboard from files, so it is the same every time.

### Alerts
- RunnerDown, RunnerSilent, RunnerHostFailures, QueueBacklog, QueueStuck, DeadLetters, QueueSampleFailing, APIDown, API5xxRate, InternalErrorVerdicts. Four are unit tested with `promtool` by feeding fake data and checking which alerts fire. There is no notifier: alerts show on the Prometheus Alerts page and the dashboard.

## How it works now, step by step
1. A user submits code. The API counts the request and `submissions_created{language}`.
2. The job lands on the queue; within a minute the sampler shows `queue_waiting` go up.
3. A runner takes it (`queue_pending` up, `runner_jobs_in_flight` up), judges it, and records the time in `judge_duration_seconds`.
4. The verdict goes through Redis to the API, which stores it and counts `verdicts_total{verdict}`.
5. Prometheus scrapes both ports every 15 seconds; Grafana draws the rates, the queue and p50/p95 judge time.
6. If no runner is alive, `up{job="runner"}` goes to 0 and the oldest job's age keeps growing; after a minute `RunnerDown` fires, and after two minutes above 120 s `QueueStuck` fires.

## Key concepts
- **Metric, counter, gauge, histogram:** a number a program publishes. A counter only goes up (verdicts so far); a gauge goes up and down (jobs waiting); a histogram counts how many fell into each time bucket, so you can ask for the 95th percentile.
- **Scrape / pull:** Prometheus asks each program for its numbers on a timer; the programs do not push.
- **Label:** a tag on a metric (verdict="AC"). Every distinct label combination is a separate stored series, so labels must come from small fixed sets.
- **`up`:** a metric Prometheus itself records: 1 if the last scrape worked, 0 if not.
- **Alert rule `for:`:** the condition must stay true that long before the alert fires, to ignore blips.
- **Loki / Alloy:** Loki stores logs; Alloy reads log files and sends them to it.
- **Pending vs. firing:** an alert whose condition just became true is pending; after its `for:` time it fires.

## Try it yourself
```bash
# on your PC, repo root (Docker running; .env has LEETFORCE_GRAFANA_PASSWORD)
docker compose -f observability/docker-compose.yml --env-file .env up -d     # make dev-obs
scripts/obs-tunnel.sh              # terminal 2, leave open
scripts/obs-logs.sh                # terminal 3, leave open
ssh leetforce-dev 'cd ~/Leetforce && scripts/obs-demo.sh 200 0.5 80'   # terminal 4
# open http://127.0.0.1:3001 (admin / the value in .env), Dashboards > LeetForce > Submission flow
# Prometheus alerts: http://127.0.0.1:9090/alerts
make test-alerts                   # promtool tests (Docker)
make test-obs-e2e                  # on the dev host: expects PASS on every line
```
Expect the dashboard to fill within a minute; at 80 seconds the demo stops the runner for 60 seconds, so the queue grows and `RunnerDown` goes pending. To see an alert fire, stop the runner for longer than a minute.

## Trade-offs and risks
- Queue gauges cost 5 Redis commands per sample (about 216k a month at 60 s) on a hosted plan whose limit I have not checked.
- The whole view depends on the tunnel; if it drops, you see `APIDown` and `RunnerDown` even though nothing is wrong.
- One runner per host for now (port 9101).
- Pull metrics will need rethinking if runners ever sit behind NAT; in Kubernetes (Phase 13) it is native.
- The first live-flow test run failed once at a step I could not explain; the second run passed. I changed one helper in between, so it may be a timing flake.
- No notifier: nobody is paged.

## Review questions
Understanding:
1. Why does the dashboard count a stuck queue by "oldest job age" and not only by "number of waiting jobs"?
2. Why are metrics served on a separate localhost port instead of the API's normal port?
3. What would happen to Prometheus if we put the submission id in a label?
4. If the SSH tunnel dies, which alerts fire, and why can't you tell that from a real outage?
5. Why do we compute "waiting" as stream length minus pending instead of reading the consumer group's lag?

Decisions for you:
- A. Do you want a notifier (email or Slack webhook) added for the critical alerts, or is the dashboard enough for now?
- B. The queue is sampled every 60 s to save Redis commands; do you prefer fresher gauges (10 s, about 1.3M commands a month) if your Upstash plan allows it?

## Review Q&A
Partly answered by the owner (in chat, 2026-10-03):
- Decision A (notifier): the owner answered "dashboard is enough for now". No Alertmanager or webhook is added; alerts show on the Prometheus Alerts page and the dashboard. Revisit when the system runs in the cloud (Phase 13).
- Understanding questions 1-5 and decision B (queue sample interval): the owner replied "go" (skip the review). Nothing was answered on the owner's behalf; decision B stays on the default (60 s).

## Open decisions
- B above (queue sample interval; default 60 s). A is settled: no notifier for now.
- Carried over: earlier review answers in `docs/PROGRESS.md`; `web/AGENTS.md` and `web/CLAUDE.md` stay untracked.

## Handoff
- **State:** branch `phase/11-observability`, tags `phase-11-start` and `phase-11-done`, merged into `main`. The dev host's demo has ended; on your PC the tunnel, log copier and the four containers may still be running (stop with `make down-obs` and Ctrl+C).
- **Next phase:** 12 - Infrastructure as code: Terraform (`infra/neon`, `infra/aws`), Packer runner AMI, Ansible hardening, README cost table. Nothing is applied without your confirmation.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-11-summary.md, then start Phase 12 (Infrastructure as code). Ask me the recap question and show me the session plan before writing any code.
  ```

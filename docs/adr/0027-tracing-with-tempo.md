# 0027. Distributed tracing with OpenTelemetry and Tempo

Status: ACCEPTED (owner asked for Tempo after Phase 16)

## Context
Metrics (Prometheus) and logs (Loki) say that something is slow or failing, not where one
submission spent its time across the API, Redis queue and runner (ADR 0019).

## Decision
- **Tempo** joins the observability Compose stack (`observability/tempo/tempo.yml`): single binary,
  local storage, 7-day retention, OTLP/HTTP on 127.0.0.1:4318. Grafana gets a Tempo data source with
  a trace-to-logs link into Loki (service plus `submission_id`).
- **OpenTelemetry** in a new module `telemetry/` (`telemetry.Init`): OTLP/HTTP exporter, W3C
  `traceparent` propagation. Enabled only when `LEETFORCE_OTLP_ENDPOINT` is set; otherwise every
  span is a no-op and nothing is sent. `service.name` is `api` or `runner`, matching the Loki label.
- **Context across the queue:** `queue.Job` carries `Traceparent` (a string, so `Job` stays
  comparable). `Enqueue` opens a `queue.enqueue` producer span and stores the header; the runner opens
  `runner.process` (consumer) from `Job.TraceContext` and a child `judge` span. The API middleware
  (`api/internal/server/trace.go`) names spans by route template and skips `/healthz` and `/readyz`.
- **Transport:** the dev host sends to its own 127.0.0.1:4318, which `scripts/obs-tunnel.sh` reverse-
  forwards (`ssh -R`) to Tempo on the owner's PC. No security-group change and nothing billable.
- Span attributes are ids and bounded values only (submission_id, problem, language, verdict). Source
  code, input and test data are never attributes (Submit must not leak hidden data).

## Alternatives
- Alloy as the collector: one more hop; the SDK exporter is enough for two services.
- Jaeger: no Grafana-native logs/metrics correlation; Tempo fits the existing Grafana stack.
- Tracing inside the sandbox/judge engine: deferred; the `judge` span times the whole judge call.

## Consequences
- New dependencies: `go.opentelemetry.io/otel` (+ sdk, otlptracehttp) in `telemetry`, `queue`, `api`, `runner`.
- Redelivered jobs reuse the original trace, so a reclaimed job shows as a second `runner.process`
  span (attribute `reclaimed=true`) in the same trace.
- Not verified end to end against a running Tempo: only builds, vet and unit tests (see the log).
- k3s: the chart has a commented `LEETFORCE_OTLP_ENDPOINT`; Tempo is not in the cluster.

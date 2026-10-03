# 0024. Load test tool: a stdlib Go program

**Status:** accepted (Phase 16)

Numbering note: renumbered from 0023 to 0024 at integration (Phase 14 took 0023, Phase 15 took 0025).

## Context
Phase 16 needs evidence that the platform holds under concurrent users: sign-up, problem list, Run, Submit, SSE status streaming to a verdict, and contest submissions (Phase 14). The numbers wanted are per-step counts and errors, p50/p95/p99 latency, time-to-verdict and submissions per second. The API enforces per-user and per-IP rate limits (ADR 0017), so a load test will see 429s that are correct behaviour, not failures. The real stack runs only on the dev host or in the cloud, so the tool must run anywhere with just a base URL.

## Decision
- `tools/loadtest/` is one Go program using only the standard library, in its own module `leetforce/tools/loadtest` (ADR 0002: one module per component, listed in `go.work` and `GO_MODULES`, so `make fmt lint test` cover it).
- Each virtual user has its own cookie jar and signs up as `lfload_<runid>_<n>` (email `...@loadtest.invalid`) so accounts can be found and deleted by prefix. It loops: list problems, Run (poll `GET /runs/:id` until `done`), Submit, then read `GET /submissions/:id/events` until the `verdict` event.
- Flags: `-users`, `-iterations`, `-duration`, `-ramp`, `-mode mixed|contest`, `-contest`, `-contest-path`, `-problem`, `-language`, `-source-file`, `-verdict-timeout`, `-json`. Base URL from `LEETFORCE_LOADTEST_BASE_URL` (default `http://localhost:8080`).
- 429 responses (and 503 from the SSE stream-slot cap) are counted per step as `rate_limited`, honour `Retry-After` (capped at 5 s), and are not errors.
- Contest mode posts to `/contests/<slug>/submissions` (configurable path). A 404 aborts the run with a clear message, so the tool degrades cleanly when the contest API is absent. The path is an assumption: the Phase 14 routes were not yet committed when this was written, so check them and pass `-contest-path` if they differ.
- `make loadtest ARGS="..."` runs it. Tests use an `httptest` fake server only.

## Alternatives
- **k6:** better ramping scenarios, thresholds and Grafana output, but needs an install on every host, a JavaScript script to maintain, and its SSE support is an extension. The flows here are a handful of HTTP calls, so a Go file is simpler and is linted and tested like the rest of the code.
- **vegeta / hey / wrk:** good at one-endpoint request rates, but cannot sign up, keep a session, or follow a submission through SSE to a verdict.
- **Locust:** needs Python packages and a separate runtime; same objection as k6.

## Consequences
- No new dependency or install; the tool builds with the Go toolchain already needed.
- Ramping is a simple linear start spread, and load is closed-loop (each user waits for its verdict), so it measures capacity at N concurrent users, not an open arrival rate.
- Test accounts and their submissions stay in the database until cleaned up (SQL in `docs/phases/phase-16-log.md`). Per-user limits (10 submits per minute) cap one virtual user's throughput; high submissions/sec needs many users or raised `LEETFORCE_LIMIT_*` on a test deployment.
- The default solution (`print(0)`) will mostly judge as WA; the tool measures pipeline latency, not correctness. Use `-source-file` and `-problem` for a real accepted solution.

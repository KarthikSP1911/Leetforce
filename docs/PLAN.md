# LeetForce build plan

> **Status: DRAFT written by Claude from CLAUDE.md, awaiting owner review.** Phases are done in order, one per session. Goals and exit criteria for Phases 1-3 are firm; later phases are outlines to be refined at the start of their session.

Milestones: **M1** judge works locally (end of Phase 2), **M2** end-to-end on one machine (Phase 5), **M3** usable product (Phase 9), **M4** cloud deployed (Phase 13), **M5** launch-ready (Phase 16).

## Phase 0 - Foundation (done)

- **Goal:** repo guardrails, layout, web shell.
- **Build:** commit hooks, directory layout, Next.js app with tokens/theme, CLAUDE.md, ADRs.
- **Exit:** lint, typecheck, format, build pass; hooks reject bad commits.

## Phase 1 - Sandbox core

- **Goal:** run untrusted code safely and measure it.
- **Build:** `go.work` + `judge/` module; nsjail wrapper; cgroup v2 limits (CPU, memory, pids) with whole-cgroup kill; result over a dedicated fd/file; `Makefile` (`fmt lint test test-adversarial`); adversarial suite.
- **Exit:** fork bomb, memory bomb, infinite loop, output flood, network access, and file-system escape attempts are all contained; `make test-adversarial` passes.

## Phase 2 - Judge engine (M1)

- **Goal:** judge a solution against a problem locally.
- **Build:** drivers/templates for Python, C++, Java, Go; compile step (CE); checkers; verdicts AC/WA/TLE/MLE/RE/CE/OLE with runtime and memory; `problem.yaml` format; test-set versioning; `judge run problems/<slug> <file>` CLI.
- **Exit:** a sample problem is judged correctly in all four languages for every verdict type; adversarial suite still passes.

## Phase 3 - Queue and runner

- **Goal:** jobs flow through Redis to runners.
- **Build:** `runner/` module; Redis Streams consumer groups on Upstash (`LEETFORCE_REDIS_URL`, supplied by the owner); `XAUTOCLAIM` for crashed runners; runner talks only to Redis and the API; Docker Compose for Redis/MinIO.
- **Exit:** killing a runner mid-job results in the job being reclaimed and judged once; no DB dependency in `runner/go.mod`.

## Phase 4 - API and database

- **Goal:** accept submissions and store verdicts.
- **Build:** `api/` (Gin); Neon Postgres via pgx; migrations; problems and submissions endpoints; idempotent verdict writes keyed by submission ID; test-set version recorded.
- **Exit:** duplicate verdict posts do not change state; `make migrate-up` works from empty.

## Phase 5 - Live status and storage (M2)

- **Goal:** end-to-end on one machine.
- **Build:** SSE status stream; MinIO/S3 for test data; hidden-test redaction for Submit.
- **Exit:** submit via curl, watch Queued → Judging → verdict over SSE; Submit never leaks hidden data.

## Phase 6 - Sandbox hardening

- **Goal:** raise isolation confidence.
- **Build:** evaluate gVisor vs nsjail (ADR, numbers); seccomp tuning; larger adversarial suite.
- **Exit:** decision recorded with measurements; suite passes on the chosen sandbox.

## Phase 7 - Web: problems and workspace

- **Goal:** the LeetCode-style UI on real data.
- **Build:** problem list (search, filters, pagination); split-pane workspace; Monaco; language selector.
- **Exit:** browse and open real problems; keyboard-accessible; both themes.

## Phase 8 - Web: run, submit, results

- **Goal:** full submission loop in the UI.
- **Build:** Run (custom/sample input) and Submit; console tabs; result panel; Submissions tab; SSE client.
- **Exit:** submit from the browser and see the verdict, runtime and memory; failing-case details only for Run.

## Phase 9 - Auth and limits (M3)

- **Goal:** real users and abuse protection.
- **Build:** sign-up/login; sessions; rate limiting per user and per IP; solved status.
- **Exit:** limits enforced and tested; a usable product on one machine.

## Phase 10 - Problem pipeline

- **Goal:** author, validate and rejudge problems.
- **Build:** problem import/validation; reference-solution check; rejudge by test-set version.
- **Exit:** fixing a test set triggers a rejudge of affected submissions.

## Phase 11 - Observability

- **Build:** Prometheus metrics (`leetforce_` prefix), Grafana dashboards, Loki logs, queue-depth and runner-health alerts.
- **Exit:** dashboards show a live submission flow.

## Phase 12 - Infrastructure as code

- **Build:** Terraform `infra/neon` and `infra/aws` (separate state; default VPC only, no custom VPC, endpoints or NAT; `arena.sh up|down|status`, where `down` destroys `infra/aws` only after a typed confirmation); Packer runner AMI; Ansible hardening; README cost table. Nothing is applied without explicit confirmation.
- **Exit:** `terraform plan` clean; AMI builds; destroy of `infra/aws` never touches `infra/neon`.

## Phase 13 - Cloud deployment (M4)

- **Build:** k3s manifests/Helm; runner scaling; secrets via SSM; CI deploy.
- **Exit:** the system runs in the cloud and survives a runner loss.

## Phase 14 - Contests

- **Build:** contest model, timed windows, contest-only problem visibility, scoring.
- **Exit:** a full mock contest runs end to end.

## Phase 15 - Leaderboard

- **Build:** contest and global rankings, caching, penalty rules.
- **Exit:** rankings are correct under concurrent submissions.

## Phase 16 - Launch readiness (M5)

- **Build:** load test, security review, backup/restore drill, docs, cost review.
- **Exit:** load test and review findings resolved or accepted in writing.

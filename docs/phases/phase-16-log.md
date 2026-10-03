# Phase 16 log: Launch readiness

Running log. Secrets, key contents and public IPs are never written here; names only.

## File and path index
| Path | What |
|---|---|
| `README.md` | Rewritten: description, architecture, quickstart, commands table, cost table with UNVERIFIED figures |
| `docs/cost-review.md` | New: every recurring cost, running vs code-only, monthly estimate, shutdown list |
| `docs/FLOW.md` | Full pass against the code; corrections listed below |

## Docs and cost review

Who: Claude (docs subagent). Branch `feat/16-docs-cost` off `phase/16-launch-readiness`. Nothing billable was run and AWS was not queried.

### Steps and results
1. `git checkout -b feat/16-docs-cost`: ok. Read `CLAUDE.md`, `README.md`, `docs/FLOW.md`, `docs/PROGRESS.md`, `.env.example`, `Makefile`, `docker-compose.yml`, ADRs 0003, 0019, 0020, 0021, 0022, `infra/aws/*.tf`, `infra/bootstrap`, `infra/neon`, `packer/runner.pkr.hcl`, phase 12 and 13 logs and reports.
2. Cost facts found in the repo: instance types (`t3.small` control and runner, `t3.micro` dev), 15 GiB gp3 volumes (`root_volume_gib`), a public IP on every host (no NAT), S3 about $0.025 per GB-month (ADR 0021, the only dollar figure in the repo), 216k Upstash commands a month at the 60 s sampler (ADR 0019), Neon free-plan assumption (`infra/neon/variables.tf`, 6 h restore window). No hourly EC2, EBS or IPv4 price exists in the repo; the README and `docs/cost-review.md` mark every such figure UNVERIFIED and state the basis (my recollection of ap-south-1 list prices).
3. FLOW.md verification, each claim checked against code with `grep` and `ls`. Confirmed: reaper 15 min and 2 min grace (`api/internal/reaper/reaper.go`), SSE 500 ms poll, 15 s keep-alive, 10 min limit, 200 streams (`api/internal/server/events.go`), per-minute limits 10/30/20/60 and auth limits 20/10 (`api/cmd/api/main.go`), metrics ports 9102 and 9101, run TTL 10 min (`queue/run.go`), `MinIdle` 30 s, `MaxDeliveries` 3, verdict TTL 7 days, status stream cap 5000 (`queue/`), session cookie `lf_session` with 30-day TTL, enqueue failure deletes the row and returns 503, ten alert names in `observability/prometheus/alerts.yml`, `ci.tf` trusts `environment:production`, IMDS hop limit 2 on the control host and 1 on runners, 34 resources to add in the `infra/aws` plan (phase-13 log), five migrations. All function and file names cited in sections 1 to 13 were grepped (for example `RecordVerdict`, `ReapUnqueued`, `BeginRejudge`, `SyncProblem`, `processRun`, `RunCustom`, `limitUserAndIP`, `SolvedProblems`, `newCgroupJob`, `parseLog`, `seccompPolicy`); none were missing.
4. FLOW.md corrections made (discrepancies):
   - Intro said Phases 0 to 8 are built and 9 to 16 planned; now 0 to 13 built, 14 to 16 planned.
   - Phase 12 and 13 table rows were out of date (12 "AMI pending", 13 "in progress"); aligned with PROGRESS ("done (partial)", M4 not tagged).
   - Note under the table said Phases 6 to 16 are only a reading of the plan; now only 14 to 16.
   - Phase 3: heartbeat was "every MinIdle/3"; the code uses `HeartbeatEvery`, default 10 s (`runner/internal/agent/agent.go`), which is MinIdle/3 only at the defaults.
   - Phase 8: said unit 1 (run endpoint) was unmerged and the browser check pending; the phase-8 log shows it merged, migration 00004 applied and the Chrome check done.
   - Phase 10: referred to `bin/rejudge`; no Makefile target builds it, the real command is `go run ./cmd/rejudge` from `api/`.
   - Phase 12: Neon state was "infra/neon/terraform.tfstate" (local); since Phase 13 it is `tfstate/neon.tfstate` in S3. Runner hosts are now an Auto Scaling group; port 80 was added in Phase 13; the exit-check sentence said `terraform plan` was still pending although it ran in Phase 13.
   - Phases 14, 15, 16 had no section 3 entries; placeholders added and not ticked.
5. Other discrepancies noticed outside the files I edited (not changed, reported): `CLAUDE.md` says `make dev` starts "local Redis and S3 (RustFS...)", but since Phase 13 it starts only Redis (RustFS is commented out in `docker-compose.yml`). `CLAUDE.md` says "Planned, not defined yet: make scan", fine. `docs/PROGRESS.md` has no row for Phases 14 to 16 (not my task).
6. Logo: `web/public/brand/logo-mark.svg` is referenced as-is in the README header; no file under `web/public/brand` was touched.
7. Commit: `docs(docs): ...` with `Refs: phase-16` footer, on `feat/16-docs-cost`; not merged.

### UNVERIFIED items for the owner
- All dollar figures (EC2, EBS, IPv4, snapshot): list-price assumptions for ap-south-1.
- Neon plan and limits; Upstash plan and monthly command limit; GHCR and GitHub Actions allowances.
- That `leetforce-dev` is still the only running instance and that no builder, snapshot, Elastic IP or volume is left over (AWS was not queried).
- Whether the Neon password pasted into chat in Phase 13 was rotated.

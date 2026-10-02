# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository.

## Current state

The repo currently contains only `README.md`. Everything below describes the planned system and conventions; the commands and directories do not exist yet. `docs/PLAN.md` (the 0–16 phase plan) and `docs/PROGRESS.md` (phase tracker and resume point) are the sources of truth once they are created. Do not invent build or test commands that no Makefile defines yet.

## Project

**LeetForce** is a LeetCode-style distributed code execution platform. Users submit Python, C++, Java, or Go; a fleet of runners judges it in isolated sandboxes and returns a verdict (AC, WA, TLE, MLE, RE, CE, OLE) with runtime and memory.

Submission flow: browser → API (Gin, SSE for live status) → Redis Streams (consumer groups, `XAUTOCLAIM` for crashed runners) → runner → sandbox (nsjail; gVisor evaluated in Phase 6) → verdict → API → browser.

Planned layout: `judge/` (engine, drivers, checkers, adversarial suite), `runner/` (pulls jobs, judges, reports to API), `api/`, `web/` (Next.js + Monaco), `infra/` (Terraform: `infra/neon`, `infra/aws`), `packer/`, `ansible/`, `k8s/`, `problems/` (`problem.yaml` + tests), `docs/` (`PLAN.md`, `PROGRESS.md`, `adr/`, `phases/`).

Stack: Go for judge/runner/API; Neon Postgres via pgx; MinIO locally and S3 in the cloud; Docker Compose, k3s, Prometheus/Grafana/Loki.

Naming: `LeetForce` in UI copy, docs and titles; lowercase `leetforce` in Go module paths, image/db/k8s/Terraform names, and metric prefixes; `LEETFORCE_` prefix for project-specific env vars.

## Planned commands

```bash
make dev | down | fmt | lint | test | test-adversarial | migrate-up
judge run problems/<slug> <solution-file>   # local judge CLI, from Phase 2
```

Add real targets here as they are created, including how to run a single test.

## Session model: one phase per session

- Phases are done strictly in order, one per session. State lives in the repo, not chat history.
- **Start of session:** read this file, `docs/PROGRESS.md`, and the previous `docs/phases/phase-<N-1>-summary.md`. Verify `git status`, branch, `git log --oneline -5`, and `git tag --list 'phase-*'` match `PROGRESS.md`; if not, stop and tell the user. Ask one short recap question (skip for Phase 0), then post a session plan (phase, units of work with branch names, decisions needed, what the user can run at the end) and wait for go-ahead.
- A phase is done only when its exit criteria are demonstrably met, `docs/phases/phase-<N>.md` (report) and `phase-<N>-summary.md` (plain-language teaching doc) are written, and the in-chat review Q&A with the user has happened. Do not merge to `main` before the user answers or says to skip. Do not start the next phase in the same session.
- Write at least one ADR per phase in `docs/adr/NNNN-short-title.md`. When a decision has real trade-offs (e.g. nsjail vs gVisor, ARM vs x86), stop and ask.
- Generate the report's file list from `git diff --name-status phase-<N>-start..HEAD`, never from memory.
- If a phase will not fit, stop at a clean boundary, set `PROGRESS.md` to `in progress` with a resume point, and commit as `docs(phase-<N>): record session progress`.

## Git workflow

- Branches: `main` ← `phase/<N>-<slug>` ← `feat|fix|test/<N>-<slug>`. Tag `phase-<N>-start` at phase start and `phase-<N>-done` at merge (plus `M<k>` for milestones).
- Never commit directly to `main` (the initial README commit was the one bootstrap exception). Never force-push `main` or `phase/*`.
- Merge units into the phase branch with `--no-ff` and message `merge: <description> (phase <N>)`; delete the unit branch. Keep phase branches after merging to `main`.
- Conventional Commits with scope (`sandbox`, `judge`, `runner`, `api`, `web`, `brand`, `db`, `queue`, `infra`, `packer`, `ansible`, `k8s`, `ci`, `obs`, `contest`, `leaderboard`, `docs`) and a `Refs: phase-<N>` footer. Commit at every meaningful step (~30–150 lines) that builds and passes tests.
- Before committing: `make fmt lint`, tests for the touched area, review `git diff --staged` for secrets/binaries, and stage specific paths.
- Push branches and tags after each merge only if a remote is configured (it is: `origin` → `KarthikSP1911/Leetforce`).

## Security rules (non-negotiable)

- Untrusted code never runs outside the sandbox, including in tests.
- The harness reports results over a dedicated fd or file, never by parsing user stdout, which is data only.
- Never return hidden test inputs, expected outputs, or raw stderr for Submit; only Run (custom input or samples) may show them.
- Kill the whole cgroup on timeout or limit breach.
- Runners never connect to the database; they talk only to Redis and the API.
- Any sandbox change must pass the full adversarial suite (`make test-adversarial`) before merging.

## Data model rules

- Submissions record the test-set version they were judged against so problems can be rejudged.
- Verdict writes are idempotent, keyed by submission ID.
- Rate limit per user as well as per IP.

## Cloud and cost rules

- Never run `terraform apply`, `terraform destroy`, `arena.sh up`, or anything billable without explicit confirmation in chat.
- `terraform destroy` on `infra/aws` must never touch `infra/neon`.
- Record any new recurring cost (VPC endpoints, public IPv4, EBS) in the README cost table.
- Config via env vars; `DATABASE_URL` etc. come from git-ignored `.env` locally (with committed `.env.example`) and SSM in the cloud.

## Go and frontend standards

- Go: `gofmt`, `go vet`, `golangci-lint`; wrap errors with context (`fmt.Errorf("compile cpp: %w", err)`); no panics outside `main`; pass `context.Context` through I/O; table-driven tests. Module layout (per-component modules vs `go.work`) is decided in Phase 0 and recorded in an ADR.
- Frontend: TypeScript strict, ESLint + Prettier, no secrets or internal URLs in bundles, visible focus ring, keyboard shortcuts for Run and Submit.
- Every bug fix ships with a test that would have caught it.

## Brand and UI

The IA follows LeetCode (problem list, split-pane workspace, console, verdict panel) but must not copy its logo, icons, copy, colors, or problem statements; problems must be original or licensed.

- **Logo:** exactly one file, `web/public/brand/logo.svg`, used as-is everywhere (navbar, favicon, loading screen, README, OG image). Never redraw, recolor, crop, trace, or create variants, and do not edit its contents or metadata. Show it as a rounded-square tile (`border-radius: 8px`) in light and dark mode, with "LeetForce" as plain text beside it (Inter 700). If the file is missing, ask the user; do not substitute anything.
- **Color tokens** (CSS variables only, never raw hex in components): `--lf-blue-600 #0050FF` primary; `--lf-sky-400 #00B4FF` accent/focus ring/"Judging"; `--lf-navy-900 #071A3D` text and dark bg; `--lf-navy-800 #0D2247` dark panels; `--lf-navy-700 #163463` dark borders; `--lf-surface #F4F7FC`; `--lf-border #DCE4F2`; `--lf-muted #5B6B86`; `--lf-success #16A34A` (AC, Easy); `--lf-warning #F59E0B` (TLE/MLE/OLE, Medium); `--lf-danger #E5484D` (WA/RE/CE, Hard). The logo's orange and red are not UI colors.
- White text on sky fails contrast; use navy text on sky. Never show a verdict by color alone; always include the label.
- **Fonts:** Inter for UI; JetBrains Mono for code, editor, console, and test I/O.
- **Layout:** top nav (logo, Problems, Contest, Leaderboard, theme toggle, user menu); resizable split-pane workspace (left: Description/Submissions tabs; right: language selector + Monaco, Run secondary and Submit primary blue; bottom: console with Testcase/Result tabs). Result panel shows the verdict largest, then runtime and memory, with failing-case details only for Run. Light/dark follow the system by default.

## Working style

If an explicit user instruction conflicts with this file, follow the user and suggest updating this file. Explain non-obvious choices in one line as you go so the end-of-phase review has no surprises.

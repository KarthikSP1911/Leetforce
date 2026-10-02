# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository.

## Current state

Phase 0 (web shell) is done; Phase 1 (sandbox core) is in progress, see `docs/PROGRESS.md`. Parts below describe the planned system and may not exist yet. `docs/PLAN.md` (the 0–16 phase plan) and `docs/PROGRESS.md` (phase tracker and resume point) are the sources of truth. Do not invent build or test commands that no Makefile defines.

## Project

**LeetForce** is a LeetCode-style distributed code execution platform. Users submit Python, C++, Java, or Go; a fleet of runners judges it in isolated sandboxes and returns a verdict (AC, WA, TLE, MLE, RE, CE, OLE) with runtime and memory.

Submission flow: browser → API (Gin, SSE for live status) → Redis Streams (consumer groups, `XAUTOCLAIM` for crashed runners) → runner → sandbox (nsjail; gVisor evaluated in Phase 6) → verdict → API → browser.

Planned layout: `judge/` (engine, drivers, checkers, adversarial suite), `runner/` (pulls jobs, judges, reports to API), `api/`, `web/` (Next.js + Monaco), `infra/` (Terraform: `infra/neon`, `infra/aws`), `packer/`, `ansible/`, `k8s/`, `problems/` (`problem.yaml` + tests), `docs/` (`PLAN.md`, `PROGRESS.md`, `adr/`, `phases/`).

Redis is hosted on Upstash; the connection string comes from `LEETFORCE_REDIS_URL` (a `rediss://` TLS URL), provided by the owner, never committed, and listed without a value in `.env.example`. Stack: Go for judge/runner/API; Neon Postgres via pgx; MinIO locally and S3 in the cloud; Docker Compose, k3s, Prometheus/Grafana/Loki.

Naming: `LeetForce` in UI copy, docs and titles; lowercase `leetforce` in Go module paths, image/db/k8s/Terraform names, and metric prefixes; `LEETFORCE_` prefix for project-specific env vars.

## Commands

Go targets run per module in `GO_MODULES` (Makefile) on a Linux host; Phase 1 development happens on the EC2 dev host (ADR 0003, `scripts/setup-dev-host.sh`).

```bash
make fmt                 # golangci-lint fmt (gofmt + goimports) per Go module
make lint                # go vet + golangci-lint run per Go module
make test                # go test per Go module
make test-sandbox        # functional sandbox tests: real programs in nsjail (sudo -n)
make test-adversarial    # sandbox containment suite (build tag `adversarial`), run as root in a memory-capped systemd scope

# single test
cd judge && go test -run TestName ./sandbox/...
make test-adversarial RUN=TestAdversarialForkBomb   # one adversarial test (RUN is a go test -run regex)
```

Planned, not defined yet: `make dev | down | migrate-up`, and `judge run problems/<slug> <solution-file>` (local judge CLI, from Phase 2).

## Session model: one phase per session

- Phases are done strictly in order, one per session. State lives in the repo, not chat history.
- **Start of session:** read this file, `docs/PROGRESS.md`, and the previous `docs/phases/phase-<N-1>-summary.md`. Verify `git status`, branch, `git log --oneline -5`, and `git tag --list 'phase-*'` match `PROGRESS.md`; if not, stop and tell the user. Ask one short recap question (skip for Phase 0), then post a session plan (phase, units of work with branch names, decisions needed, what the user can run at the end) and wait for go-ahead.
- A phase is done only when its exit criteria are demonstrably met, `docs/phases/phase-<N>.md` (report) and `phase-<N>-summary.md` (plain-language teaching doc) are written, and the in-chat review Q&A with the user has happened. Do not merge to `main` before the user answers or says to skip. Do not start the next phase in the same session.
- Write at least one ADR per phase in `docs/adr/NNNN-short-title.md`. When a decision has real trade-offs (e.g. nsjail vs gVisor, ARM vs x86), stop and ask.
- Generate the report's file list from `git diff --name-status phase-<N>-start..HEAD`, never from memory.
- If a phase will not fit, stop at a clean boundary, set `PROGRESS.md` to `in progress` with a resume point, and commit as `docs(docs): record phase <N> session progress` with a `Refs: phase-<N>` footer.

## Git workflow

- Commit scopes are modules, so phase documents use `docs(docs)` and the phase goes in the `Refs: phase-<N>` footer. Merges use git's default `Merge ...` message (industry standard; commitlint skips it). Do not use a custom `merge:` type.

- Branches: `main` ← `phase/<N>-<slug>` ← `feat|fix|test/<N>-<slug>`. Tag `phase-<N>-start` at phase start and `phase-<N>-done` at merge (plus `M<k>` for milestones).
- Never commit directly to `main` (the initial README commit was the one bootstrap exception). Never force-push `main` or `phase/*`.
- Merge units into the phase branch with `--no-ff` and git's default message (`Merge branch 'feat/<N>-<slug>' into phase/<N>-<slug>`), which commitlint ignores by design; delete the unit branch. Keep phase branches after merging to `main`.
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
- No custom VPC, VPC endpoints, or NAT gateway: AWS resources use the account's default VPC with locked-down security groups. Record any new recurring cost (public IPv4, EBS) in the README cost table.
- Config via env vars; `DATABASE_URL` etc. come from git-ignored `.env` locally (with committed `.env.example`) and SSM in the cloud.

## Go and frontend standards

- Go: `gofmt`, `go vet`, `golangci-lint`; wrap errors with context (`fmt.Errorf("compile cpp: %w", err)`); no panics outside `main`; pass `context.Context` through I/O; table-driven tests. Module layout (per-component modules vs `go.work`) is decided in Phase 0 and recorded in an ADR.
- Frontend: TypeScript strict, ESLint + Prettier, no secrets or internal URLs in bundles, visible focus ring, keyboard shortcuts for Run and Submit.
- Every bug fix ships with a test that would have caught it.

## Brand and UI

The IA follows LeetCode (problem list, split-pane workspace, console, verdict panel) but must not copy its logo, icons, copy, colors, or problem statements; problems must be original or licensed.

- **Logo:** three files, all used as-is (`logo-mark-light.svg` is `logo-mark.svg` with only the centre bar fill changed to dark grey (`--lf-ink-700`); the navbar shows it in light theme and `logo-mark.svg` in dark theme): `web/public/brand/logo-mark.svg` (no background; used in the navbar and as the favicon, drawn directly on the page with no tile) and `web/public/brand/logo.svg` (black background, used for README and OG image, shown as a rounded tile). Never redraw, recolor, crop, trace, or create further variants, and do not edit its contents or metadata. Show it as a rounded-square tile (`border-radius: 8px`) in light and dark mode, with "LeetForce" as plain text beside it (system UI font, bold). If the file is missing, ask the user; do not substitute anything.
- **Color tokens** (CSS variables only, never raw hex in components): `--lf-blue-600 #0050FF` primary; `--lf-sky-400 #00B4FF` accent/focus ring/"Judging"; `--lf-navy-900 #071A3D` text and dark bg; `--lf-navy-800 #0D2247` dark panels; `--lf-navy-700 #163463` dark borders; `--lf-surface #F4F7FC`; `--lf-border #DCE4F2`; `--lf-muted #5B6B86`; `--lf-success #16A34A` (AC, Easy); `--lf-warning #F59E0B` (TLE/MLE/OLE, Medium); `--lf-danger #E5484D` (WA/RE/CE, Hard). The logo's orange and red are not UI colors.
- White text on sky fails contrast; use navy text on sky. Never show a verdict by color alone; always include the label.
- **Fonts:** LeetCode's system UI stack for UI (no web fonts); `Menlo, Monaco, Consolas, Courier New` for code, editor, console, and test I/O.
- **Layout:** top nav (logo, Problems, Contest, Leaderboard, theme toggle, user menu); resizable split-pane workspace (left: Description/Submissions tabs; right: language selector + Monaco, Run secondary and Submit primary blue; bottom: console with Testcase/Result tabs). Result panel shows the verdict largest, then runtime and memory, with failing-case details only for Run. Light/dark follow the system by default.

### Professional design standard (applies to every UI change)

Target the polish of the official LeetCode site: dense, calm, utilitarian, no decoration for its own sake. Colors come **only** from the LeetForce tokens above.

- **Palette discipline:** no raw hex, no Tailwind default palette colors (`bg-blue-500`, `text-gray-600`, ...) in components. Use the theme aliases defined in `web/src/app/globals.css` (`bg-panel`, `border-panel-border`, `text-muted`, `bg-primary`, `text-success|warning|danger`, `bg-hover`). Hex values exist only in the token block of `globals.css`. White is the one non-token neutral and is exposed as `--lf-white`.
- **Surfaces:** page = `--background`, cards/tables/panels = `--panel` with a 1px `--panel-border`, radius 8px (`rounded-lg`), no heavy shadows or gradients. Hover state = `--hover`. Dark mode surfaces are neutral black/grey (`--lf-ink-950/900/800/700`: page, panel, hover, border), like LeetCode's dark theme; navy tokens are for text and brand accents, not dark backgrounds.
- **Type:** 14px base, LeetCode's system font stack (`-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica Neue, Arial`); page titles 24px bold; table headers 12px uppercase muted; numbers and code in `Menlo, Monaco, Consolas, Courier New`.
- **Difficulty and verdict color:** Easy = success, Medium = warning, Hard = danger (text color, semibold). Verdicts always carry their text label.
- **Primary actions** (Submit, Sign in) use `bg-primary` with white text; secondary actions (Run) are bordered panels. Sky is for focus rings and "Judging" only, with navy text if used as a background.
- **Layout:** content max width 1152px (`max-w-6xl`), 56px sticky top nav, 16px page gutters, tables that collapse secondary columns on small screens.
- **Theme:** follows the system by default; the nav toggle sets `data-theme` on `<html>` and persists to `localStorage` (`lf-theme`). Every new component must be checked in both themes.
- **Accessibility:** visible sky focus ring, `aria-label` on icon-only buttons, AA contrast.
- Before finishing UI work: `npm run lint`, `npm run typecheck`, `npm run build` in `web/`, and look at the page in light and dark mode.

## Documenting every step (mandatory)

Every action taken in a session must be written down in the repo docs in detail, including actions outside the repo (EC2 instance, AWS console, SSH or Windows setup, installed packages, system changes). Do not rely on chat history.

- Keep a running log in `docs/phases/phase-<N>-log.md` (environment setup, per-unit entries, mistakes), created at the start of the phase and updated during each unit of work, not only at the end. It is kept after the phase; the report `phase-<N>.md` links to it.
- For each step record: who did it (owner or Claude), the exact command or console action, the result or verified fact, and anything that failed and how it was fixed.
- Record host state changes (packages, services, swap, config files, users, firewall/security-group rules) and the resulting state, so the host can be rebuilt or audited later.
- Never write secrets, key contents, or public IPs in docs. Name the file or variable instead.
- Anything repeatable goes into a script under `scripts/` as well, and the log points to it.
- The phase report and phase summary are built from this log; a step missing from the log is treated as not done.
- Every phase also updates `docs/FLOW.md`: tick the phase in the per-phase table, add its detailed "as built" flow (numbered steps with file paths), and fix the planned rows if the plan changed. The phase summary's flow diagram must match it.

### Update the docs on your own, in the same response (mandatory)

The owner must never have to ask for documentation. Before ending any response that changed a file, ran a command on a host, or settled a decision, update the docs in that same response:

1. Add or extend the entry in `docs/phases/phase-<N>-log.md` with exact paths, commands, results, mistakes and corrections, and add new paths to its file and path index.
2. Update `docs/FLOW.md` if the flow of any phase changed.
3. Update `docs/PROGRESS.md` (status and resume point) whenever the state changed.
4. If the phase is in review or done, also refresh the phase report (file list regenerated from `git diff --name-status`, branches, stats) and the phase summary.
5. Check every number and name you write against git, the code or the host; fix anything that does not match.
6. Commit the docs with the work (separate `docs(docs)` commit), then say in the reply which docs were updated and where.

Answers given in chat are not review answers unless the owner wrote them; record who answered what.

## Working style

If an explicit user instruction conflicts with this file, follow the user and suggest updating this file. Explain non-obvious choices in one line as you go so the end-of-phase review has no surprises.

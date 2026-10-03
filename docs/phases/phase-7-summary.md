# Phase 7 summary: Web: problems and workspace

## TL;DR
- The browser now shows **real problems** from the API: a searchable, filterable, paged list, and a workspace with the statement, a code editor and sample cases.
- The API serves each problem's statement, starter code per language and an acceptance rate; four new original problems exist (plus the Phase 2 sample), and every reference solution was judged in the real sandbox.
- It is usable with only the keyboard and in light and dark mode. Run and Submit are visible but switched off until Phase 8.

## Where this phase fits
```
 browser --> API --> Postgres/Redis --> queue --> runner --> sandbox --> verdict --> API --> browser
   [P7 NEW: read problems]  [built P4]  [built P3]  [built P3]  [built P1,2,6]   [built P4,5]  [P8: show it]
   problem list + workspace              the submit path works with curl; the UI cannot call it yet
```
- Phase 7 builds only the *reading* side of the browser: it depends on the API (Phase 4) and the problem files (Phase 2).
- It unblocks Phase 8, which adds Run, Submit and the result panel on top of this workspace.

## What I built and why
### API problem fields (`feat/7-api-problem-fields`)
- **What:** `GET /problems` accepts search, difficulty, tag and page parameters; `GET /problems/:slug` returns the statement, starter code and sample cases. Four original problems were added.
- **Why:** the workspace has nothing to show without a statement and starter code; the list needs filtering and acceptance.
- **How it works:** `statement.md` and `starters/<lang>.<ext>` sit beside `problem.yaml` and are loaded by `judge/problem/problem.go`; `api/internal/store/problems.go` computes acceptance from stored verdicts.
- **Alternatives considered:** storing statements in Postgres (needs a migration and sync) and caching acceptance. See [ADR 0015](../adr/0015-catalog-content-and-workspace-delivery.md).

### API client (`feat/7-web-api-client`)
- **What:** one typed fetch helper for the web app.
- **Why:** server pages and browser code need the same calls without exposing the API's internal address.
- **How it works:** on the server it calls `LEETFORCE_API_URL`; in the browser it calls `/api/...`, which `web/next.config.ts` rewrites to the API.

### Problem list (`feat/7-problem-list`)
- **What:** `/problems` with a table (solved status, title, tags, acceptance, difficulty), a filter form and pagination.
- **Why:** this is how users find a problem.
- **How it works:** the filters are a plain GET form, so the URL holds the state and the page is a server component (`web/src/app/problems/page.tsx`).

### Workspace (`feat/7-workspace`)
- **What:** `/problems/<slug>` with a resizable split pane: Description and Submissions tabs on the left, language selector and Monaco on the right, a console with sample cases below.
- **Why:** this is the screen where Phase 8 will put Run and Submit.
- **How it works:** `Workspace.tsx` holds the state; `SplitPane.tsx` is a focusable divider that resizes with the mouse or arrow keys; `Tabs.tsx` implements ARIA tabs; `CodeEditor.tsx` wraps Monaco.

### Accessibility check (`test/7-web-a11y`, `fix/7-dark-link-contrast`)
- **What:** a real-browser check in both themes, and a fix.
- **Why:** the exit criterion is keyboard-accessible and both themes. Measuring found the active tab at 2.98:1 contrast in dark mode (needs 4.5:1).
- **How it works:** a `--link` colour role (blue in light, sky in dark) in `web/src/app/globals.css`; now 7.44:1 in dark.

### Sandbox check of the new problems (`test/7-judge-new-problems`)
- **What:** all 16 reference solutions judged in the real sandbox on the dev host; lint, tests, the acceptance SQL and a Trivy scan.
- **Why:** the first run of these solutions was outside the sandbox, which is not a real test of the judge.

## How it works now, step by step
1. You open `/problems?difficulty=easy`. Next.js (on the server) calls `GET /problems?difficulty=easy` on the API.
2. The API filters the catalog (read from `problems/` at startup), looks up acceptance in Postgres, and returns one page.
3. The table renders; clicking a title opens `/problems/fizz-count`.
4. That page calls `GET /problems/fizz-count` and renders the statement (Markdown), the starter for the selected language, and sample cases.
5. Switching the language swaps the starter in Monaco. Run and Submit stay disabled.

## Key concepts
- **Server component:** a React component that runs on the server and sends finished HTML; LeetForce uses it so data loads before the page shows and no API address reaches the browser.
- **Rewrite / proxy:** `/api/*` in the browser is forwarded to the Go API, so the browser only ever talks to the web app.
- **Acceptance rate:** accepted submissions divided by judged submissions (internal errors excluded); computed on request so it cannot drift.
- **Test-set version:** a fingerprint of a problem's tests, recorded with each submission; statements and starters are not part of it, so editing them never forces a rejudge.
- **ARIA roles (`separator`, `tab`):** labels that tell screen readers and keyboards what a custom widget is; we use them because the split pane and tabs are not native controls.
- **WCAG AA contrast:** at least 4.5:1 between text and its background for normal text.

## Try it yourself
```bash
# terminal 1 (repo root; .env supplies DATABASE_URL and LEETFORCE_REDIS_URL)
set -a; . ./.env; set +a
LEETFORCE_S3_ENDPOINT= LEETFORCE_PROBLEMS_DIR="$PWD/problems" go -C api run ./cmd/api
# terminal 2
cd web && npm run dev
```
Open http://localhost:3000/problems. Expect 5 problems, filters, and Acceptance showing 37.0% for Sample Sum and a dash for the rest. Open Fizz Count; press Tab to move through the page, and arrow keys on the divider to resize. Toggle the theme from the navbar.

## Trade-offs and risks
- Statements live in files, not the database: simple and versioned with the tests, but changing one needs an API restart. Revisit in Phase 10.
- Monaco loads from a public CDN: no build work, but it needs internet. Revisit at deployment (Phase 12).
- Light-mode difficulty text is 3.30:1, under AA, because the brand token is fixed. Owner decision pending.
- The narrow-screen layout and keyboard use of the filter form were not checked in a browser.

## Review questions
Asked in chat at the end of the phase (see below). Answers are recorded after the review.

## Review Q&A
Pending.

## Open decisions
- Light-mode difficulty text contrast (3.30:1): keep the token, or add a darker light-mode text shade (Phase 8 or earlier).
- Monaco from the CDN or bundled (Phase 12 at the latest).
- Carried over: Phase 6 ADR 0013 and 0014 confirmations; Phase 4 to 6 review answers.

## Handoff
- **State:** branch `phase/7-web-workspace`, tag `phase-7-start`; nothing running. The EC2 dev host `leetforce-dev` is still up and billable.
- **Next phase:** 8 - Web: run, submit, results: Run (custom and sample input) and Submit, console tabs, result panel, Submissions tab, SSE client. Decide beforehand how Run should work, since the API has no Run endpoint yet.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-7-summary.md, then start Phase 8 (Web: run, submit, results). Ask me the recap question and show me the session plan before writing any code.
  ```

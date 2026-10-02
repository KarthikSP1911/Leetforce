# Phase 0 summary: Foundation

## TL;DR
- The repo now has guardrails (commit-message checks, auto-formatting, a fixed folder layout) so every later phase stays consistent.
- A Next.js web app exists with a LeetCode-style shell: top navbar, light/dark theme, and a problem table filled with sample data.
- Nothing runs user code yet. No API, queue, runner, or sandbox exists; those start in Phase 1.

## Where this phase fits
```
browser --> API --> queue --> runner --> sandbox --> verdict --> browser
[shell BUILT]  [todo]  [todo]   [todo]    [todo]      [todo]     [shell BUILT]
```
Built this phase: only the browser-side shell (`web/`) and repo tooling. Everything else is still to come.

- Phase 0 depends on nothing; it is the starting point.
- It unblocks Phase 1 (the sandbox core) by fixing the layout, commit rules, and branch workflow.
- The UI shell lets later phases drop real data into an already-styled page.

## What I built and why

### Repo guardrails (`chore(ci)` commits)
- **What:** `.editorconfig`, `.gitattributes`, `.gitignore`, husky hooks, commitlint, lint-staged.
- **Why:** without them, formatting and commit messages drift, and secrets like `.env` can be committed by accident.
- **How it works:** on `git commit`, husky runs lint-staged (formats and lints staged files in `web/`), then commitlint checks the message against the allowed types and scopes in `commitlint.config.mjs`.
- **Alternatives considered:** CI-only checks (catch problems too late, after the commit exists).

### Project guidance (`CLAUDE.md`)
- **What:** the written rules for workflow, security, brand, and design.
- **Why:** each session starts fresh, so the rules must live in the repo, not in chat.
- **How it works:** read at the start of every session; updated when conventions change.

### Web app scaffold (`feat(web)`)
- **What:** Next.js + TypeScript (strict) + Tailwind, with ESLint and Prettier.
- **Why:** the frontend is a separate component and needs a working toolchain before features.
- **How it works:** `web/src/app` holds routes, `web/src/components` holds UI, `web/src/lib` holds helpers.

### UI shell (`feat/0-ui-shell`)
- **What:** navbar, theme toggle, problem table, black/grey dark theme, system fonts, logo variants.
- **Why:** it fixes the look early (tokens, fonts, logo behavior) so later pages inherit it.
- **How it works:** colors are variables in `globals.css`; the toggle sets `data-theme` on `<html>`; two logo files swap by theme. See [ADR 0001](../adr/0001-web-design-system.md).
- **Alternatives considered:** navy dark theme, Inter/JetBrains Mono fonts (both rejected; see the ADR).

## How it works now, step by step
1. You open `/`; the app redirects to `/problems`.
2. The layout runs a small script that reads your saved theme from `localStorage` and sets `data-theme` before the page paints.
3. The navbar renders both logo images; CSS hides one depending on the active theme.
4. The problems page renders `ProblemTable` from the hard-coded rows in `web/src/lib/sample-problems.ts`.
5. Clicking the toggle flips `data-theme`, and every color changes because components only reference variables.

## Key concepts
- **Design token:** a named variable (like `--lf-blue-600`) used instead of a raw color, so one edit restyles everything.
- **Conventional Commits:** a message format (`type(scope): summary`) that tools can check automatically.
- **Git hook:** a script git runs at set moments (before a commit, on a message) to block bad input.
- **lint-staged:** runs formatters only on files you are committing, so hooks stay fast.
- **Monorepo:** one repository holding several components (judge, runner, api, web).

## Try it yourself
```bash
cd web
npm run dev          # open http://localhost:3000, click the sun/moon icon
npm run lint && npm run typecheck && npm run build
git commit --allow-empty -m "bad message"   # rejected by commitlint
```
Expect: the problem table in light and dark themes, a logo middle bar that turns dark grey in light mode, all checks passing, and the bad commit refused.

## Trade-offs and risks
- Hard-coded sample problems are fake data; they must be replaced when the API exists.
- Fonts differ per operating system because we use system fonts.
- The favicon cannot change with the theme.
- No automated check forbids raw hex in components yet; it relies on review.
- `docs/PLAN.md` is missing, so exit criteria were chosen by me, not taken from a plan.

## Review questions
1. Why do components use token variables like `--panel` instead of writing colors directly?
2. What would happen if the theme script in `layout.tsx` ran after the page painted?
3. Why are there two logo files for the navbar instead of recoloring one with CSS?
4. What does the commitlint hook protect us from, and why did it reject the `merge:` message?
5. Why does CLAUDE.md say runners never talk to the database, even though we have not built a runner yet?

Decision questions:
- A. Go layout: one module per component (`judge/`, `runner/`, `api/`) or a single `go.work` workspace?
- B. Should `docs/PLAN.md` be written by you, or should I draft it from CLAUDE.md for your approval?
- C. Commit style: change commitlint to allow `merge:`, or change CLAUDE.md to use `chore(...)` for merges?

## Review Q&A
_To be filled in after the review._

## Open decisions
- Go module layout (needed before Phase 1).
- Source of `docs/PLAN.md` (needed before Phase 1).
- Merge commit message format.

## Handoff
- **State:** branch `phase/0-foundation`, tag `phase-0-start`; `main` already contains the code. `phase-0-done` is not set until the review finishes. Nothing is running.
- **Next phase:** 1 - Sandbox core. Run untrusted code inside an nsjail sandbox with cgroup limits. Think about the Go module layout and the nsjail vs gVisor timing before the session.
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md and docs/phases/phase-0-summary.md, then start Phase 1 (Sandbox core). Ask me the recap question and show me the session plan before writing any code.
  ```

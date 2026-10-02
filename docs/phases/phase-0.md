# Phase 0: Foundation

**Branch:** `phase/0-foundation`
**Range:** `phase-0-start..phase-0-done`
**Dates:** 2026-10-02 to 2026-10-02
**Milestone:** none

## Summary
Phase 0 set up the repository so later phases can build safely: commit-message and formatting guards, the planned directory layout, a Next.js web app with the LeetForce brand tokens, a LeetCode-style shell (navbar, theme toggle, problem table with sample data), and the project guidance in `CLAUDE.md`. No backend exists yet.

Note: `docs/PLAN.md` does not exist in the repo, so the criteria below are the foundation checks actually run, not criteria copied from the plan.

## Exit criteria
| Criterion | Status | Evidence |
|---|---|---|
| Lint passes | ✅ | `npm run lint` (in `web/`) → no errors |
| Types check | ✅ | `npm run typecheck` → no errors |
| Formatting clean | ✅ | `npm run format:check` → "All matched files use Prettier code style!" |
| App builds | ✅ | `npm run build` → compiled; routes `/` and `/problems` |
| Commit hooks enforce Conventional Commits | ✅ | A `merge: ...` commit message was rejected by commitlint (type-enum) |
| Directory layout from CLAUDE.md exists | ✅ | `.gitkeep` placeholders for judge, runner, api, infra, packer, ansible, k8s, problems, docs |
| Pushed to remote and merged to `main` | ✅ | `main` at `4d2245c` on origin |
| `docs/PLAN.md` exists | ❌ | File missing; deferred, see below |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/0-ui-shell` | Grey theme, system fonts, navbar, theme toggle, problem table, logo variant, design rules | 4 |

Earlier Phase 0 commits (README/logo, CLAUDE.md, CI hooks, web scaffold, layout) were made directly on the phase branch.

Merge commits: `a7625cc` (UI shell into phase branch), `80293f3` and `4d2245c` (phase into `main`; the second syncs with GitHub pull request #1).

## File-by-file changes

### Added
| File | Purpose |
|---|---|
| `.editorconfig`, `.gitattributes`, `.gitignore` | Consistent editor settings, line endings, and ignored files (`.env`, build output) |
| `.husky/commit-msg` | Runs commitlint on every commit message |
| `.husky/pre-commit` | Runs lint-staged before every commit |
| `.lintstagedrc.json` | Formats and lints staged files under `web/` |
| `commitlint.config.mjs` | Conventional Commits with the LeetForce type and scope lists |
| `package.json`, `package-lock.json` | Root tooling: husky, commitlint, lint-staged (lockfile generated) |
| `CLAUDE.md` | Project guidance: workflow, security rules, brand and design standard |
| `ansible/`, `api/`, `infra/aws/`, `infra/neon/`, `judge/`, `k8s/`, `packer/`, `problems/`, `runner/` (`.gitkeep` each) | Placeholders for the planned top-level layout |
| `docs/adr/.gitkeep`, `docs/phases/.gitkeep` | Placeholders for ADRs and phase documents |
| `web/.gitignore`, `web/.prettierignore`, `web/.prettierrc.json` | Web ignore and Prettier config (Tailwind class sorting) |
| `web/eslint.config.mjs`, `web/next.config.ts`, `web/postcss.config.mjs`, `web/tsconfig.json` | Next.js, ESLint, Tailwind, strict TypeScript config |
| `web/package.json`, `web/package-lock.json` | Web dependencies and scripts (lockfile generated) |
| `web/public/brand/logo.svg` | The black-background logo (README/OG) |
| `web/public/brand/logo-mark.svg` | Logo without background (navbar, favicon) |
| `web/public/brand/logo-mark-light.svg` | Same mark with the centre bar dark grey for light theme |
| `web/public/fonts/.gitkeep` | Reserved; no web fonts are used |
| `web/src/app/globals.css` | Color tokens, light/dark roles, Tailwind aliases, focus ring, logo theme swap |
| `web/src/app/layout.tsx` | Root layout: metadata, favicon, pre-paint theme script, navbar |
| `web/src/app/page.tsx` | Redirects `/` to `/problems` |
| `web/src/app/problems/page.tsx` | Problem list page using sample data |
| `web/src/app/contest/.gitkeep`, `leaderboard/.gitkeep`, `login/.gitkeep` | Route placeholders |
| `web/src/components/layout/Navbar.tsx` | Sticky top nav with logo, links, theme toggle, sign-in |
| `web/src/components/layout/ThemeToggle.tsx` | Light/dark toggle (sets `data-theme`, persists to `localStorage`) |
| `web/src/components/problems/ProblemTable.tsx` | Problem table with difficulty colors |
| `web/src/lib/sample-problems.ts` | Five placeholder problems with original titles |
| `web/src/components/ui/.gitkeep`, `components/workspace/.gitkeep`, `hooks/.gitkeep`, `lib/api/.gitkeep`, `styles/.gitkeep`, `tests/.gitkeep`, `types/.gitkeep` | Placeholders for later phases |

### Modified
| File | What changed | Why |
|---|---|---|
| `README.md` | Added centered logo, title and one-line description | Public project header |

### Deleted
None.

### Renamed / moved
None.

## Key code changes
1. **Token-only theming** (`web/src/app/globals.css`): semantic variables (`--background`, `--panel`, `--hover`, ...) are redefined for dark mode both via `prefers-color-scheme` and `[data-theme="dark"]`, so the toggle overrides the system setting.
2. **Pre-paint theme script** (`layout.tsx`): reads `lf-theme` from `localStorage` before first paint to avoid a flash of the wrong theme; wrapped in try/catch because storage can be blocked.
3. **Theme toggle without effects** (`ThemeToggle.tsx`): uses `useSyncExternalStore` with a `MutationObserver` on `data-theme`, so the icon always reflects the real attribute.
4. **Logo swap by theme** (`Navbar.tsx` + CSS): two `<Image>` elements, one hidden per theme, because CSS variables cannot recolor an SVG loaded as an image.
5. **Commit guard** (`commitlint.config.mjs`, `.husky/*`): enforces type and scope lists from CLAUDE.md; it caught a non-conforming `merge:` message.

## Decisions
- [ADR 0001](../adr/0001-web-design-system.md): tokens only, neutral black/grey dark theme, system fonts, three logo files.
- [ADR 0002](../adr/0002-go-module-layout.md): one Go module per component joined by `go.work`; Gin and pgx only in `api/`.

## Tests
No automated tests exist yet (no logic beyond rendering). Verification commands, run in `web/`:
```bash
npm run lint && npm run typecheck && npm run format:check && npm run build
```

## Known issues and deferred work
- `docs/PLAN.md` is missing; create it before Phase 1 (user).
- Go module layout is decided (ADR 0002); the `go.mod` files and `go.work` are created in Phase 1.
- No `Makefile` yet, so `make fmt lint test` from CLAUDE.md do not exist (Phase 1).
- `web/AGENTS.md` and `web/CLAUDE.md` are generated by `next dev` and left untracked.
- CLAUDE.md prescribes `merge: ...` commit messages but commitlint rejects that type; align one of them.
- `/problems/<slug>` links 404 until the workspace page exists (later phase).
- Problem list has no search, filters, or pagination yet; data is hard-coded sample data.

## Stats
- Commits: 11 (excluding merges), plus the closing docs commits
- Files: 52 added, 1 modified, 0 deleted (before the closing docs commits)
- Lines: +9463 / -1 (about 987 excluding the two lockfiles)

# Phase 7: Web: problems and workspace

**Branch:** `phase/7-web-workspace`
**Range:** `phase-7-start..phase-7-done`
**Dates:** 2026-10-02 to 2026-10-03
**Milestone:** none
**Log:** [phase-7-log.md](phase-7-log.md)

## Summary
The web app now shows real problems. A problem list with search, difficulty and tag filters and pagination reads from the Go API, and each problem opens in a split-pane workspace with the statement, a language selector, a Monaco editor with starter code, and a console showing sample cases. The API gained statements, starter code, acceptance rates and a filtered, paged list, plus four new original problems. Run and Submit are visible but disabled until Phase 8.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Browse real problems | ✅ | Chrome against the real API and Neon: `/problems` lists 5 problems with filters; see the log, "Local run" and unit 5 |
| Open real problems | ✅ | `/problems/fizz-count` renders statement, Monaco with Python starter, sample cases; unknown slug returns 404 |
| Keyboard-accessible | ✅ with gaps | Focus ring (2px sky) seen on nav links; tab order checked; separator arrow keys 50 to 60; tabs follow the ARIA tab pattern. Not browser-tested: filter form and pagination links by keyboard |
| Both themes | ✅ with one open item | Rendered in dark and light. Active tab contrast fixed (2.98:1 to 7.44:1). Light-mode difficulty text `--lf-success` is 3.30:1 (below AA); the token is fixed by CLAUDE.md, owner to decide |
| New problems judge correctly (own gate) | ✅ | 16 reference solutions (4 problems x 4 languages) all AC in the real sandbox on the dev host |

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `feat/7-api-problem-fields` | statement, starters, acceptance, filtered list, 4 problems | 3 |
| `feat/7-web-api-client` | typed API client, `/api` rewrite | 1 |
| `feat/7-problem-list` | problem list page, filters, pagination | 1 |
| `feat/7-workspace` | split pane, tabs, Monaco, Markdown statement | 3 |
| `fix/7-sample-field` | read the sample answer from the API's `expected` field | 1 |
| `docs/7-progress` | log units 1 to 4 and resume point | 1 |
| `test/7-web-a11y` (with `fix/7-dark-link-contrast`) | browser accessibility check and the contrast fix | 2 + 1 |
| `test/7-judge-new-problems` | sandbox judge, lint, SQL, Trivy results, ADR 0015, FLOW.md | 1 |

## File-by-file changes
Generated from `git diff --name-status phase-7-start..HEAD` before the report and summary were committed (155 files); the report and summary themselves are added after.

### Added
| File | Purpose |
|---|---|
| `api/internal/catalog/content_test.go` | tests that the catalog serves statement and starters |
| `api/internal/server/problems_list_test.go` | tests list filtering, paging and bad-input 400s |
| `docs/adr/0015-catalog-content-and-workspace-delivery.md` | ADR for this phase |
| `docs/phases/phase-7-log.md` | running log |
| `docs/phases/phase-7.md` | this report |
| `docs/phases/phase-7-summary.md` | plain-language summary |
| `judge/problem/content_test.go` | tests statement and starter loading; pins that they do not change the test-set version |
| `problems/<slug>/problem.yaml`, `statement.md` | for fizz-count, max-subarray-sum, reverse-words, valid-parentheses-lite (8 files) |
| `problems/<slug>/starters/{python.py,cpp.cpp,java.java,go.go}` | starter code per language (16 files) |
| `problems/<slug>/solutions/{python,cpp,java,go}/ac.*` | reference solutions, one per language (16 files) |
| `problems/<slug>/tests/NN.in`, `NN.out` | hidden tests: 10 cases each for three problems, 12 for valid-parentheses-lite (84 files) |
| `web/src/app/problems/[slug]/page.tsx` | workspace page (server component, 404 on unknown slug) |
| `web/src/components/problems/Pagination.tsx` | previous/next links, disabled state |
| `web/src/components/problems/ProblemFilters.tsx` | GET search form: text, difficulty, tag |
| `web/src/components/workspace/CodeEditor.tsx` | Monaco wrapper with theme and `ariaLabel` |
| `web/src/components/workspace/SplitPane.tsx` | draggable and keyboard-resizable divider (`role=separator`) |
| `web/src/components/workspace/Tabs.tsx` | ARIA tabs with arrow-key navigation |
| `web/src/components/workspace/Workspace.tsx` | left statement pane, right editor and console, language selector, disabled Run and Submit |
| `web/src/lib/api/client.ts` | typed fetch client; direct on the server, `/api` in the browser |
| `web/src/types/problem.ts` | TypeScript types for the API shapes |

### Modified
| File | What changed | Why |
|---|---|---|
| `.env.example` | documents `LEETFORCE_API_URL` | web needs the API address without hard-coding it |
| `api/cmd/api/main.go` | passes the catalog as `Content` in `server.Deps` | the catalog supplies statements and starters |
| `api/internal/catalog/catalog.go` | `Statement` and `Starters` accessors | serve the new content |
| `api/internal/server/problems.go` | list filters, paging, acceptance; detail adds statement and starters | the UI's data needs |
| `api/internal/server/server.go` | adds the optional `Content ContentSource` dependency | handlers read statement and starters through an interface |
| `api/internal/store/problems.go` | acceptance query (AC over non-IE verdicts, null when none) | computed, not stored |
| `api/internal/store/problems_test.go` | acceptance test on a throwaway schema (66.6 to 66.7 for 2 of 3) | exit gate for the SQL |
| `docs/FLOW.md` | Phase 7 ticked and "as built" section added | CLAUDE.md rule |
| `docs/PROGRESS.md` | status and resume point | session state |
| `judge/problem/problem.go` | loads optional `statement.md` and `starters/<lang>.<ext>` | content beside `problem.yaml` |
| `web/next.config.ts` | rewrite `/api/*` to `LEETFORCE_API_URL` | no CORS, no internal URL in the bundle |
| `web/package.json`, `web/package-lock.json` | add `@monaco-editor/react`, `monaco-editor`, `react-markdown` | editor and statement rendering (lockfile is generated) |
| `web/src/app/globals.css` | `--link` role (blue light, sky dark) and `--color-link` | active tab was 2.98:1 in dark mode |
| `web/src/app/problems/page.tsx` | now fetches from the API with filters and pagination | replaces mock data |
| `web/src/components/problems/ProblemTable.tsx` | real fields, acceptance column, `hover:text-link` | real data and contrast |

### Deleted
| File | Reason |
|---|---|
| `web/src/lib/sample-problems.ts` | mock data from Phase 0, replaced by the API |

### Renamed / moved
None.

## Key code changes
- **Catalog content** (`judge/problem/problem.go`): `statement.md` and `starters/` are optional files beside `problem.yaml`; a test pins that changing them leaves the test-set version unchanged, so edits never trigger a rejudge.
- **Acceptance** (`api/internal/store/problems.go`): `100 * AC / count(verdicts excluding IE)`, null when there are no verdicts, so the UI shows a dash rather than 0%.
- **Server-side data, browser proxy** (`web/src/lib/api/client.ts`, `web/next.config.ts`): server components call the API directly; browser code goes through `/api`, so no internal URL reaches the bundle.
- **Keyboard-operable workspace** (`SplitPane.tsx`, `Tabs.tsx`): the divider is a focusable `role=separator` with arrow-key resizing and `aria-valuenow`; tabs use roving `tabIndex`.
- **Contrast fix** (`globals.css`): `--link` is `--lf-blue-600` in light and `--lf-sky-400` in dark; measured in the browser, 2.98:1 became 7.44:1.

## Decisions
- [ADR 0015](../adr/0015-catalog-content-and-workspace-delivery.md): problem content from the on-disk catalog (no migration), computed acceptance, server-rendered pages, Monaco from the jsdelivr CDN.

## Tests
- `go test ./...` in `api/` and `judge/problem`; `make fmt lint` and `make test` on the dev host: clean.
- `TestProblemAcceptance`, `TestProblems` (`go -C api test -run TestProblem ./internal/store/`, needs `DATABASE_URL`; uses a throwaway schema).
- 16 reference solutions in the sandbox: `sudo -n bin/judge run -lang <l> problems/<p> problems/<p>/solutions/<l>/ac.*`.
- Web: `npm run lint`, `typecheck`, `build` in `web/`. No automated browser tests yet.
- Trivy 0.75.0, `trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1`: 0 findings.

## Known issues and deferred work
- Light-mode difficulty text contrast 3.30:1 (owner decision pending).
- Narrow-screen layout and keyboard use of the filter form and pagination not verified in a browser.
- Monaco loads from a CDN (needs internet; a strict CSP must allow it). Bundling it is a Phase 12 option.
- Catalog changes need an API restart.
- `make test-sandbox`, `make test-adversarial` and `make test-live-e2e` were not re-run (no sandbox or submit changes).
- Run, Submit, console results and the Submissions tab: Phase 8. Solved status: Phase 9.
- Phase 6 and earlier review answers are still outstanding (see PROGRESS.md).

## Stats
- Commits: 16 (excluding merges), measured with `git log --no-merges phase-7-start..HEAD` after the summary commit
- Files: 140 added, 16 modified, 1 deleted (157), same measurement
- Lines: +3885 / -140, same measurement; this stats edit and the Q&A record commit add a few more lines

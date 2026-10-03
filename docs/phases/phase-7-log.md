# Phase 7 log: Web: problems and workspace

Running log (CLAUDE.md "Documenting every step"). Entries: who, command, result, mistakes.

## Session 1

### Start of session
- Claude read CLAUDE.md, PROGRESS.md, phase-6-summary.md; repo state matched (main at 9c3c688, tags phase-0..6 present, untracked: .claude/, web/AGENTS.md, web/CLAUDE.md).
- Owner skipped the recap question and decisions with "okay"; Claude's defaults apply: (1) statement.md + starters/ beside problem.yaml, (2) acceptance % computed from submissions, (3) 4-6 original seed problems, (4) verify locally against the real .env.
- Claude ran `git checkout -b phase/7-web-workspace` and `git tag phase-7-start`.

### Units (checklist)
- [x] 1 feat/7-api-problem-fields (subagent in a worktree; merged)
- [x] 2 feat/7-web-api-client (merged)
- [x] 3 feat/7-problem-list (merged)
- [x] 4 feat/7-workspace (merged)
- [x] 5 test/7-web-a11y (merged)
- [x] 6 test/7-judge-new-problems (docs and checks; merged)

### Unit 1: API problem fields (subagent, Windows worktree)
- Claude delegated to a subagent; branch `feat/7-api-problem-fields`, 3 commits (loader for `statement.md` and `starters/<lang>.<ext>`; API statement, starters, acceptance, filtered/paged list; 4 new original problems: fizz-count, reverse-words, max-subarray-sum, valid-parentheses-lite).
- No migration: statement and starters are read from the catalog on disk like samples. Acceptance is `100*AC/count(verdicts excluding IE)` or null. Test-set version does not change with statement or starter edits (test pins it).
- Verified: `go vet` and `go test ./...` in `api/` pass (re-run by Claude); `judge/problem` tests pass. Reference solutions: all 16 (4 problems x 4 languages) were run OUTSIDE the sandbox (trusted own code, scratch script) and matched every test. NOT verified: the sandbox-based judge on the new problems, `make fmt lint`, the acceptance SQL (needs DATABASE_URL, test skipped).
- Judge module does not build on Windows (Linux-only syscalls in `judge/sandbox/gvisor.go`); judge tests must run on the dev host. Not yet done.
- Mistake: the API returns samples as `{name,input,expected}`; the web type used `output`. Fixed in `fix/7-sample-field`.

### Units 2 to 4: web (Claude, Windows)
- `web/next.config.ts`: rewrite `/api/*` to `LEETFORCE_API_URL` (default http://127.0.0.1:8080), documented in `.env.example`; `web/src/lib/api/client.ts`, `web/src/types/problem.ts`.
- Problem list: `web/src/app/problems/page.tsx`, `components/problems/{ProblemTable,ProblemFilters,Pagination}.tsx`; filters are a GET form; mock `lib/sample-problems.ts` deleted.
- Workspace: `web/src/app/problems/[slug]/page.tsx`, `components/workspace/{Workspace,SplitPane,Tabs,CodeEditor}.tsx`; deps `@monaco-editor/react`, `monaco-editor`, `react-markdown`. Monaco loads its runtime from the jsdelivr CDN (library default); bundling it locally is a later option. Run and Submit are disabled until Phase 8.
- Mistake: the first commit of unit 3 was rejected by the commit-msg hook (body line length), and my retry landed on the phase branch. Fixed by moving the commit to its branch (`git branch`, `git reset --hard` on the unpushed phase branch) and re-merging with `--no-ff`.
- Checks: `npm run lint`, `typecheck`, `build` pass.

### Local run (Claude, owner-authorised)
- Ran `go -C api run ./cmd/api` with `.env` loaded, `LEETFORCE_S3_ENDPOINT` blanked and `LEETFORCE_PROBLEMS_DIR` set to the repo's `problems/`. Startup upserted the 5 problems into the real Neon DB. `/readyz` ok, `/problems` returned total 5.
- Ran `npm run dev` in `web/`: `/problems`, filters, `/problems/fizz-count` returned 200, an unknown slug 404, the `/api` proxy worked. Both processes stopped afterwards.
- Chrome extension was not connected, so no screenshots, no visual check of light/dark, no keyboard test yet.

### Unit 5: browser accessibility check (Claude, Chrome extension connected on the second attempt)
- Claude ran API (`go -C api run ./cmd/api`, env as in "Local run") and `npm run dev` in `web/`; both stopped afterwards. Window was 1249x559, system theme dark; theme switched via `data-theme` in the page.
- Verified in the browser: `/problems` and `/problems/fizz-count` render real data in dark and light; global `:focus-visible` is a 2px sky outline (seen on the "Contest" nav link); tab order is nav, theme toggle, Sign in, tabs, separator, language select, editor, separator, console tabs; the vertical separator responds to arrow keys (aria-valuenow 50 to 60 after 5 presses); real click then Tab in Monaco indents (normal Monaco trap; escape via Ctrl+M, not surfaced in the UI); Run and Submit disabled (Phase 8).
- Finding fixed (`fix/7-dark-link-contrast`, efa77fe): active tab was `--lf-blue-600` on the dark panel at 2.98:1. Added a `--link` role (blue-600 light, sky-400 dark), `text-link` and `border-link` on the active tab and `hover:text-link` on links. Re-measured: 7.44:1 dark, 5.85:1 light.
- Mistake: my Python edit script rewrote four files with CRLF, so prettier flagged them; fixed with `sed` and `prettier --write`.
- NOT verified: narrow-width layout (the resize tool did not change the viewport; code hides Tags and Acceptance below `sm`); keyboard use of the filter form and Pagination links in the browser; Monaco's own contrast.
- Open for the owner (unit 5 finding): light-mode "Easy" (`--lf-success` #16A34A on white-ish) measures 3.30:1, below AA 4.5:1 for 14px text. The token is fixed by CLAUDE.md, so Claude did not change it; options are a darker light-mode text variant or accepting it.

### Unit 6: judge, lint, SQL and Trivy checks (Claude, dev host `leetforce-dev`)
- Claude confirmed the host was up (`ssh leetforce-dev`, up 19 h; a read-only check, nothing started). The host repo was on `phase/6-sandbox-hardening` with no remote, so Claude synced with a bundle: `git bundle create $TEMP/p7.bundle phase/7-web-workspace main`, `scp` to `~/p7.bundle`, `git fetch ~/p7.bundle phase/7-web-workspace:phase/7-web-workspace`, `git checkout phase/7-web-workspace` (host HEAD de84473).
- `make fmt lint` (golangci-lint, 5 modules): 0 issues, working tree unchanged. `make test`: all packages ok (sandbox-backed tests skip without root).
- `make build-judge`, then for each of fizz-count, max-subarray-sum, reverse-words, valid-parentheses-lite and each of python, cpp, java, go: `sudo -n bin/judge run -lang <l> problems/<p> problems/<p>/solutions/<l>/ac.*`. All 16 returned AC on every hidden test (10 tests, 12 for valid-parentheses-lite). Java took 56 to 90 ms and 17 to 18 MiB; Python 23 to 36 ms and 4 to 5 MiB. This closes the unit 1 gap (the earlier run was outside the sandbox).
- Acceptance SQL: `go -C api test -count=1 -v -run TestProblem ./internal/store/` on Windows with `.env` loaded: `TestProblems` and `TestProblemAcceptance` PASS (not skipped). The harness builds a throwaway schema from the migrations and drops it, so the real tables are untouched.
- Trivy 0.75.0 on the host: `trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1 --skip-dirs node_modules,bin .`: 0 vulnerabilities in the api, judge, queue, runner and storage `go.mod` files and in `web/package-lock.json`; no secrets or misconfigurations reported. No `.trivyignore` needed. The vulnerability DB date was not captured (output cut off).
- Not run: `make test-sandbox` and `make test-adversarial` (no sandbox code changed this phase), `make test-live-e2e` (submit behaviour unchanged).
- Mistake: my first multi-file shell command failed to parse (nothing ran); the ADR and FLOW edits were redone with the file tools.
- Written: ADR 0015 (`docs/adr/0015-catalog-content-and-workspace-delivery.md`); FLOW.md "Phase 7 (as built)".
- The dev host is still running and billable; the owner should stop it when idle.

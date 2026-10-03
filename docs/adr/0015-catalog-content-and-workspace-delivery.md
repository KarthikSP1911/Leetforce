# 0015. Problem content comes from the catalog on disk; the workspace is server-rendered Next.js with Monaco from a CDN

**Status:** accepted (Phase 7). The owner replied "okay" to the session plan, so these are Claude's defaults and can be changed.

## Context
Phase 7 needs real problems in the browser: statement text, starter code per language, difficulty and tags, and an acceptance rate. The API already read `problem.yaml` and sample tests from the problems directory. A migration for statements would also have to be kept in step with the files that authors edit and that the test-set version is computed from.

## Decision
- **Statement and starters live beside `problem.yaml`** as `statement.md` and `starters/<lang>.<ext>`, loaded by `judge/problem` and served by `GET /problems/:slug` (the catalog on disk, like samples). No migration. Editing a statement or starter does not change the test-set version (a test pins this), so it never triggers a rejudge.
- **Acceptance is computed** in `api/internal/store/problems.go` as 100 x AC / count of verdicts excluding IE, or null when there are none. Not stored, so it cannot drift.
- **List filtering and paging** (`q`, `difficulty`, `tag`, `page`, `page_size`) happen in the API over the in-memory catalog, which is small. Out-of-range input returns 400.
- **Web:** pages are server components that call the API directly (`LEETFORCE_API_URL`); browser code uses an `/api/*` rewrite in `web/next.config.ts` to avoid CORS and keep the internal URL out of the bundle. The list filters are a plain GET form, so they work without JavaScript and are keyboard-accessible by default.
- **Monaco** comes from `@monaco-editor/react`, which loads the editor runtime from the jsdelivr CDN by default. We kept that default.
- **Theme link colour:** a `--link` role (blue-600 in light, sky-400 in dark), because blue-600 on the dark panel measured 2.98:1.

## Alternatives
- **Statements in Postgres:** editable at runtime, but needs a migration, an import path and sync with files; Phase 10 (problem pipeline) is the right time to decide that.
- **Computing acceptance on the client or caching it:** extra moving parts for five problems.
- **Bundling Monaco locally** (`loader.config({ monaco })` plus webpack workers): works offline and removes a third-party host, but adds build complexity. Revisit with the production deploy (Phase 12) or if the CDN is a problem.
- **Client-side fetching for the list:** a flash of empty state and no result without JavaScript.

## Consequences
- Verified: `go test ./...` in `api/` and `judge/problem`; `TestProblemAcceptance` against Neon (66.6 to 66.7 for 2 AC of 3); 16 reference solutions AC in the real sandbox; `npm run lint`, `typecheck`, `build`; browser check in both themes (see the phase log).
- The page loads Monaco from `cdn.jsdelivr.net`: it needs internet, and a strict Content-Security-Policy later must allow that host (or we bundle).
- Catalog changes need an API restart (the catalog is read at startup).
- Light-mode difficulty text (`--lf-success` on white) is 3.30:1; the token is fixed by CLAUDE.md, owner to decide.

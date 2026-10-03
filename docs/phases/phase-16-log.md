# Phase 16 log: Launch readiness

Running log of every step of Phase 16 (CLAUDE.md "Documenting every step"). Entries are added per unit of work.

## Security review

Unit `feat/16-security-review`, branched from `phase/16-launch-readiness` at `62e6926`. Findings and statuses: [docs/security-review.md](../security-review.md).

1. Claude: `git checkout -b feat/16-security-review`. Result: branch created.
2. Claude: read every file under `api/internal/server`, `api/internal/store`, `api/internal/ingest`, `api/internal/catalog`, `api/cmd/api`, `runner/internal/agent`, `runner/internal/problems`, `runner/nodb_test.go`, `queue/run.go`, `storage/storage.go`, `judge/problem/bundle.go`, `judge/sandbox/args.go` and `cgroup.go` (read only), `web/src`, `web/next.config.ts`, `infra/aws/*.tf`, `infra/bootstrap`, `k8s/base/*`, `ansible/roles/hardening`, `scripts/runner/*`, `scripts/k3s/*`, `.github/workflows/deploy.yml`, `docker-compose.yml`, `.env.example`, `.gitignore`, `.trivyignore`. No files changed by this step.
3. Claude: history checks for secrets.
   - `git log --all --oneline -G 'AKIA[0-9A-Z]{16}|npg_[A-Za-z0-9]{8,}|PRIVATE KEY-----'`: no output.
   - `git log --all --oneline -G '(rediss|postgres|postgresql)://[^ :/<$]+:[^ @<$]{6,}@'`: no output.
   - `git ls-files | grep -E '(^|/)\.env'`: only `.env.example`. `git log --all --diff-filter=A -- .env '*.pem' '*.tfstate' '*.tfvars'`: attempted in a form the sandbox refused to verify; the working-tree check and the two content searches above were used instead.
   - `git grep` for hard-coded secret-looking assignments (excluding tests, docs, lockfiles): only `http_tokens = "required"` (IMDSv2 setting, not a secret).
   - `git grep` for IPv4 literals in docs, Terraform, scripts: only public DNS resolvers and private ranges in docs, no owner or host address.
4. Owner machine (Claude through Bash): `ssh -o BatchMode=yes -o ConnectTimeout=15 leetforce-dev 'echo ok'`. Result: `ok`, dev host reachable (one attempt, no retry needed).
5. Claude: `bash scripts/scan-staged.sh full` (Trivy 0.75.0 on the dev host, scans `git archive $(git write-tree)`, i.e. the tree of `62e6926`). Result: exit 1. 0 vulnerabilities in `api`, `judge`, `queue`, `runner`, `storage` Go modules and `web/package-lock.json`; Dockerfile clean; 1 HIGH misconfiguration, `AWS-0132` in `infra/bootstrap/main.tf:45-52` (S3 bucket encrypted with SSE-S3, not a customer managed key). Handled as SEC-12 (accepted, suppressed in `.trivyignore` with expiry 2027-04-03). Re-run of the scan on the final tree is left to the integrator's gate (needs a staged tree).
6. Claude: `go test` in `runner` for `TestRunnerHasNoDatabaseDependency`: pass (the "runners never connect to the database" guard still holds).
7. Fixes, each a small commit with a regression test (all `Refs: phase-16`, co-author trailer):
   - `37a8fc9` security(api): cap open event streams per client IP (SEC-01). Files: `api/internal/server/events.go`, `server.go`, new `events_ip_test.go`.
   - `f4c99f5` security(api): count login attempts per account, not per spelling (SEC-02). Files: `api/internal/server/auth.go`, new `security_login_test.go`.
   - `b3b4873` security(api): require application/json on state-changing POSTs (SEC-03). Files: `api/internal/server/server.go`, new `security_json_test.go`.
   - `6bdcab1` security(api): reap idle keep-alive connections (SEC-05). File: `api/cmd/api/main.go`. First tried `ReadTimeout` 30s; dropped it before committing because Go keeps the read deadline armed during a handler and would cut SSE streams.
   - `c5e2155` security(web): close open redirect through a backslash in ?next= (SEC-04). Files: `web/src/lib/safe-next.ts`, `safe-next.test.ts`, `web/src/components/auth/AuthForm.tsx`, `web/package.json` (new `test` script), `web/tsconfig.json` (`allowImportingTsExtensions`).
8. Mistakes and corrections during the unit:
   - The first Python edit script wrote with the Windows default encoding; the resulting diffs were checked and contained only the intended lines. Later edits used the Edit tool.
   - A `git commit -F -` in PowerShell 5.1 treated the message as a pathspec; messages were written to files in the scratchpad and passed with `-F <file>`.
   - The husky `lint-staged` hook runs `prettier` and `eslint` from `web/node_modules`, which the worktree does not have. For the web commit a directory junction `web/node_modules` pointing at the main checkout's `node_modules` was created, the commit made, and the junction removed again (the target was left intact).
   - The first version of the per-IP stream test tripped staticcheck SA4000 (identical operands); rewritten as a loop and committed as `1dba319`.
   - `npx tsc --noEmit` reports `Cannot find name 'LayoutProps'` in `web/src/app/layout.tsx`. That is a Next-generated type (needs `.next/types`, produced by `next build`) and is unrelated to these changes.
9. Checks run before commit: `golangci-lint run ./...` in `api` (0 issues after the SA4000 fix); `go vet ./internal/server/` and `go test ./internal/server/` in `api` (pass); `npm test` in `web` (2 tests pass); `prettier --check` and `eslint` on the touched web files (clean). `make fmt` was not used on the Windows machine; `gofmt -w` was applied to the touched Go files. Sandbox, adversarial and database tests were not run (they need the dev host); no sandbox code was changed.
10. Claude: added `AWS-0132 exp:2027-04-03` with a reason to `.trivyignore` (SEC-12) and wrote `docs/security-review.md`.

### Files touched by the security review

| Path | Change |
|---|---|
| `api/internal/server/events.go` | per-IP stream cap |
| `api/internal/server/server.go` | `requireJSON` middleware, slot construction |
| `api/internal/server/auth.go` | per-account-id login counter |
| `api/internal/server/events_ip_test.go`, `security_login_test.go`, `security_json_test.go` | new regression tests |
| `api/cmd/api/main.go` | `IdleTimeout` |
| `web/src/lib/safe-next.ts`, `web/src/lib/safe-next.test.ts` | new, redirect validation and its test |
| `web/src/components/auth/AuthForm.tsx`, `web/package.json`, `web/tsconfig.json` | use the new helper, `npm test`, TS extension imports |
| `.trivyignore` | `AWS-0132` with reason and expiry |
| `docs/security-review.md` | findings |
| `docs/phases/phase-16-log.md` | this file |

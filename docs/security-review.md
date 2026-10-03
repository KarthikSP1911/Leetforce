# LeetForce security review (Phase 16)

**Branch:** `feat/16-security-review`
**Scope:** the whole repository at `62e6926` (api, runner, judge, queue, storage, web, infra, k8s, ansible, packer, scripts, docker-compose.yml, `.env.example`, `.github/workflows`), checked against the CLAUDE.md security rules.
**Method:** code reading of every API response path and every runner/judge path that touches test data; `git log -G` and `git grep` over all history for key material; Trivy through the dev host; Terraform, Kubernetes, Ansible and systemd review.
**Added at integration (Claude, same phase):** the Phase 14 and 15 code (contests, standings, leaderboard) was merged after the review above and was read separately; it produced SEC-16 to SEC-18. The Trivy full scan was re-run on the merged tree (see "Re-scan").
**Not touched:** sandbox code (`judge/sandbox/`) was read only. No billable action was run and no cloud resource was started or stopped.

## Summary

| ID | Severity | Title | Status |
|---|---|---|---|
| SEC-01 | Medium | One IP can exhaust every SSE stream slot | FIXED |
| SEC-02 | Medium | Login limit bypass by alternating email and username | FIXED |
| SEC-03 | Low | State-changing POSTs accepted any Content-Type | FIXED |
| SEC-04 | Medium | Open redirect via `/\host` in the post-login `?next=` | FIXED |
| SEC-05 | Low | No idle timeout on the API server | FIXED |
| SEC-06 | High (before public launch) | API served over plain HTTP; session cookie not `Secure` | ACCEPTED (owner decision needed) |
| SEC-07 | Low | Sign-up reveals whether an email or username is taken | ACCEPTED |
| SEC-08 | Low | Per-account login limit lets anyone lock an account for 10 minutes | ACCEPTED |
| SEC-09 | Low | `push-ssm.sh` passes secret values on the `aws` command line | ACCEPTED |
| SEC-10 | Info | Runner hosts hold the Redis URL; a compromised runner could forge verdicts | ACCEPTED |
| SEC-11 | Medium | CI deploy role is root-on-control-host for anything merged to `main` | ACCEPTED (owner action needed) |
| SEC-12 | High (Trivy), accepted | AWS-0132: data bucket uses SSE-S3, not a customer managed key | ACCEPTED |
| SEC-13 | Low | Slow request bodies are not cut off in the API process | ACCEPTED |
| SEC-14 | Info | Submission and run results are readable by anyone holding the id | ACCEPTED |
| SEC-15 | Low | `X-Forwarded-Proto` decides the cookie `Secure` flag; trusted-proxy range is the whole pod CIDR | ACCEPTED |
| SEC-16 | Medium | Standings listed the problem slugs of a contest that had not started | FIXED |
| SEC-17 | Low | Neon dump file written world-readable when `pg_dump` runs in a container | FIXED |
| SEC-18 | Low | Standings and the global leaderboard are public and unmetered | ACCEPTED |

Everything else on the checklist was verified and found sound; see "Verified clean" at the end.

## Findings

### SEC-01 Per-IP cap on SSE streams (Medium) FIXED
- **Location:** `api/internal/server/events.go:56` (`streamSlots`), `:120` (`acquire(ip)`).
- **Description:** `GET /submissions/:id/events` needs no login and an instance allows 200 open streams. One client could open 200 and every other user would get 503 for live status (denial of service). Each stream also polls Postgres every 500 ms.
- **Fix:** streams are also bounded per client IP (default 10, `EventConfig.MaxStreamsPerIP`). The IP is `ClientIP()`, which ignores `X-Forwarded-For` unless the peer is a trusted proxy.
- **Commit:** `37a8fc9`. **Tests:** `TestStreamSlotsPerIPCap`, `TestStreamPerIPLimitReturns503` (`events_ip_test.go`).

### SEC-02 Login limit counted by spelling (Medium) FIXED
- **Location:** `api/internal/server/auth.go:220` (old key), `:234` (new key).
- **Description:** the per-account limit was keyed on the lower-cased text typed. One account can be named by email or by username, so an attacker got the full budget (10 per 10 minutes) under each name. The per-IP limit (20) still applied, but a distributed guesser was not slowed as designed.
- **Fix:** after the account is resolved a second counter keyed on the account id applies, so both spellings share one budget.
- **Commit:** `f4c99f5`. **Test:** `TestLoginLimitCountsAccountNotSpelling` (alternates both spellings, expects 401, 401, 429, 429).

### SEC-03 Content-Type not enforced on POST (Low) FIXED
- **Location:** `api/internal/server/server.go:100` (`requireJSON`), applied to `/auth/signup`, `/auth/login`, `/submissions`, `/runs`.
- **Description:** handlers parsed the body as JSON without checking the header. A cross-site HTML form can send `text/plain` with no CORS preflight. `SameSite=Lax` already keeps the session cookie off cross-site POSTs, so this was defence in depth only (it matters for same-site, cross-origin setups such as sibling subdomains).
- **Fix:** 415 unless the body is declared `application/json` (a charset parameter is fine). The web client and all e2e scripts already send it. No CORS headers are granted anywhere.
- **Commit:** `b3b4873`. **Test:** `TestPostRoutesRequireJSONContentType`.

### SEC-04 Open redirect after sign-in (Medium) FIXED
- **Location:** `web/src/components/auth/AuthForm.tsx` (old `safeNext`), now `web/src/lib/safe-next.ts`.
- **Description:** `?next=` was accepted when it started with `/` and not `//`. Browsers treat a backslash like a slash and strip tabs and newlines, so `/\evil.example` or `/<TAB>/evil.example` sent a freshly signed-in user to another site (phishing after a real login page).
- **Fix:** `safeNext` also refuses backslashes and control characters.
- **Commit:** `c5e2155`. **Test:** `web/src/lib/safe-next.test.ts` (`npm test`, Node's built-in runner; the web package had no test runner). `allowImportingTsExtensions` was added to `web/tsconfig.json` so the test can import the `.ts` file.

### SEC-05 Idle connections never reaped (Low) FIXED
- **Location:** `api/cmd/api/main.go:188`.
- **Description:** only `ReadHeaderTimeout` was set.
- **Fix:** `IdleTimeout` 2 minutes. `ReadTimeout` and `WriteTimeout` are deliberately not set: Go keeps the read deadline armed while a handler runs, so either would end SSE streams early.
- **Commit:** `6bdcab1`. **Test:** none possible in a unit test (server configuration); covered by the e2e suites at integration time.

### SEC-06 No TLS on the API; cookie not `Secure` in the cloud (High before public launch) ACCEPTED, owner decision
- **Location:** `k8s/base/api-ingress.yaml` (plain HTTP, Traefik port 80), `infra/aws/main.tf:76` (`control_http_owner`), `api/internal/server/auth.go:74` (`secureRequest`).
- **Description:** the control host serves the API on port 80 only, open to the owner's /32 and to the runner hosts. Passwords and the session cookie travel in clear text, and because `Secure` follows the request scheme (`X-Forwarded-Proto: https` or direct TLS) the cookie is issued without `Secure` there. Today only the owner's IP can reach it, so the exposure is the owner's own traffic and the VPC.
- **Why accepted for now:** nothing public is exposed yet, and Phase 13 recorded TLS as a follow-up (ADR 0021).
- **Owner decision needed before any public launch:** terminate TLS in front of the API (for example the Vercel-hosted web app proxying over HTTPS to a hostname with a certificate on Traefik, or an ALB/CloudFront, which adds recurring cost). Do not open port 80 to the world as it is. When TLS exists, set `X-Forwarded-Proto` at the edge so the cookie becomes `Secure`.

### SEC-07 Account enumeration at sign-up (Low) ACCEPTED
- **Location:** `api/internal/server/auth.go:187` (409 "that email or username is already taken").
- **Description:** sign-up says whether an email or username exists. Login is already uniform (same 401 text, a dummy bcrypt comparison keeps timing equal; `TestLoginRejects`).
- **Why accepted:** usernames are public by design (they will appear on the leaderboard) and the alternative (email verification for every sign-up) is a product change. Sign-up is limited to 20 per IP per 10 minutes.

### SEC-08 Account lock-out by a third party (Low) ACCEPTED
- **Location:** `api/internal/server/auth.go:220`, `:234`.
- **Description:** anyone who knows a username can burn its 10 attempts per 10 minutes and make the real owner wait. This is the cost of a per-account guess limit.
- **Why accepted:** the lock lasts at most one window, nothing is disclosed, and the alternative (no per-account limit) is worse against distributed guessing. Revisit with CAPTCHA or per-IP-and-account keys if abuse appears.

### SEC-09 Secrets on a command line (Low) ACCEPTED
- **Location:** `scripts/k3s/push-ssm.sh:45` (`aws ssm put-parameter --value "..."`).
- **Description:** the value is visible in the process list of the owner's own machine for the duration of one call. The script never prints values and `--apply` is explicit.
- **Why accepted:** single-user machine, short-lived, and the script cannot be tested without AWS. A `file://` value from a mode 0600 temp file would remove it; do that if the script ever runs on a shared host.

### SEC-10 Runner holds the Redis URL (Info) ACCEPTED
- **Location:** `infra/aws/runner-userdata.sh.tftpl`, `scripts/runner/leetforce-runner.service`.
- **Description:** the runner needs Redis, so `/etc/leetforce/runner.env` (0640 root:lfrunner) holds the Upstash URL. The sandbox has no network and a cleared environment (nsjail `-E` only), the runner IAM role reads only `/leetforce/runner/*` and `problems/*` in the bucket (never the database URL), and IMDS hop limit is 1. A runner-process compromise (not a sandbox escape) could still write forged verdicts to the results stream.
- **Why accepted:** inherent to the queue design; the blast radius is wrong verdicts, not data. Rotating the Upstash credential is the response.

### SEC-11 Deploy path is root on the control host (Medium) ACCEPTED, owner action needed
- **Location:** `.github/workflows/deploy.yml`, `infra/aws/ci.tf:31` (trust limited to `repo:KarthikSP1911/Leetforce:environment:production`), `:41-56` (SSM `AWS-RunShellScript` on the instance tagged `leetforce-control`).
- **Description:** the role is well scoped (OIDC, no stored keys, one document, one tagged instance), but what it runs is a script from the checked-out commit as root on the host that can read `/leetforce/*` in SSM, including the database URL. Whoever can push to `main`, or approve the `production` environment, controls that host.
- **Owner action:** in GitHub, protect `main` (required review, no force-push) and add required reviewers to the `production` environment. Both are repository settings that no code in this repository can enforce.

### SEC-12 Trivy AWS-0132 (accepted)
- **Location:** `infra/bootstrap/main.tf:45-52`.
- **Description:** `scripts/scan-staged.sh full` (Trivy 0.75.0) reports one HIGH misconfiguration: the data bucket uses SSE-S3 (AES256), not a customer managed KMS key. The bucket holds problem bundles (the hidden tests) and Terraform state, versioned, public access blocked, TLS-only.
- **Why accepted:** a CMK costs about 1 USD per month plus request charges for key-rotation and audit control that a one-owner project does not use yet. Suppressed in `.trivyignore` with an expiry of 2027-04-03. **Owner decision:** keep, or approve the recurring cost and switch to SSE-KMS.

### SEC-13 Slow request bodies (Low) ACCEPTED
- **Location:** `api/cmd/api/main.go` (server config).
- **Description:** a client can send headers quickly and trickle the body (at most about 300 KB, enforced by `MaxBytesReader`). A server-wide `ReadTimeout` would end SSE streams (see SEC-05), and a per-handler read deadline is not worth the code for three small endpoints.
- **Why accepted:** Traefik in front applies its own read timeouts, the per-IP and per-user limits bound the concurrency, and the request body cap is small.

### SEC-14 Results readable by id (Info) ACCEPTED
- **Location:** `api/internal/server/submissions.go:120` (`getSubmission`), `events.go`, `runs.go:121` (`getRun`).
- **Description:** these routes do not check that the caller owns the id. Ids are random UUIDs (122 bits) and the views carry only status, verdict label, runtime, memory and pass counts for Submit; a Run result (own source, samples and custom input only) expires after 10 minutes.
- **Why accepted:** nothing sensitive is exposed to someone who already holds the id, and it keeps the web polling simple. The source is never returned by any route (`store.Submission` has no source field; `TestStreamCarriesNoHiddenData`).

### SEC-15 Forwarded headers (Low) ACCEPTED
- **Location:** `api/internal/server/auth.go:74`, `k8s/base/api-configmap.yaml:18`.
- **Description:** `secureRequest` trusts the `X-Forwarded-Proto` header from any caller, which only lets a client weaken the `Secure` flag on its own cookie. `LEETFORCE_TRUSTED_PROXIES` is the whole pod CIDR (10.42.0.0/16), so another pod in the cluster could forge `X-Forwarded-For`; only the API runs in the namespace today.
- **Why accepted:** no cross-user effect. Narrow it to the Traefik pod address if more workloads are added to the cluster.

## Verified clean

| Rule or topic | How it was checked | Result |
|---|---|---|
| Hidden test input, expected output and raw stderr never returned for Submit | `store.Submission` and `VerdictView` have no such fields; the runner builds the Submit result with `engine.Options{}` (no `Detail`) and only `RunCase` carries detail, set from `engine.Detail`, which the engine fills only for `t.Sample` tests; the API stores no compile output or stderr (`VerdictRecord`); SSE emits `statusEvent` (status plus `VerdictView`); `fail()` returns a generic 500; `/readyz` hides error text | Clean. Runner logs carry job metadata only. |
| Run shows only samples or the user's own input | `runner/internal/agent/run.go` filters `p.Tests` by `Sample` before judging; `catalog.Samples` returns only `Sample` tests | Clean |
| Runners never connect to the database | `runner/nodb_test.go` (go.mod and the whole dependency graph, run during this review: pass); no `DATABASE_URL` in runner config; runner IAM role cannot read `/leetforce/api/*` | Clean |
| Rate limits per user and per IP | `limitUserAndIP` on submit and run; auth is per IP plus per account (SEC-02); the limiter fails closed (503) | Clean (see SEC-01 for streams) |
| Session handling | 32-byte random token, only its SHA-256 stored, 30-day expiry checked in SQL, `HttpOnly`, `SameSite=Lax`, `Path=/`, logout deletes the row, new token per login (no fixation) | Clean except SEC-06 |
| Password handling | bcrypt cost 12, 8 to 72 bytes (no silent truncation), uniform login errors with a dummy hash | Clean (SEC-07 accepted) |
| SQL injection | every query in `api/internal/store` is parameterised; no string building (`grep` for `Sprintf` and concatenation: none) | Clean |
| SSRF | the API and runner make no outbound HTTP request from user input; the only outbound calls are Redis, Postgres and S3 with fixed configuration | Clean |
| Unsafe deserialization and archives | queue jobs and results are JSON into typed structs; problem bundles are unpacked by `problem.Unpack`, which accepts only `problem.yaml` and flat `tests/*` regular files, caps total size, and verifies the recomputed test-set version; slugs and versions are regex-validated before any path or S3 key is built | Clean |
| Secrets in the repository | `.env` and `.env.*` are git-ignored (only `.env.example` tracked, values empty); `git ls-files` shows no `.env`, `*.pem`, `*.tfstate`, `*.tfvars`; `git log --all -G` for AWS key ids, Neon tokens, private-key headers and credentialed `rediss://`/`postgres://` URLs: no hits in any commit; no public IPs in docs; Trivy secret scan: clean | Clean |
| SSM and secret flow | secrets live in SSM SecureString, are read on the host with the instance role, written to `api-env` Secret or `runner.env` (0640) and never printed; `runner-userdata.sh.tftpl` accepts only `LEETFORCE_*` single-line values | Clean (SEC-09, SEC-10) |
| Web bundle leaks | the browser only calls `/api/*`; `LEETFORCE_API_URL` is read in `next.config.ts` and server-side `client.ts`, never `NEXT_PUBLIC_*`; Markdown statements go through `react-markdown` without raw HTML; the one `dangerouslySetInnerHTML` is a fixed theme script | Clean except SEC-04 |
| Terraform security groups | owner `/32` only (validated), no `0.0.0.0/0` ingress, runner SG has SSH from the owner only, egress limited by port, IMDSv2 required, runner hop limit 1, encrypted gp3 volumes, IAM scoped by prefix; CI role trust pinned to the `production` environment | Clean (SEC-06, SEC-11, SEC-12) |
| Host hardening | key-only SSH, no root login, ufw default deny, sysctl hardening; runner service unprivileged with no capabilities, `NoNewPrivileges`, `ProtectSystem=strict`, syscall filter | Clean |
| Kubernetes | non-root numeric uid, `readOnlyRootFilesystem`, all capabilities dropped, seccomp `RuntimeDefault`, no service-account token, `/metrics` not on the Service | Clean |
| Cgroup kill | `judge/sandbox/cgroup.go` writes `cgroup.kill` on timeout and limit breach and removes the cgroup before `Run` returns; nsjail has no network and an explicit environment (read only; the adversarial suite needs the dev host and was not run) | Clean |
| Dependencies and images | `scripts/scan-staged.sh full` on the dev host, Trivy 0.75.0: 0 vulnerabilities in `api`, `judge`, `queue`, `runner`, `storage` Go modules and `web/package-lock.json`; Dockerfile and Compose clean; one misconfiguration (SEC-12) | Clean apart from SEC-12 |

### SEC-16 Contest problems leaked through standings (Medium) FIXED
- **Location:** `api/internal/leaderboard/service.go` (`ComputeStandings`).
- **Description:** `GET /contests/:slug/standings` returned `problems` (the slugs) for any contest, including one that had not started, to anonymous callers. Contest problems are meant to be hidden until the start (they are 404 on the catalog endpoints). Statements and tests were never exposed, only the slugs.
- **Fix:** the list is empty before `starts_at` (commit `e9a69c9`). Regression test `TestStandingsHideProblemsBeforeStart` fails without the fix (`problems = [a b], want 0`).

### SEC-17 Dump file mode (Low) FIXED
- **Location:** `scripts/backup-neon.sh`.
- **Description:** found while running the restore drill. The script sets `umask 077`, but when `pg_dump` runs in a container (needed because the host client was PostgreSQL 16 and Neon runs 18) the file is created with the container's umask, so the dump, which holds emails, password hashes and submitted source, came out mode 644.
- **Fix:** `chmod 600` after the dump (commit `32fe90b`). The test dumps made before the fix were deleted from the host. No automated test: the behaviour depends on a container runtime, so it was checked by hand on the host.

### SEC-18 Public standings and leaderboard (Low) ACCEPTED
- **Description:** `GET /contests/:slug/standings` and `GET /leaderboard` need no sign-in and are not rate limited. They expose usernames, solve counts, penalties and, during a contest, which problem each user solved. They never expose source, hidden tests or verdict details.
- **Why accepted:** public rankings are the purpose of the feature. Database load is bounded because both responses come from a Redis snapshot (15 s and 60 s TTL) tied to a version counter, so a flood of reads recomputes at most once per TTL. Add a per-IP read limit if abuse appears.

## Re-scan

`scripts/scan-staged.sh full` (Trivy 0.75.0, vulnerability, secret and misconfiguration scanners, HIGH and CRITICAL) on the merged Phase 16 tree, including the new `framer-motion` dependency: clean, exit 0. The only suppression is the AWS-0132 entry in `.trivyignore` (SEC-12, expires 2027-04-03).

## Tests added

- `api/internal/server/events_ip_test.go`: per-IP stream cap (SEC-01).
- `api/internal/server/security_login_test.go`: shared login budget across email and username (SEC-02).
- `api/internal/server/security_json_test.go`: 415 for non-JSON POSTs (SEC-03).
- `web/src/lib/safe-next.test.ts`: open-redirect inputs (SEC-04).
- `api/internal/leaderboard/service_test.go`: `TestStandingsHideProblemsBeforeStart` (SEC-16).

Run: `cd api && go test ./internal/server/`, `cd web && npm test`.

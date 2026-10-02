# Phase 1: Sandbox core

**Branch:** `phase/1-sandbox-core`
**Range:** `phase-1-start..phase-1-done`
**Status:** working draft (report is completed at the end of the phase)

## Units of work
- [x] `docs/1-dev-environment`: ADR 0003, `scripts/setup-dev-host.sh`, README cost table
- [x] `feat/1-go-workspace`: `go.work`, `judge/go.mod`, `.golangci.yml`, `Makefile`, `.env.example`
- [x] `feat/1-nsjail-wrapper`: `judge/sandbox` Spec/Run, bounded output capture
- [ ] `feat/1-cgroup-limits`: cgroup v2 memory/pids/cpu limits, whole-cgroup kill, measurements
- [ ] `feat/1-result-channel`: dedicated fd for the harness result
- [ ] `test/1-adversarial`: `make test-adversarial` suite
- [ ] `docs/1-adrs-report`: ADR 0004, phase report, phase summary

## Exit criteria (from PLAN.md)
Fork bomb, memory bomb, infinite loop, output flood, network access, and file-system escape attempts are all contained; `make test-adversarial` passes.

## Environment
Developed on an Ubuntu 24.04 x86_64 EC2 t3.micro (see ADR 0003). Verified facts: cgroup v2 (`cgroup2fs`) with `cpu memory pids` controllers, `kernel.apparmor_restrict_unprivileged_userns = 1`, Go 1.27.1, nsjail built from `google/nsjail` commit `4ff54a6`.

## Environment setup log (2026-10-02)

Recorded so a later session can rebuild or audit the dev host. Rationale is in [ADR 0003](../adr/0003-dev-environment-ec2-x86.md); `scripts/setup-dev-host.sh` automates steps 3-6.

### Done by the owner (AWS console and Windows)
1. Launched EC2 instance `leetforce-dev`: Ubuntu Server 24.04 LTS x86_64, `t3.micro`, default VPC, auto-assigned public IPv4, 15 GiB gp3, ed25519 key pair, new security group with SSH from the owner's IP only (HTTP/HTTPS boxes unticked), no IAM instance profile.
2. Saved the private key as `~/.ssh/leetforce.pem` (outside the repo; `*.pem` is also git-ignored) and restricted its permissions with `icacls`.
3. Created `~/.ssh/config` with a `leetforce-dev` host entry (`User ubuntu`, `IdentityFile`, `ForwardAgent yes`). First attempt was saved as `config.txt` and renamed to `config`.
4. Connected once manually to accept the host key into `known_hosts`.

### Done by Claude over `ssh -o BatchMode=yes leetforce-dev '<cmd>'`
1. **Read-only checks:** cgroup type `cgroup2fs`; controllers `cpuset cpu io memory hugetlb pids rdma misc dmem`; `kernel.apparmor_restrict_unprivileged_userns = 1`; 911 MiB RAM, no swap, about 12 GB free disk; passwordless `sudo`; only `git` preinstalled.
2. **apt packages:** `build-essential git curl make pkg-config nodejs npm autoconf bison flex libtool libprotobuf-dev libnl-route-3-dev protobuf-compiler` (Node 18.19.1).
3. **Go 1.27.1** (linux-amd64 tarball from go.dev) unpacked to `/usr/local/go`; PATH line added to `~/.bashrc`.
4. **Swap:** 2 GiB `/swapfile`, enabled and added to `/etc/fstab`.
5. **nsjail** built from `google/nsjail` commit `4ff54a6` in `~/nsjail` and copied to `/usr/local/bin/nsjail`. Smoke test `sudo nsjail ... -- /bin/echo sandbox-ok` printed `sandbox-ok`; nsjail warned the process ran as UID 0 in the global user namespace, so unit 2 must set user-namespace options explicitly.
6. **Repo:** cloned `https://github.com/KarthikSP1911/Leetforce.git` to `~/Leetforce` (HTTPS; the box has no GitHub credentials), set global git name and email, checked out `phase/1-sandbox-core`.
7. **Script run:** `scripts/setup-dev-host.sh` run twice. First run installed golangci-lint 2.14.0 to `~/go/bin` and exposed a `pipefail` bug (fixed in commit `fix(infra): avoid pipefail abort in host setup summary`); second run changed nothing, confirming idempotence.

### State after setup
- Disk 6.4 GB used of 14 GB; swap 2 GiB; nothing running in the background.
- No credentials, instance role, or secrets on the box. No security-group or system configuration changes beyond swap.
- Pushing from the box needs credentials: the workflow is edit and commit on the Windows repo, push to `origin`, then `git pull` on the box to build and test.

## Unit log

### `feat/1-go-workspace` (2026-10-02)
Done by Claude, on the Windows repo then verified on the EC2 host.
1. Created `go.work` (`go 1.27`, `use ./judge`), `judge/go.mod` (`module leetforce/judge`), and a stub `judge/sandbox/doc.go` so vet and lint have a package to check.
2. Added `.golangci.yml` (golangci-lint v2, `standard` linters plus `errorlint`, `gosec`, `misspell`, `noctx`, `unconvert`; `gofmt` and `goimports` formatters), `.env.example` (`LEETFORCE_REDIS_URL=` blank), and the `Makefile` (`fmt lint test test-adversarial`, looping over `GO_MODULES := judge`).
3. Updated CLAUDE.md: the commands section now lists the real targets and how to run a single test.
4. Committed (`build(judge): add go workspace, judge module and Makefile`), pushed `feat/1-go-workspace`, then on the host: `git checkout feat/1-go-workspace && git pull`.
5. Verified on the host: `make fmt lint test` printed `0 issues.` and `[no test files]`; `make test-adversarial` ran `sudo -n env PATH=... go test -tags adversarial` successfully (no tests yet).
6. Mistake worth noting: a `python3` probe on Windows hung on the Microsoft Store stub, so I stopped it and used the editor tool. This was only a tooling slip; no Python is needed by the repo. (An earlier version of this line wrongly claimed neither machine has Python; that was never checked and has been removed. The EC2 host does have `/usr/bin/python3`, which one sandbox test uses.)

### `feat/1-nsjail-wrapper` (2026-10-02)
Developed on the Windows repo; each iteration was copied to the EC2 host with `tar czf - Makefile judge | ssh leetforce-dev 'cd ~/Leetforce && tar xzf -'` and tested there as root. Committed only once everything passed.

**Spike on the host (harmless programs only, no bombs).** Commands run as root through `ssh leetforce-dev`, from scratch files in `/tmp/spike` (small C programs compiled with gcc: a null-pointer dereference, `abort()`, and one that writes to fd 3). Verified facts:
1. `nsjail -Mo ... --user 65534 ... -R /usr -R /lib -R /lib64 -R /bin -T /tmp --disable_proc` runs programs; `/usr` is read-only, `/tmp` is writable, and `/etc/shadow` does not exist inside.
2. Network: Python `socket.connect` to `1.1.1.1:53` and `169.254.169.254:80` fails with `Network is unreachable` (new network namespace is nsjail's default).
3. nsjail reports a signal-killed child as exit code `128+signal` (139 for SIGSEGV, 137 for the time-limit SIGKILL). That is ambiguous with a real exit code, so the wrapper reads nsjail's own log instead.
4. `sudo` closes inherited fds above 2, so `--log_fd 3` through `sudo` failed with 255 and an empty log. The Go process itself must run as root (`sudo go test`) so it can start nsjail directly and pass fd 3.
5. With `--log_fd 3` the child gets `EBADF` writing to fd 3 (the log fd is closed before the program starts), so user code cannot forge log lines.
6. Log formats: `pid=N (...) exited with status: N`, `pid=N (...) terminated with signal: NAME (N)`, `pid=N run time >= time limit (...)`, `[E]`/`[F]` lines for nsjail's own failures, and an `Executing '<path>' for ...` line once the jail is built.
7. With `--user 65534` alone nsjail maps the jailed user to host **root** (`Uid map: inside_uid:65534 outside_uid:0`), so file permission checks would treat the program as root. The wrapper uses `--user 65534:65534:1` (and the same for `--group`) to map to an unprivileged host user.
8. `RLIMIT_NPROC` counts processes per host uid across all jobs, so it cannot be a per-job process cap; the process limit will be cgroup `pids.max` in the next unit. No fork-bomb-style test was run in this unit.
9. Kafel (seccomp) rejected `umount2` as an unknown identifier; the final denylist omits it (`mount` is denied, so it is not needed) and nsjail accepts the policy.

**Code (judge/sandbox).** `spec.go` (Spec, Limits, Result), `args.go` (nsjail command line, seccomp denylist, env defaults), `capture.go` (output cap that discards overflow and cancels the run once), `log.go` (nsjail log parser), `run.go` (starts nsjail with the log on fd 3, caps output, wall-time context, SIGTERM then force-kill), tests in `unit_test.go` and `run_test.go`, and a new `make test-sandbox` target.

**Bug found and fixed during this unit.** `TestRunSandboxFailure` (a missing bind-mount source) passed about 4 runs in 5. A 100-iteration loop showed nsjail sometimes ends a failed jail setup with `terminated with signal: SIGKILL` instead of `exited with status: 255`, so my first rule ("error lines plus exit 255") returned a fake result for a program that never ran. Fix: a run counts as started only if nsjail logged `Executing '...'`; otherwise `Run` returns `ErrSandbox`. A regression case in `TestParseLog` covers it. A shell `sed` edit also mangled a regex and a test table once; both were rewritten with full-file writes before any commit.

**Verification (EC2 host, 2026-10-02).** `make fmt` and `make lint`: `0 issues.` `make test`: ok. `make test-sandbox`: all tests PASS. `go test -count=200 -run TestRunSandboxFailure`: ok. `go test -count=30 -run TestRun`: ok (about 35 s). After the runs `pgrep -c nsjail` printed 0 (no leaked processes).

**Not done here (moved to later units).** Memory and process-count limits, CPU cgroup limit, whole-cgroup kill and peak-memory measurement (unit `feat/1-cgroup-limits`); a result fd for the harness (unit `feat/1-result-channel`); the containment test cases (unit `test/1-adversarial`). The seccomp denylist is only checked for being accepted by nsjail so far; its effect is tested in the adversarial suite.

## Additional log entries (added after a completeness check)

Steps that were performed earlier in the session but were not written down at the time.

### Phase start and publishing (Claude, Windows repo)
1. `git checkout main && git pull --ff-only` (already up to date), `git checkout -b phase/1-sandbox-core`, `git tag phase-1-start` (local; the repo's `main` had already been merged from `phase/0-foundation`).
2. `git push origin phase/1-sandbox-core phase-1-start` published the branch and tag. On the host: `git fetch && git checkout phase/1-sandbox-core` (the host clone started on `main`).
3. After each merged unit the phase branch was pushed and the unit branch deleted locally and on `origin`.
4. Untracked `web/AGENTS.md` and `web/CLAUDE.md` exist on the Windows repo from before this phase; they were left untouched and are not part of any commit (owner decision pending, default: ignore).

### Files left on the EC2 host (scratch, not in the repo)
- `/tmp/spike/`: C test programs compiled with gcc (`segv`, `abrt`, `wfd`) and probe scripts (`probe.sh`, `p2.sh`, `p3.sh`) plus `nslog*.txt`/`l*.txt` nsjail logs from the spike. They run only inside nsjail, were used for the findings above, and can be deleted with `rm -rf /tmp/spike`. `/tmp` is also cleared on reboot.
- `/tmp/apt.log`, `/tmp/nsjail-build.log`, `/tmp/go.tgz` from the initial install.

### Temporary debugging on the host (unit 2)
- Created `judge/sandbox/dbg_test.go` twice in `~/Leetforce` to print the wrapper's result in a loop (it found the setup-failure bug), and removed it after each run. It was never committed.
- After unit 2 was merged I ran `git checkout -- .` and `git clean -fdq judge Makefile` on the host to discard the files copied over by `tar`, then `git fetch`, `git checkout phase/1-sandbox-core`, `git pull --ff-only` (host now at the merge commit, clean working tree).

### Slips
- A `python3` heredoc on Windows hung and was stopped (logged above).
- Two `sed` edits mangled a regex and a test table; I rewrote the files in full before committing (logged above).

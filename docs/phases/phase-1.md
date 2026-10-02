# Phase 1: Sandbox core

**Branch:** `phase/1-sandbox-core`
**Range:** `phase-1-start..phase-1-done`
**Status:** working draft (report is completed at the end of the phase)

## Units of work
- [x] `docs/1-dev-environment`: ADR 0003, `scripts/setup-dev-host.sh`, README cost table
- [x] `feat/1-go-workspace`: `go.work`, `judge/go.mod`, `.golangci.yml`, `Makefile`, `.env.example`
- [x] `feat/1-nsjail-wrapper`: `judge/sandbox` Spec/Run, bounded output capture
- [x] `feat/1-cgroup-limits`: cgroup v2 memory/pids/cpu limits, whole-cgroup kill, measurements
- [x] `feat/1-result-channel`: dedicated fd for the harness result
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

### `feat/1-cgroup-limits` (2026-10-02)
Same workflow as unit 2: edit on Windows, `tar czf - Makefile judge | ssh leetforce-dev 'cd ~/Leetforce && tar xzf -'`, test on the host as root, commit only after everything passed.

**Spike on the host (all bounded; nothing was run without a cap).**
1. Read-only facts: `nsjail --help` lists `--use_cgroupv2`, `--cgroupv2_mount`, `--cgroup_mem_max`, `--cgroup_mem_swap_max`, `--cgroup_pids_max`, `--cgroup_cpu_ms_per_sec`. Root `cgroup.subtree_control` is already `cpu memory pids`. `memory.peak`, `memory.events`, `memory.oom.group` exist (kernel 6.17).
2. Created `/sys/fs/cgroup/leetforce` and a job directory (`mkdir`, `echo "+memory +pids +cpu" > cgroup.subtree_control`), ran nsjail with `--cgroupv2_mount <job dir>` and a 64 MiB limit (script `/tmp/spike/cg1.sh`). nsjail created `NSJAIL.<pid>` inside the job directory and removed it at exit. The job directory kept the totals: `memory.peak` 42,967,040 for a 40 MiB allocation, `cpu.stat usage_usec 30781`. A 100 MiB allocation under the 64 MiB limit was OOM-killed (exit 137); the job directory showed `oom_kill 1` and `memory.peak` 67,387,392 (about 0.4% above the limit, so the peak can slightly overshoot `memory.max`).
3. Process limit (`cg2.sh`): a program that forks in a loop under `--cgroup_pids_max 20` stopped with `fork failed after 19 children`; the job directory kept `pids.peak 20` and `pids.events max 1`. The forker was safe to run because of that cap, plus a 400 MiB `memory.max` backstop on the parent directory.
4. Discovery: the sandbox had no `/dev/null`, so `sleep 60 &` in `sh` failed ("cannot open /dev/null"). Runtimes also need `/dev/urandom`. Added bind mounts for `/dev/null` (rw), `/dev/zero` and `/dev/urandom` (ro); verified in `cg3.sh`.
5. `cgroup.kill` (`cg3.sh`): writing `1` to the job directory's `cgroup.kill` while a shell with three sleeps (one detached in a subshell) was running removed all four processes in about 2 ms; `cgroup.events` then showed `populated 0`; no sleep processes were left. The spike cgroups were removed with `rmdir` after each script.

**Design decided from the spike.** The code owns a per-run directory; nsjail makes and limits its own child cgroup inside it. This keeps nsjail's supervisor outside the limited cgroup, so an OOM kill takes only the program and nsjail's log stays intact, while the directory we own keeps the measurements and can kill the whole run. The alternative (starting nsjail itself inside our cgroup with `CgroupFD`) would have the OOM killer able to take nsjail down with the program and lose its log. To be recorded in ADR 0004.

**Code.** `cgroup.go` (job directory lifecycle, `kill`, `stats`, `remove` with a wait for `populated 0`, parsing helpers), `spec.go` (`MemoryBytes`, `MaxPIDs`, `CPUMilliPerSec`, `CgroupRoot`, and Result fields `PeakMemoryBytes`, `CPUTime`, `OOMKilled`, `PeakPIDs`, `PIDLimitHit`), `args.go` (cgroup flags with swap off, `/dev` nodes), `run.go` (`Run` owns the cgroup lifecycle around `runJob`; cancellation kills the whole cgroup; a cgroup that cannot be emptied becomes `ErrSandbox`), and tests `cgroup_test.go` (stats parsing and tree removal, no root needed) and `cgroup_run_test.go` (memory, process, whole-cgroup kill, CPU time).

**Mistakes made and caught during the unit.**
- A first `sed` edit of the removal loop produced an infinite loop with no exit; I replaced the block with a proper edit. The removal also briefly reused the first loop's deadline, which would have skipped removal and returned a nil error; it now has its own deadline.
- Lint (`gosec`, `staticcheck`, `noctx`) flagged 8 items across two rounds (directory permissions, a uint64 to int64 conversion, file paths from variables, a subprocess without a context); fixed with `0o750` directories, a bounded conversion, and documented `//nolint:gosec` for fixed paths and test inputs.

**Verification (EC2 host).** `make fmt lint`: `0 issues.` `make test`: ok. `make test-sandbox`: all PASS, including `TestRunMemoryLimit` (30 MiB runs clean with peak at least 30 MiB; 300 MiB under a 64 MiB limit is OOM-killed and the peak stays within 110% of the limit), `TestRunProcessLimit` (pid limit hit, peak at most 20, fork refused), `TestRunKillsWholeCgroup` (no process and no `job-*` cgroup left), `TestRunCPUTimeMeasured`. 25 repeated rounds of the cgroup tests passed in 34.7 s, leaving 0 job cgroups and 0 nsjail processes; host free memory stayed about 500 MiB available.

**Open items.** The 400 MiB backstop on `/sys/fs/cgroup/leetforce` is set by the tests only; a production runner must set it itself (Phase 3 / Phase 12). The parent directory is created under the root cgroup outside systemd's tree; this works on the dev host, and Phase 6/12 should decide whether to run it under a systemd slice with `Delegate=yes`. The seccomp denylist's effect is still untested (unit `test/1-adversarial`).

**Process incident (unit 3 commits).** The two planned `feat(sandbox)` commits were rejected by commitlint because their body lines were longer than 72 characters (`body-max-line-length`). My shell script did not stop on the failure, so the files stayed staged and the next command's docs commit (`f1657fc`, "log unit 3 cgroup limits work") included all eight code files. The code and tests in that commit are exactly the tree verified on the host (fmt, lint, test, test-sandbox and the 25-round stress run), so nothing is wrong functionally, but the commit mixes code and docs, which CLAUDE.md forbids. The branch had already been merged and pushed, and phase branches must never be force-pushed, so the history is left as is. Corrective rules for the rest of the phase: commit messages keep body lines to 72 characters or fewer; commit commands are chained with `&&` so a rejected commit stops the sequence; every commit is checked with `git log --stat -1` before the merge.

### Host state added by unit 3 (found in a completeness check)
- **Persistent cgroup directory.** `/sys/fs/cgroup/leetforce` exists on the host. The sandbox tests create it (`requireNsjail` / `capHostCgroup`) and set `memory.max` to 400M, `memory.swap.max` to 0 and `cgroup.subtree_control` to `+memory +pids +cpu` on it. It is empty between runs (no `job-*` directories) and does not survive a reboot (cgroupfs is in memory). The spike scripts removed their own copy with `rmdir` ("cleaned"); the tests recreate it. Remove manually with `sudo rmdir /sys/fs/cgroup/leetforce` while it is empty.
- **More scratch files in `/tmp/spike/`** (not in the repo, safe to delete with `rm -rf /tmp/spike`): `alloc.c`, `alloc`, `forker.c`, `forker`, the scripts `cg1.sh`, `cg2.sh`, `cg3.sh`, and 60 `l1.txt` to `l60.txt` nsjail logs from the setup-failure loop (`p3.sh`).
- **Test binaries.** The tests compile C programs into `/var/tmp/lf-sandbox-*` and remove them afterwards; none were left after the runs.
- **Host checkout after the merge.** After unit 3 was merged I ran `git checkout -- .` and `git clean -fdq judge Makefile` on the host to discard the files copied with `tar`, then `git fetch`, `git checkout phase/1-sandbox-core`, `git pull --ff-only`. The host is at the merge commit with a clean working tree.
- **No kernel or system settings were changed** beyond the cgroup directory above.

### `feat/1-result-channel` (2026-10-02)
Same workflow as units 2 and 3 (edit on Windows, `tar | ssh` to the host, test as root, commit after everything passed). This time the log was written during the unit.

**Spike on the host.** Script `/tmp/spike/fd4.sh` (run as root) started nsjail with `--log_fd 3 --pass_fd 4` redirected to files and a shell that wrote to fd 4 and fd 3. Result: the write to fd 4 reached the file (`via-fd4`); the write to fd 3 failed with `Bad file descriptor`; `/proc` is not mounted so `ls /proc/self/fd` failed as expected. So `--pass_fd` keeps exactly the one extra descriptor open and the nsjail log descriptor stays closed to the program.

**Design.**
- A second pipe is passed as `ExtraFiles[1]`, which is fd 4 in nsjail (`ResultFD = 4`), kept open for the program with `--pass_fd 4`. The host reads it separately from stdout and stderr into a capped buffer (`Limits.MaxResultBytes`, default 1 MiB). Exceeding the cap discards the excess and kills the whole run, like an output flood (`Result.OutputExceeded`).
- `Result.ResultData` is data from inside the sandbox, never a verdict. Exit status, signal, wall time, CPU time, peak memory, OOM kills and pid-limit hits remain host-measured (nsjail log on fd 3 and cgroup files) and are the only facts to trust.
- Known limit, to be written into ADR 0004: a harness that runs in the same process as user code (Python, Java drivers in Phase 2) cannot keep fd 4 secret from that code, so user code can forge the *content* of ResultData. That is acceptable only because the content is the program's output data; Phase 2 must compare outputs on the host side with the checker and never let ResultData decide a verdict or override the measured facts.

**Code.** `spec.go` (`MaxResultBytes`, `Result.ResultData`, `OutputExceeded` now also covers the result fd, validation), `args.go` (`ResultFD`, `--pass_fd`), `run.go` (second pipe, `copyAsync` and `drain` helpers shared by the log and result readers), tests in `unit_test.go` and the new `result_test.go`.

**Tests added (run as root via `make test-sandbox`).** `TestRunResultFD` (result, stdout and stderr stay separate); `TestRunForgedResultsDoNotChangeOutcome` (five cases: fake verdict JSON on stdout and stderr with real exit 3; a fake nsjail "exited with status: 0" line on stdout and stderr with real exit 4; the same fake line written to fd 3, which fails, with real exit 5; a success claim on fd 4 with real exit 6; nothing written to fd 4 gives empty ResultData); `TestRunResultFDCap` (`yes >&4` is cut at 1000 bytes and stopped promptly); `TestRunBackgroundResultHolderIsCleanedUp` (a background `sleep` holding fd 4 does not hold the run open, and the process and cgroup are gone afterwards).

**Verification (EC2 host).** `make fmt lint`: `0 issues.` `make test`: ok. `make test-sandbox`: all PASS. 25 rounds of the result, forgery, background, basics, memory and kill tests: ok in 34.7 s. After the runs, with root: 0 `job-*` cgroup directories, 0 processes in `/sys/fs/cgroup/leetforce`, `nr_descendants 0`; `pgrep` found 0 nsjail and 0 sleep processes; no `/var/tmp/lf-sandbox-*` directories; host memory unchanged (about 490 MiB available). A first leftover check printed 0 only because a non-root `ls` of the root-only cgroup directory was denied; I repeated it with sudo to be sure.

**Host state changes in this unit.** Only `/tmp/spike/fd4.sh`, `fd4log.txt` and `fd4out.txt` (scratch). No system settings changed.

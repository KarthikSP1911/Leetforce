# Phase 1: Sandbox core

**Branch:** `phase/1-sandbox-core`
**Range:** `phase-1-start..phase-1-done`
**Dates:** 2026-10-02 to 2026-10-02 (one session)
**Milestone:** none (M1 is the end of Phase 2)

The step-by-step working log (environment setup, per-unit commands, spikes, mistakes) is in [phase-1-log.md](phase-1-log.md). The plain-language explainer is [phase-1-summary.md](phase-1-summary.md).

## Summary
Phase 1 built the sandbox core: `sandbox.Run` in `judge/sandbox` runs an untrusted program inside nsjail with cgroup v2 memory, process and CPU limits, kills the whole cgroup on any timeout or breach, and reports host-measured facts (exit status, signal, wall and CPU time, peak memory, OOM and process-limit hits) apart from the program's output and a dedicated result descriptor. A 32-test suite, including 11 adversarial containment tests (fork, memory and output bombs, network and file-system escapes), passes on an Ubuntu 24.04 x86 EC2 dev host. This is the safety foundation every later phase relies on.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| Fork bomb contained | ✅ | `make test-adversarial` → `TestAdversarialForkBomb` PASS (pid limit hit, peak at most 64, no process left); `TestAdversarialThreadBomb` PASS |
| Memory bomb contained | ✅ | `TestAdversarialMemoryBomb` PASS (OOM-killed, peak within 110% of the 64 MiB limit); `TestAdversarialAggregateMemory` PASS (8 x 20 MiB); `TestAdversarialTmpfsAndFileSizeLimits` PASS |
| Infinite loop contained | ✅ | `TestAdversarialInfiniteLoops` PASS (5 sub-tests: busy loop, sleep, ignored signals, CPU limit, infinite stdin) |
| Output flood contained | ✅ | `TestAdversarialOutputFlood` PASS (4 sub-tests, capped at 4096 bytes, stopped in under 5 s) |
| Network access contained | ✅ | `TestAdversarialNetwork` PASS: TCP, UDP, DNS and the cloud metadata address blocked; a real host listener accepted 0 connections |
| File-system escape attempts contained | ✅ | `TestAdversarialFilesystemAndPrivilegeEscape` PASS (about 40 attempts, all denied, runs as uid 65534) |
| `make test-adversarial` passes | ✅ | Clean checkout of the merged phase branch on the host: `make test-adversarial` exit 0, 32 PASS, 0 FAIL, 0 SKIP; 5 full runs in total, identical |
| Whole-run cleanup (not in PLAN, from CLAUDE.md) | ✅ | `TestRunKillsWholeCgroup`, `TestAdversarialOrphansAreKilled`, `TestAdversarialZHostSurvives` PASS; after runs 0 `job-*` cgroups, 0 nsjail processes |
| Result over a dedicated channel (not in PLAN, from CLAUDE.md) | ✅ | `TestRunResultFD`, `TestRunForgedResultsDoNotChangeOutcome` (5 cases) PASS |
| `make fmt lint` clean | ✅ | `golangci-lint` output `0 issues.`; `go vet -tags adversarial ./...` clean |

Extra evidence: weakening network isolation on a copy of `args.go` (adding `--disable_clone_newnet`) made `TestAdversarialNetwork` and `TestNsjailArgs` fail; the file was then restored with `git checkout`. All 54 OOM events in the host `dmesg` were `CONSTRAINT_MEMCG` (cgroup-local) and none was host-wide.

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `docs/1-dev-environment` | ADR 0003, host setup script, README cost table | 2 |
| `feat/1-go-workspace` | `go.work`, `judge` module, lint config, Makefile, `.env.example` | 2 |
| `feat/1-nsjail-wrapper` | Spec, nsjail arguments, output cap, nsjail log parser, `Run` | 3 |
| `feat/1-cgroup-limits` | per-run cgroup v2 lifecycle, memory/pids/CPU limits, whole-cgroup kill | 2 (see Known issues: one commit also carries the code) |
| `feat/1-result-channel` | dedicated result fd (fd 4), forged-result tests | 3 |
| `test/1-adversarial` | adversarial suite, `make test-adversarial` in a memory-capped scope | 3 |
| `docs/1-adrs-report` | ADR 0004, report, summary, review record | see Stats |

Six commits were made directly on `phase/1-sandbox-core` (progress, logging rule, host log, missing log entries, the unit 3 incident note, host state log).

## File-by-file changes
Generated with `git diff --name-status phase-1-start..HEAD` and `--numstat` at the tip of `docs/1-adrs-report` (before the review Q&A commit); lines are added/deleted.

### Added
| File | Purpose |
|---|---|
| `.env.example` | Template for local settings; `LEETFORCE_REDIS_URL` left blank (4/0) |
| `.golangci.yml` | golangci-lint v2: standard linters plus errorlint, gosec, misspell, noctx, unconvert; gofmt and goimports (15/0) |
| `Makefile` | `fmt`, `lint`, `test`, `test-sandbox`, `test-adversarial` (adversarial binary built as the user, run as root in a `systemd-run` scope capped at 600M memory, no swap, 1500 tasks) (37/0) |
| `go.work` | Go workspace listing `./judge` (ADR 0002) (3/0) |
| `judge/go.mod` | Module `leetforce/judge` (3/0) |
| `judge/sandbox/doc.go` | Package comment (3/0) |
| `judge/sandbox/spec.go` | `Spec`, `Limits` (wall, CPU, file, fds, output, result, tmpfs, memory, pids, CPU quota), `Result` with host-side measurements (116/0) |
| `judge/sandbox/args.go` | Builds the nsjail command line: namespaces, uid 65534 mapped on the host, read-only mounts, `/dev` nodes, tmpfs `/tmp`, rlimits, seccomp denylist, cgroup flags, log fd 3, result fd 4 (133/0) |
| `judge/sandbox/capture.go` | `cappedBuffer`: bounded stdout, stderr, result and log capture that signals the first overflow (57/0) |
| `judge/sandbox/log.go` | Parses nsjail's own log: exit status, signal, time-limit kill, setup errors, and the `Executing` marker meaning the program started (54/0) |
| `judge/sandbox/cgroup.go` | Per-run cgroup directory: create, `cgroup.kill`, read `memory.peak`, `cpu.stat`, `memory.events`, `pids.peak`, `pids.events`, wait for empty, remove (174/0) |
| `judge/sandbox/run.go` | `Run` (cgroup lifecycle and error mapping) and `runJob` (starts nsjail, pipes, caps, cancellation); `ErrSandbox` (192/0) |
| `judge/sandbox/unit_test.go` | Table tests: spec validation, nsjail arguments, capped buffer, log parsing, seconds rounding (193/0) |
| `judge/sandbox/run_test.go` | Real-program tests: output, stdin, exit codes, unprivileged user, read-only `/usr`, no host files, wall limit, output cap, no network, log fd not inherited, setup failure (165/0) |
| `judge/sandbox/cgroup_test.go` | Stats parsing and cgroup tree removal, no root needed (68/0) |
| `judge/sandbox/cgroup_run_test.go` | Memory limit, process limit, whole-cgroup kill, CPU time; helpers `compileC`, `requireNoProcess`, `requireNoRunCgroups` (185/0) |
| `judge/sandbox/result_test.go` | Result fd, five forged-result cases, result cap, background fd holder (115/0) |
| `judge/sandbox/adversarial_test.go` | The 11 adversarial tests and their C attack programs (451/0) |
| `scripts/setup-dev-host.sh` | Idempotent dev-host provisioning: packages, swap, Go, nsjail, golangci-lint (55/0) |
| `docs/adr/0003-dev-environment-ec2-x86.md` | Decision record: EC2 x86 dev host instead of WSL2 (25/0) |
| `docs/adr/0004-sandbox-design.md` | Decision record: sandbox design with measured numbers (38/0) |
| `docs/phases/phase-1.md` | This report |
| `docs/phases/phase-1-log.md` | Step-by-step working log of the phase |
| `docs/phases/phase-1-summary.md` | Plain-language explainer, review questions, Q&A, handoff |

### Modified
| File | What changed | Why |
|---|---|---|
| `CLAUDE.md` | Real commands replaced the planned ones (including `test-sandbox` and `RUN=`); "Current state" updated; new rule "Documenting every step"; log file named `phase-<N>-log.md` (25/5) | Keep the repo guidance true, and make step logging mandatory |
| `README.md` | Added a cost table (EC2 t3.micro, 15 GiB EBS, public IPv4) (10/0) | CLAUDE.md requires recording new recurring costs |
| `docs/PLAN.md` | Removed a stray character on line 1 (1/1) | Typo from the Phase 0 draft |
| `docs/PROGRESS.md` | Phase 1 status and resume point kept current through the phase (5/4) | Required session state |

### Deleted
| File | Reason |
|---|---|
| none | |

### Renamed / moved
| From | To | Reason |
|---|---|---|
| `docs/phases/phase-1.md` (working draft) | `docs/phases/phase-1-log.md` | Free `phase-1.md` for the formal report; the log is kept (git shows the final `phase-1.md` as a new file, the log is the original content) |

## Key code changes
1. **A run only counts if nsjail logged that it started** (`judge/sandbox/log.go`, `run.go`). A failed jail setup sometimes ends with `terminated with signal: SIGKILL` instead of `exited with status: 255` (seen about 1 time in 5), so exit codes alone could report a host failure as a program result. `parseLog` records the `Executing '...'` line and `runJob` returns `ErrSandbox` when it is missing:
   ```go
   if !term.started {
       detail := strings.Join(tail(term.failure, 5), "; ")
       if detail == "" {
           detail = lastLines(logBuf.Bytes(), 3)
       }
       return nil, fmt.Errorf("%w: program did not start: %s", ErrSandbox, detail)
   }
   ```
2. **The host owns the cgroup; nsjail's child lives inside it** (`cgroup.go`, `run.go`). `Run` creates a job directory, lets nsjail create its limited child there, reads totals from the job directory afterwards, and always kills and removes it. If it cannot be emptied, `Run` fails instead of leaking processes:
   ```go
   defer func() {
       if rerr := job.remove(); rerr != nil {
           res = nil
           err = errors.Join(err, fmt.Errorf("%w: %w", ErrSandbox, rerr))
       }
   }()
   ```
3. **Two dedicated descriptors** (`args.go`, `run.go`). nsjail's log is fd 3 (closed before the program starts; a program writing to it gets `EBADF`); the harness result is fd 4 (`--pass_fd 4`). Both are read by the host into separate capped buffers; stdout and stderr are never parsed for a result.
4. **Security hardening found by probing** (`args.go`): `--user 65534:65534:1` instead of `--user 65534` (the latter mapped the jailed user to host root); `/dev/null`, `/dev/zero`, `/dev/urandom` mounted because runtimes need them; no `RLIMIT_NPROC` (per host user, not per run) so the process cap is cgroup `pids.max`.
5. **A test helper that cannot confuse the host with a leak** (`cgroup_run_test.go`): `requireNoProcess` flags a process only if its command line matches and it is still in the `leetforce` cgroup tree.

## Decisions
- [ADR 0002](../adr/0002-go-module-layout.md) (Phase 0): one Go module per component with `go.work`; used here for `judge`.
- [ADR 0003](../adr/0003-dev-environment-ec2-x86.md): Ubuntu 24.04 x86 EC2 t3.micro dev host, not WSL2; nsjail runs as root.
- [ADR 0004](../adr/0004-sandbox-design.md): nsjail isolation, host-owned cgroup with measured totals, nsjail log and result on dedicated descriptors, host-measured facts only.

## Tests
- 32 test functions in 6 files in `judge/sandbox`: unit tests (`unit_test.go`, `cgroup_test.go`), real-program tests (`run_test.go`, `cgroup_run_test.go`, `result_test.go`), and the adversarial suite (`adversarial_test.go`, build tag `adversarial`).
- Commands, run on the Linux dev host (see ADR 0003):
  ```bash
  make fmt lint
  make test                 # unit tests; real-program tests skip without root
  make test-sandbox         # real programs in nsjail (sudo -n)
  make test-adversarial     # everything, including the adversarial suite, in a memory-capped systemd scope
  make test-adversarial RUN=TestAdversarialForkBomb
  ```
- Final gate result on the clean merged checkout: exit 0, 32 PASS, 0 FAIL, 0 SKIP.

## Known issues and deferred work
- **CPU time-limit kills look like SIGKILL** (nsjail sets soft and hard `RLIMIT_CPU` equal). A judge must classify TLE from the measured `CPUTime`, not from the signal. Moves to **Phase 2**.
- **`ResultData` is forgeable in content** by a harness that shares a process with user code. Phase 2 must compare outputs on the host and never let `ResultData` decide a verdict. **Phase 2.**
- **The seccomp denylist is not tested alone**; its denials overlap with the unprivileged user namespace. A seccomp-only test and a review of running as root move to **Phase 6**.
- **Production cgroup parent**: the tests put a 400 MiB backstop on `/sys/fs/cgroup/leetforce`; a real runner must set its own, and a systemd slice with `Delegate=yes` is decided in **Phase 3 / Phase 12**.
- **Commit `f1657fc`** (`docs(docs): log unit 3 cgroup limits work`) also contains all the unit 3 code, because two feature commits were rejected by commitlint and the next commit swept in the staged files. It is merged and pushed and phase branches are never force-pushed, so it stays; recorded in the log with corrective rules.
- **Untracked `web/AGENTS.md` and `web/CLAUDE.md`** predate this phase and were left untouched (decision pending, default: not committed).
- **PLAN.md review**: the Phase 1 section was treated as approved for this session; the owner's formal review of the draft plan is still open.
- **Host scratch files** in `/tmp/spike/` and an empty `/sys/fs/cgroup/leetforce` remain on the dev host (listed in the log); no repo impact.
- The dev instance is billable while running (README cost table); stop it when idle.

## Stats
Measured with the git commands from CLAUDE.md section 7.1 at the tip of `docs/1-adrs-report`, before the review Q&A commit and the merge to `main` (those add a few commits and change only files already listed).
- Range: `phase-1-start..HEAD`
- Commits: see the final numbers in the summary's Handoff section (the count includes the report commits themselves)
- Files: 25 added, 4 modified, 0 deleted (generated list above is authoritative)
- Lines: see `git diff --shortstat phase-1-start..HEAD`

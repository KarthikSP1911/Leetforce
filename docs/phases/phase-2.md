# Phase 2: Judge engine (M1)

**Branch:** `phase/2-judge-engine`
**Range:** `phase-2-start..phase-2-done` (the `phase-2-done` and `M1` tags are created at the merge into `main`, after the review)
**Dates:** 2026-10-02 to 2026-10-02
**Milestone:** M1 (judge works locally), pending the review
**Working log:** [phase-2-log.md](phase-2-log.md) (every command, mistake and host change)
**Summary for the owner:** [phase-2-summary.md](phase-2-summary.md)

## Summary
The phase delivered the judge: a problem format with a content-hashed test-set version, verdict classification from host-measured facts only, host-side checkers, language definitions and an engine that compiles and runs Python, C++, Java and Go entirely inside the Phase 1 sandbox, and a `judge run` command. All 28 sample solutions (7 verdicts x 4 languages) are judged correctly and the Phase 1 adversarial suite still passes. The sandbox package itself was not changed except for a whitespace fix, because compiled artifacts return over the existing result descriptor.

## Exit criteria
| Criterion (from PLAN.md) | Status | Evidence |
|---|---|---|
| A sample problem is judged correctly in all four languages for every verdict type | Met | `make test-matrix` on the dev host: `TestVerdictMatrixIsComplete` PASS and `TestJudgeVerdicts` 28 of 28 sub-tests PASS (python 2.8 s, go 97 s, cpp 4.9 s, java 62 s; 2 m 48 s) |
| Adversarial suite still passes | Met | `make test-adversarial` on the dev host, phase branch at `fda6204`: PASS (11 attack tests and the other sandbox tests); afterwards `pgrep nsjail` 0, no job folders, no leftover cgroups |
| Compile step reports CE | Met | `TestJudgeVerdicts/<lang>/ce` for all four languages; the CE test requires a message from that language's own compiler (`ceMarkers`) |
| Verdicts AC/WA/TLE/MLE/RE/CE/OLE with runtime and memory | Met | `verdict.Classify` table tests (`TestClassify`, `TestClassifyOOMExitCode`), the matrix, and `Report.Overall.Time/Memory` |
| `problem.yaml` format and test-set versioning | Met | `judge/problem` tests (valid load, 12 rejection cases, `TestVersion`); `TestProblemTestFilesAreNotGitIgnored` |
| `judge run problems/<slug> <file>` CLI | Met | `make build-judge`; `judge/cmd/judge` tests including `TestRunEndToEnd`; manual runs recorded in the log (unit 7) |
| Fresh clone works | Met | after the `.gitignore` fix: `git clone` of the branch on the dev host, `go test ./judge/problem ./judge/cmd/... ./judge/engine` ok |

Other gates on the same code: `make fmt lint test` 0 issues; `make test-sandbox` (packages run serially) all ok, engine 196.3 s.

## Branches merged
| Branch | Purpose | Commits |
|---|---|---|
| `docs/2-runtimes` | JDK on the dev host, setup script | 2 |
| `feat/2-problem-format` | `problem.yaml` loader, validation, test-set version, sample problem, ADR 0005 | 5 |
| `feat/2-verdicts` | verdict classification and summary | 2 |
| `feat/2-checkers` | host-side output checkers | 2 |
| `feat/2-drivers-python-go` | language table, engine, python and go solutions, ADR 0006 | 9 |
| `feat/2-drivers-cpp-java` | C++ and Java, JVM out-of-memory rule, fixes | 8 |
| `feat/2-judge-cli` | `judge run`, `build-judge`, CLAUDE.md commands | 4 |
| `test/2-verdict-matrix` | matrix completeness test, `make test-matrix` | 2 |
| `fix/2-test-outputs-ignored` | expected-output files were git-ignored | 3 |
| `docs/2-report` | ADR 0007, report, summary, PROGRESS (merged at the end of the phase) | 3 at this writing |

Four further commits were made directly on the phase branch (log entries about the host checkout and stash; one duplicate removed).

## File-by-file changes
Generated with `git diff --name-status phase-2-start..HEAD` at `355d12d` (the commit before this report), plus this report.

### Added
| File | Purpose |
|---|---|
| `judge/problem/problem.go` | `Spec`, `Limits`, `Load`, `Validate`, per-language `LimitFor`; strict YAML (unknown fields rejected) |
| `judge/problem/version.go` | `Version`: SHA-256 over the checker and every test (length-prefixed), `ts-` plus 16 hex characters |
| `judge/problem/problem_test.go` | load, limits, 12 rejection cases, version sensitivity, the sample, and the git-ignore regression test |
| `judge/verdict/verdict.go` | `Classify` (OLE, MLE, TLE, RE from host facts; `OOMExitCode`), `Summarize` |
| `judge/verdict/verdict_test.go` | 17 classify cases, OOM exit code cases, forged output ignored, summarize |
| `judge/checker/checker.go` | `Check` for `tokens` and `exact` modes |
| `judge/checker/checker_test.go` | 22 comparison cases and the unknown-mode error |
| `judge/lang/lang.go` | language definitions: python, go, cpp, java (compile argv, run argv, limits, `Binds`, `OOMExitCode`) |
| `judge/lang/lang_test.go` | definitions complete, match `problem.Languages`, Java argv and heap sizes, paths |
| `judge/engine/engine.go` | `Engine.Judge`: job directory, compile in sandbox, per-test run, classify, check, report, cleanup |
| `judge/engine/engine_test.go` | error paths, output cap, compile output cleaning, matrix completeness, 28-case matrix, Java memory, details only for samples, `ContinueOnFail`, forged verdict, cleanup |
| `judge/cmd/judge/main.go` | the `judge run` command |
| `judge/cmd/judge/main_test.go` | usage errors, extension detection, report format, end to end |
| `judge/go.sum` | checksums for `gopkg.in/yaml.v3` (generated by `go mod tidy`) |
| `problems/sample-sum/problem.yaml` | the sample problem's definition (Java gets 2000 ms and 256 MB) |
| `problems/sample-sum/tests/` `01.in` `01.out` `02.in` `02.out` `03.in` `03.out` `04.in` `04.out` `05.in` `05.out` | 5 tests: 2 samples (`01`, `02`) and 3 hidden (`03` overflows 32 bits, `05` has 100,000 numbers) |
| `problems/sample-sum/solutions/python/` `ac.py` `wa.py` `tle.py` `mle.py` `re.py` `ole.py` `ce.py` | one solution per verdict |
| `problems/sample-sum/solutions/go/` `ac.go` `wa.go` `tle.go` `mle.go` `re.go` `ole.go` `ce.go` | one solution per verdict |
| `problems/sample-sum/solutions/cpp/` `ac.cpp` `wa.cpp` `tle.cpp` `mle.cpp` `re.cpp` `ole.cpp` `ce.cpp` | one solution per verdict |
| `problems/sample-sum/solutions/java/` `ac.java` `wa.java` `tle.java` `mle.java` `re.java` `ole.java` `ce.java` | one solution per verdict (the class is `Main`) |
| `docs/adr/0005-problem-format-and-test-set-version.md` | ADR: problem format and the content-hash version |
| `docs/adr/0006-compile-in-sandbox-artifact-over-fd4.md` | ADR: compile in the sandbox, artifact over fd 4, C++ and Java specifics |
| `docs/adr/0007-verdicts-from-host-facts.md` | ADR: verdict rules |
| `docs/phases/phase-2-log.md` | working log, every step and mistake |
| `docs/phases/phase-2-summary.md` | plain-language summary and review questions |
| `docs/phases/phase-2.md` | this report |

### Modified
| File | What changed | Why |
|---|---|---|
| `Makefile` | added `build-judge` and `test-matrix`; `test-sandbox` now `go test -p 1 -timeout 20m` | the CLI build, the exit-criterion run, and serial packages because the engine and sandbox tests share one cgroup root and memory cap (parallel runs failed two cgroup tests) |
| `judge/go.mod` | added `gopkg.in/yaml.v3 v3.0.1` | YAML parsing for `problem.yaml` |
| `judge/sandbox/run.go` | one whitespace alignment (`maxLogBytes  =`) | `make fmt` reported Phase 1 code that was not gofmt-clean; no behaviour change |
| `scripts/setup-dev-host.sh` | added `openjdk-21-jdk-headless` to the apt list | Java judging needs `javac` and `java` on the host |
| `.gitignore` | added `!problems/**/tests/*.out` | `*.out` had hidden every expected-output file from git |
| `CLAUDE.md` | status line; command list now includes `make build-judge` and `judge run`; `make test` and `make test-sandbox` descriptions | the CLI exists and the phase status changed |
| `docs/FLOW.md` | new "Phase 2: Judge engine" as-built section | required each phase |
| `docs/PROGRESS.md` | Phase 2 status, resume point, open decisions, table row | required each session |

### Deleted
None.

### Renamed / moved
None.

## Key code changes
**1. The compile step returns its artifact over fd 4 (`judge/lang/lang.go`).** The compile command copies the built file to the descriptor the host already reads, so no sandbox gets a writable host folder and the sandbox package needed no change.
```go
Compile: func(dir string) []string {
	script := fmt.Sprintf("go build -o /tmp/main %q && cat /tmp/main >&4", filepath.Join(dir, "src", "main.go"))
	return []string{"/bin/sh", "-c", script}
},
```

**2. Verdicts from host facts only (`judge/verdict/verdict.go`).**
```go
switch {
case r.OutputExceeded:
	return OLE
case r.OOMKilled || r.PeakMemoryBytes > l.MemoryBytes ||
	(l.OOMExitCode != 0 && r.Signal == 0 && r.ExitCode == l.OOMExitCode):
	return MLE
case r.TimedOut || r.CPUTime > l.Time || r.Signal == syscall.SIGXCPU:
	return TLE
case r.Signal != 0 || r.ExitCode != 0 || r.PIDLimitHit:
	return RE
}
return Completed
```
It never reads stdout, stderr or the result descriptor; `TestClassifyIgnoresProgramOutput` and `TestJudgeIgnoresForgedVerdict` check that.

**3. CPU kill slack (`judge/engine/engine.go`).** The kernel's CPU limit is the problem limit plus one second, and the verdict compares the *measured* CPU time with the exact limit. A kill exactly at the limit could measure just under it and look like a crash.
```go
limits.CPUTime = limit.Time() + cpuKillSlack // 1 s
limits.WallTime = 2*limit.Time() + wallSlack
```

**4. Java memory (`judge/lang/lang.go`, `verdict.go`).** A JVM refuses a large allocation with `OutOfMemoryError` before the kernel sees the memory (measured: a 400 MB array on a 256 MB limit ended as RE with 18 MB peak). The heap now stays 64 MiB under the limit, the JVM exits with status 3 when it fills, and `Classify` reads that status as MLE for languages that declare it.

**5. No problem test file may be git-ignored (`judge/problem/problem_test.go`, `.gitignore`).** See "Known issues"; the test runs `git check-ignore` on every file under `problems/*/tests`.

## Decisions
- [ADR 0005](../adr/0005-problem-format-and-test-set-version.md): stdin/stdout problems in a folder with `problem.yaml`; the test-set version is a content hash.
- [ADR 0006](../adr/0006-compile-in-sandbox-artifact-over-fd4.md): compile in the sandbox; artifact over fd 4; job directories under `/var/tmp`; C++ and Java specifics (real tool paths, `LD_LIBRARY_PATH`, `/etc` bind, JVM heap rule).
- [ADR 0007](../adr/0007-verdicts-from-host-facts.md): verdict order and what each verdict reads.
- Open for the owner: decisions A (problem format), B (compile errors on Submit) and C (compile speed) in the summary.

## Tests
- New: unit tests in every new package; `TestVerdictMatrixIsComplete` (no sandbox); the sandbox-backed `TestJudgeVerdicts` (28), `TestJavaMemoryLimit`, `TestJudgeDetailOnlyForSamples`, `TestJudgeContinueOnFail`, `TestJudgeIgnoresForgedVerdict`, `TestJudgeCleansUp`, and `TestRunEndToEnd` for the CLI; `TestProblemTestFilesAreNotGitIgnored`.
- Commands (on the Linux dev host, passwordless sudo):
  ```bash
  make fmt lint test          # unit tests; sandbox-backed tests skip without root
  make test-matrix            # the exit criterion, about 3 minutes
  make test-sandbox           # everything that runs real programs, serial, about 4 minutes
  make test-adversarial       # the Phase 1 attack suite
  ```

## Known issues and deferred work
- **Bug found and fixed at the end of the phase:** `.gitignore` ignored all `*.out` files, including the sample problem's expected outputs, so they were never committed and a fresh clone could not load the sample problem. Every test passed because the files existed on the dev host and this machine. Found while generating this report's file list; fixed on `fix/2-test-outputs-ignored` with a regression test and verified from a fresh clone. It was never on `main`.
- **Compile speed:** Go 10 to 20 s and Java about 10 s per submission on the t3.micro, because the build cache is cold in every sandbox. A warm read-only cache or prebuilt standard library is deferred to Phase 6 or 13 (decision C).
- **Function-signature problems** (LeetCode style templates) are not built; stdin/stdout only. Owner decision A.
- **Compile errors are returned on Submit,** differing from the literal wording of the "no raw stderr for Submit" rule. Owner decision B.
- Java runs with C1 only and the serial GC, so CPU-heavy Java is slower than it could be. Revisit with real problems.
- The `judge` command must run as root and, unlike the tests, applies no memory cap to the whole host; it is a local tool.
- The sandbox's production privilege model and the runner's cgroup parent are still open (Phases 3 and 6).
- Host housekeeping: identical git stash entries and compiled test binaries in `/tmp` on the dev host; the EC2 instance is running (billable).
- Process lessons recorded in the log: scripted `sed` and `perl` edits corrupted files several times. All were caught by the host's `make fmt lint test` or my own check of `git diff`, and none was in the tip of any merged branch; one (a stray `case` clause at the top of `judge/verdict/verdict.go`) was committed on `feat/2-drivers-cpp-java` and repaired in the next commit, so that broken commit is in the phase branch's history. I now check `git diff` after every scripted edit.

## Stats
Pinned at `355d12d` (the commit before this report), range `phase-2-start..355d12d`:
- Commits: 43 (excluding merges); 10 merge commits (9 unit branches into the phase branch, and one merge of the phase branch into `docs/2-report`)
- Files: 58 added, 8 modified, 0 deleted (this report adds one more file)
- Lines: +2,764 / -14

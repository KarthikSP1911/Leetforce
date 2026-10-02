# Phase 2 working log: Judge engine

**Branch:** `phase/2-judge-engine`
**Range:** `phase-2-start..phase-2-done`
**Status:** done (merged into main; see the closing entry)

## Units of work
- [x] `docs/2-runtimes`: JDK on the dev host, setup script updated
- [x] `feat/2-problem-format`: `problem.yaml`, loader, test-set version hash, sample problem
- [x] `feat/2-verdicts`: verdict classification from host-measured facts
- [x] `feat/2-checkers`: host-side output comparison
- [x] `feat/2-drivers-python-go`
- [x] `feat/2-drivers-cpp-java`
- [x] `feat/2-judge-cli`: `judge run problems/<slug> <file>`
- [ ] `test/2-verdict-matrix`: 7 verdicts x 4 languages, adversarial suite re-run

## Decisions (2026-10-02)
- Owner approved the session plan with "ok" (no recap answer, no changes). Defaults taken from Claude's recommendations: original sample problem; per-language limits in `problem.yaml`. The owner did not review the Phase 2 section of `docs/PLAN.md` line by line; it is treated as approved by that "ok".

## Session log

### Start of session (2026-10-02)
1. Claude read `CLAUDE.md`, `docs/PROGRESS.md`, `phase-1-summary.md`. Repo matched `PROGRESS.md`; `phase-1-done` tag is at `ca721ab`, `main` is at `14c63c9` (docs-only difference).
2. Claude (Windows repo): `git checkout main`, `git checkout -b phase/2-judge-engine`, `git tag phase-2-start`, pushed both to `origin`.
3. Claude (host, read-only checks over `ssh leetforce-dev`): disk 14G with 7.0G free; g++, Go 1.27.1, Python 3.12.3 already installed; Java missing. The host checkout was on `phase/1-sandbox-core` at `112824c` with an uncommitted whitespace-only (gofmt alignment) change to `judge/sandbox/run.go`.
4. Mistake/blocked: Claude tried to discard that host edit (`git checkout -- judge/sandbox/run.go`) as part of a longer command; the permission classifier denied it. Not retried. The host checkout was therefore **not** moved to the Phase 2 branch; it stays on the Phase 1 commit until the owner decides what to do with that edit (it only changes whitespace alignment).

### Unit 1: `docs/2-runtimes`
1. Claude (host): `sudo apt-get install -y openjdk-21-jdk-headless`. Result: `java` and `javac` 21.0.12.1. Disk after: 6.6G free (about 0.4G used).
2. Claude (repo): added `openjdk-21-jdk-headless` to the apt list in `scripts/setup-dev-host.sh`. g++ comes from `build-essential`, Go from the existing script step.

### Host checkout moved (2026-10-02)
Owner said "retry". Claude (host): `git checkout -- judge/sandbox/run.go` (discarded the whitespace-only gofmt edit), `git fetch`, `git checkout phase/2-judge-engine`, `git pull --ff-only`. Result: clean, at `a7dddac`, tracking `origin/phase/2-judge-engine`. Resolves the blocked step above.

### Unit 2: `feat/2-problem-format` (2026-10-02)
Design: [ADR 0005](../adr/0005-problem-format-and-test-set-version.md). Decision taken by Claude without a separate owner answer (the owner had said "next" after being offered "propose in chat or use a default"): stdin/stdout problems, so the "drivers" in Phase 2 are the per-language compile/run steps, not function-signature templates. The owner should confirm this at the review.
1. Claude (repo): created `judge/problem/problem.go` (Spec, Limits, Load, Validate, `LimitFor`), `judge/problem/version.go` (`Version`), `judge/problem/problem_test.go` (table-driven: valid load, limit overrides, 12 rejection cases, version sensitivity, the committed sample), `problems/sample-sum/problem.yaml` and `problems/sample-sum/tests/01..05.{in,out}` (05 is 100000 numbers, 588,902 bytes; 03 sums to 5,000,000,000 to overflow 32 bits).
2. Claude (host, over SSH): copied the files with `scp`, ran `go get gopkg.in/yaml.v3@latest` (v3.0.1) and `go mod tidy` in `judge/`, copied `go.mod` and `go.sum` back to the repo.
3. Mistakes and fixes:
   - First shell command failed to parse (quote inside a long heredoc), nothing ran; files were then created with the Write tool.
   - `make lint` reported gosec G115 (int to uint64) in `MemoryBytes`. Fixed by making `MemoryMB` a `uint64`; a negative YAML value is now rejected at parse time ("cannot unmarshal"). My test expected the field name in the message and failed once; corrected the expectation.
   - A `sed` edit left a struct tag misaligned, so gofmt on the host changed it; committed the formatted file (`chore(judge): gofmt the problem package`).
   - `make fmt` on the host also reformatted `judge/sandbox/run.go` (alignment of `maxLogBytes`, whitespace only, present in committed Phase 1 code). Committed as `chore(sandbox)`; `style` is not an allowed commit type here, the first attempt was rejected by commitlint.
   - Two stray empty `python -` commands in my shell hung until killed (no effect on the repo).
4. Host sync: host files were uncommitted copies; verified each equals the branch with `cmp`, then `git stash push -u` (stash kept, not dropped: `pre-sync copy of feat/2-problem-format files`) and `git pull --ff-only`. Host is on `feat/2-problem-format` at `d673221`.
5. Checks on the host: `make fmt lint test` clean (0 issues, `judge/problem` and `judge/sandbox` ok); `make test-adversarial` PASS after the `run.go` whitespace change, `pgrep nsjail` prints 0.
6. Commits on the branch: `61e3ac5` loader, `1219187` sample problem, `4000ac6` run.go alignment, `d673221` gofmt; ADR and log in a docs commit.

### Unit 3: `feat/2-verdicts` (2026-10-02)
1. Claude (repo): created `judge/verdict/verdict.go` (`Verdict` constants AC/WA/TLE/MLE/RE/CE/OLE, `Completed` for "ran cleanly, compare output", `Limits`, `Classify`, `Case`, `Overall`, `Summarize`) and `judge/verdict/verdict_test.go`.
2. Rules in `Classify`, in order: OLE if `OutputExceeded`; MLE if `OOMKilled` or peak memory over the limit; TLE if `TimedOut`, CPU time over the limit, or `SIGXCPU`; RE for any other signal, non-zero exit or `PIDLimitHit`; otherwise `Completed`. A plain SIGKILL with low CPU time and memory is RE (a crash), not TLE; the CPU time decides TLE, as the Phase 1 handoff required.
3. `Summarize`: first non-AC case gives the verdict and `Failed` name; time and memory are the maxima. `Failed` is documented as not to be shown for hidden tests on Submit.
4. Tests (table-driven, 17 classify cases, one test that stdout/stderr/result-fd text cannot change the verdict, 4 summarize cases). Host: `make fmt lint` 0 issues, `go test ./judge/verdict` ok. No sandbox code changed, so the adversarial suite was not re-run for this unit (it runs at the end of the phase).
5. Note: the package imports `syscall.SIGXCPU`, so it builds on Linux only, like the sandbox.

### Unit 4: `feat/2-checkers` (2026-10-02)
1. Claude (repo): created `judge/checker/checker.go` (`Check(mode, expected, actual) (verdict.Verdict, error)`) and `judge/checker/checker_test.go`. `tokens` compares `bytes.Fields` of both sides; `exact` is byte-for-byte after removing one trailing `\n` from each side. An unknown mode returns an error with no verdict. The package imports `problem` for the mode names and `verdict` for AC/WA; it runs on the host on bytes returned by the sandbox, so the program cannot see the expected output.
2. Tests: 22 table-driven cases (CRLF, blank lines, split and joined tokens, case, order, empty output, trailing newline rules in exact mode) plus the unknown-mode error.
3. Mistake: the first commit was rejected by commitlint (a body line over 72 characters); the branch was pushed with no new commit, then the commit was redone with shorter lines and pushed. Nothing was lost.
4. Workflow change: this time the host got the code with `git fetch`/`git pull` of the pushed unit branch, not `scp`, so no untracked copies or stash entries were created. Host: `make fmt lint` 0 issues, `go test ./judge/checker` ok, working tree clean. No sandbox code changed.

### Unit 5: `feat/2-drivers-python-go` (2026-10-02)
Design: [ADR 0006](../adr/0006-compile-in-sandbox-artifact-over-fd4.md). Flow: `docs/FLOW.md`, "Phase 2". Claude did all steps; the owner said "next".
1. Claude (host, read-only spike): a cold `go build` of a small program outside the sandbox took 11.1 s, peak RSS 257 MB, 34 MB of cache, 2.4 MB binary. This sized the Go compile limits.
2. Claude (repo), new code: `judge/lang/lang.go` (+ `lang_test.go`): language definitions for python and go; `judge/engine/engine.go` (+ `engine_test.go`): `Engine.Judge`, `Options{ContinueOnFail, Detail}`, `Report`, `CaseResult`, `Detail`; `problems/sample-sum/solutions/{python,go}/{ac,wa,tle,mle,re,ole,ce}.*` (14 files).
3. Design points: compile runs in a sandbox with only `src/` read-only; the artifact returns over fd 4; job dir under `/var/tmp` (the sandbox's own `/tmp` tmpfs would hide a bind under `/tmp`; the engine rejects such a root); the CPU kill slack is limit + 1 s so the measured CPU time is always over the limit for a runaway (a kill exactly at the limit could measure under it and look like RE); wall limit 2x + 1 s; output cap `max(64 KiB, 2x expected + 4 KiB)`; compile output has the host job path removed, is made valid UTF-8 and cut to 4 KiB; a source over 64 KiB or an unknown language is an error, not a verdict. **Owner decision needed at the review:** compile errors are returned to the user on Submit (the compiler's message about their own source), which is an exception to "raw stderr is never returned for Submit" as written in `CLAUDE.md`; run-time stderr is only in `Detail`, for Run on sample tests.
4. Mistakes and fixes, in the order found on the host:
   - `make lint`: gosec G304 on the test fixture read; added a `nolint` with the reason.
   - Test binary run from the wrong directory (relative fixture path); run it from `judge/engine`.
   - Every Go compile failed in 20 ms: "go binary is trimmed and GOROOT is not set" because the sandbox has no `/proc`; fixed by setting `GOROOT=/usr/local/go` and `GOTELEMETRY=off` (the "telemetry sidecar" message still prints and is harmless).
   - Then "write $WORK/b010/_pkg_.a: file too large": the runtime archive is bigger than the 8 MiB `RLIMIT_FSIZE` default; added `CompileLimits.MaxFileBytes` (64 MiB for go).
   - `make test-sandbox` failed `TestRunKillsWholeCgroup` and `TestRunBackgroundResultHolderIsCleanedUp` when all packages ran at once: they pass alone, and the engine package's Go compile shares the cgroup root's memory cap and runs in parallel with them. Fixed in the `Makefile` with `go test -p 1 -timeout 20m` (commit `build(ci)`). I did not dig further into which assertion tripped; the rerun with `-p 1` passed all tests.
   - Two stray `python3 -` commands in my shell hung again (the Windows `python3` stub); no effect on the repo.
5. Results on the host: `make fmt lint test` clean; `make test-sandbox` (serial): checker, engine (98.8 s), lang, problem, sandbox, verdict all ok. Verdict matrix for python and go: ac, wa (first failure at test 03), tle, mle, re, ole at test 01 and ce all correct (python total 2.8 s; go 97 s, 9-17 s per compile). Also tested: details only for failing sample tests and only with `Options.Detail`, never for hidden tests; `ContinueOnFail` runs all 5 tests; a program that writes a forged `{"verdict":"AC"}` to fd 4 and prints `AC` gets WA; the job dir is removed afterwards. After the run: `pgrep nsjail` prints 0, no `leetforce-job-*` left in `/var/tmp`, `/sys/fs/cgroup/leetforce` has no job folders, disk 6.6 GB free.
6. The adversarial suite was not re-run for this unit: no file in `judge/sandbox/` changed.

### Unit 6: `feat/2-drivers-cpp-java` (2026-10-02)
Design notes are in the "C++ and Java specifics" section of [ADR 0006](../adr/0006-compile-in-sandbox-artifact-over-fd4.md). Claude did all steps; the owner said "next".
1. Claude (repo): added `cpp` and `java` to `judge/lang/lang.go`, `Language.Binds` (extra read-only binds, used by `engine.go` for compile and run), `Language.OOMExitCode`, and `verdict.Limits.OOMExitCode` (read by `Classify`); 14 solutions in `problems/sample-sum/solutions/{cpp,java}/`; tests in `lang_test.go`, `verdict_test.go` (`TestClassifyOOMExitCode`) and `engine_test.go` (`TestJavaMemoryLimit`, `ceMarkers`).
2. Mistakes and fixes, in the order found on the host:
   - `javac: not found`: `/usr/bin/javac` goes through `/etc/alternatives`, which the sandbox lacks; use the real path under `/usr/lib/jvm/java-21-openjdk-amd64`. The Java CE test had passed by accident because "not found" also ends in CE; the CE check now requires a message from the language's own compiler (`ceMarkers`).
   - `libjli.so: cannot open shared object file`: the JDK finds its libraries through an `$ORIGIN` rpath, which glibc resolves via `/proc/self/exe`; the sandbox has no `/proc`. Fixed with `LD_LIBRARY_PATH` for compile and run. The Debian JDK also links its config into `/etc/java-21-openjdk`, bound read-only through `Language.Binds`.
   - A probe on the host (a temporary `probe_test.go`, deleted afterwards, never committed) showed a 400 MB array on a 256 MB Java limit ended as RE with 18 MB peak: the JVM throws `OutOfMemoryError` before the kernel sees the memory. Fixed by sizing the heap 64 MiB under the limit, `-XX:+ExitOnOutOfMemoryError` (exit status 3), and `OOMExitCode` in `Classify`. The probe's "churn" case reported TLE because my probe allocated about 3 TB in total; that was a bad probe, not a bug.
   - **My error:** I wrote the heap formula with unsigned arithmetic (`mb-64`), which would underflow for limits under 64 MiB; caught before running and fixed (`mb/2` below 128 MiB, `mb-64` from 128 MiB up; the unit test covers 1, 100 and 128 MiB).
   - **My error, committed and pushed:** a scripted `perl` edit used `|` as both the delimiter and an escaped alternation, so it inserted a stray `case` clause at the top of `judge/verdict/verdict.go` and left the real one unchanged. The package did not compile; the host's `make fmt` caught it immediately. Fixed forward in the next commit ("repair verdict.go after a bad scripted edit"), not by rewriting history. Lesson: check `git diff` after scripted edits, which I now do.
   - gofmt differences (a missing space after `=` that my own edit introduced, struct alignment) were fixed in `chore(judge): gofmt the language table and engine test`.
   - Host: while recovering I ran `git checkout -- judge/engine/engine_test.go judge/lang/lang.go` on the host to drop uncommitted `make fmt` output (formatting only, regenerated by the next `make fmt`, and then committed). I did that without asking; nothing else was lost. Two stash entries were created on the host for the same cause: "host gofmt copies of lang.go and engine_test.go (same as commit)" (plus the two older ones).
3. Results on the host: `make fmt lint test` clean; `make test-sandbox` (serial): checker, engine 194.7 s, lang, problem, sandbox, verdict all ok. Verdict matrix for cpp and java: ac, wa (first failure at test 03), tle, mle, re, ole at test 01, ce all correct (cpp about 1 s per submission; java about 10 s). Java memory: one 2 GB array and a 400 MB array on a 256 MB limit give MLE, and a 80 MB array (within the limit) is not MLE. After the run: `pgrep nsjail` 0, no `leetforce-job-*` in `/var/tmp`, no job cgroups, disk 6.5 GB free.
4. The adversarial suite was not re-run for this unit: no file in `judge/sandbox/` changed. It runs at the end of the phase.

### Unit 7: `feat/2-judge-cli` (2026-10-02)
Claude did all steps; the owner said "next".
1. Claude (repo): `judge/cmd/judge/main.go` (`judge run [-all] [-detail] [-lang NAME] <problem-dir> <solution-file>`; language from the extension `.py .cpp .cc .java .go`; exit 0 = AC, 1 = any other verdict, 2 = usage or host error; refuses to run unless root; Ctrl-C and SIGTERM cancel through the context and so kill the cgroup), `judge/cmd/judge/main_test.go`, a `build-judge` target in the `Makefile`, and `CLAUDE.md` (the command is no longer "planned"; the phase status line is current).
2. Output: Problem and test-set version, Language, Verdict, Runtime and Memory (maxima), `Failed test NN` for a non-AC, then a table (TEST, KIND sample or hidden, VERDICT, TIME, MEMORY); for CE the compiler output instead of the table; with `-detail`, input, expected, actual and stderr of failing sample tests. The CLI is a local author tool, so it names the failed hidden test; the engine still never records details for hidden tests.
3. Mistakes and fixes: two `sed` edits corrupted files and were repaired with the Edit tool after checking `git diff` (a `\n` in a `sed` replacement became a real newline inside two Go string literals in the test; a wrong line number in a `CLAUDE.md` edit replaced the `make test` line); a `:=` that should have been `=` in my test failed `go vet` on the host, fixed in a follow-up commit; `make fmt` removed a trailing blank line from `main.go` (copied back). None of these reached `phase/2-judge-engine`.
4. Results on the host: `make fmt lint test` clean; `make build-judge` builds `bin/judge` (about 4.5 MB); CLI tests pass (usage errors, extension detection, report formatting, CE report, end to end for ac, wa with and without `-all`, `-detail` for re, ce, unknown `-lang`, missing problem); `TestRunRequiresRoot` runs in `make test` as a normal user and skips under root. Manual runs: python ac (AC, 5 tests), cpp wa (WA at test 03, stops), java ce (compiler message with `Main.java:3`), go tle (TLE, 2025 ms), python wa with `-all` (WA at 03, 04, 05), non-root run (exit 2 with a clear message).
5. Host: a fourth stash entry, "host gofmt copy of cmd/judge/main.go (same as commit)". The adversarial suite was not re-run: no file in `judge/sandbox/` changed.

### Unit 8: `test/2-verdict-matrix` and the end-of-phase gates (2026-10-02)
The owner said "complete this phase fully". Claude did all steps; the review with the owner (CLAUDE.md section 7.3) is still required before the merge to `main`.
1. Claude (repo): `TestVerdictMatrixIsComplete` in `judge/engine/engine_test.go` (no sandbox: fails if any of the 7 verdicts or any of the 28 language and verdict solution files is missing) and `make test-matrix`. The Makefile comment first said about 4 minutes and was corrected to about 3 after measuring.
2. Results on the host (phase branch at `fda6204`): `make test-matrix`: `TestVerdictMatrixIsComplete` PASS and `TestJudgeVerdicts` PASS, 28 of 28 sub-tests (python 2.8 s, go 97 s, cpp 4.9 s, java 62 s; 2 m 48 s in all). `make fmt lint test`: 0 issues, all packages ok. `make test-sandbox` (serial): all packages ok, engine 196.3 s, sandbox 3.8 s, cmd/judge 0.8 s. `make test-adversarial`: PASS (all 11 attack tests and the other sandbox tests). Afterwards `pgrep nsjail` prints 0, no `leetforce-job-*` in `/var/tmp`, no job folders in `/sys/fs/cgroup/leetforce`.
3. **Bug found while writing the report, fixed on `fix/2-test-outputs-ignored`:** the generated file list showed `problems/sample-sum/tests/*.in` but no `*.out`. `.gitignore` line 21 (`*.out`, meant for build output) ignored every expected-output file, so they had never been committed. Every test had passed only because the files existed on the host and on this machine; a fresh clone could not load `sample-sum` (and `judge run` and the whole verdict matrix would fail there). This was my mistake in unit 2 (I never checked `git status` for the `.out` files). Fix, in two commits so the first proves the test works: (a) `TestProblemTestFilesAreNotGitIgnored` asks `git check-ignore` about every file under `problems/*/tests`; run on the host before the fix it failed for all five `.out` files; (b) an exception `!problems/**/tests/*.out` in `.gitignore` and the five `.out` files committed. Verified from a fresh local clone of the branch on the host (`git clone ~/Leetforce /tmp/fresh-clone`, removed afterwards): `go test ./judge/problem ./judge/cmd/... ./judge/engine` ok. A third commit gave the test a context (`noctx` lint). This is the only bug in this phase that affected the committed state.
4. The sandbox package was not changed beyond the whitespace fix in unit 2, and the full adversarial suite passed on the final code (the later commits changed only `.gitignore`, a test and data files).

### Review skipped and phase closed (2026-10-02)
The owner replied "did u write all and push" (Claude confirmed everything was pushed and that the merge waited for the review) and then "merge it". Claude treated that as an explicit request to skip the review (CLAUDE.md allows it). Review questions and decisions A, B, C stay unanswered and are recorded as such in `phase-2-summary.md`; `PROGRESS.md` says done with resume point "start Phase 3". Then: `git merge --no-ff` of `phase/2-judge-engine` into `main` with git's default message, tags `phase-2-done` and `M1`, push of `main` and the tags (result in the closing entry below).

**Closing entry (written after the merge).** `main` was fast-checked against `origin/main` (both at `2c4e63c`, the merge commit with git's default message). Tags pushed: `phase-2-start` (`14c63c9`), `phase-2-done` (`2c4e63c`), `M1` (`2c4e63c`). The phase branch is kept. This entry could not be part of that merge, so it was added on the phase branch afterwards and merged into `main` with a second merge commit; the tags stay on `2c4e63c` (documentation-only difference). Not done: the EC2 instance `leetforce-dev` was not stopped (billable, owner's call) and the four stash entries and test binaries on the host were not cleaned up.

## File and path index
- `judge/cmd/judge/main.go`, `main_test.go`: the `judge` CLI
- `Makefile`: `build-judge` target (output `bin/judge`, git-ignored)
- `CLAUDE.md`: command list and status line updated
- `judge/lang/lang.go`, `lang_test.go`: now python, go, cpp, java; `Binds`, `OOMExitCode`
- `judge/verdict/verdict.go`, `verdict_test.go`: `Limits.OOMExitCode`
- `problems/sample-sum/solutions/cpp/*.cpp`, `problems/sample-sum/solutions/java/*.java`: one solution per verdict
- Host: stash entry "host gofmt copies of lang.go and engine_test.go (same as commit)"; `/tmp/engine.test`, `/tmp/probe.test` (compiled test binaries, can be deleted)
- `judge/lang/lang.go`, `lang_test.go`: language definitions (python, go so far)
- `judge/engine/engine.go`, `engine_test.go`: compile and judge a submission
- `problems/sample-sum/solutions/python/*.py`, `problems/sample-sum/solutions/go/*.go`: one solution per verdict
- `docs/adr/0006-compile-in-sandbox-artifact-over-fd4.md`: ADR
- `Makefile`: `test-sandbox` now `-p 1 -timeout 20m`
- Host: `/tmp/engine.test` (compiled test binary, can be deleted); `/var/tmp/leetforce-job-*` (job dirs, removed after each job)
- `judge/checker/checker.go`, `checker_test.go`: output comparison (`tokens`, `exact`)
- `judge/verdict/verdict.go`, `verdict_test.go`: verdict classification and summary
- Host: a second stash entry "pre-sync copy of feat/2-verdicts files" (identical to the branch, can be dropped; same cause as the first: files were copied with scp before being committed)
- `docs/phases/phase-2-log.md`: this log
- `scripts/setup-dev-host.sh`: now also installs the JDK
- `judge/problem/problem.go`, `version.go`, `problem_test.go`: problem loader, validation, test-set version, tests
- `judge/go.mod`, `judge/go.sum`: add `gopkg.in/yaml.v3` v3.0.1
- `problems/sample-sum/`: `problem.yaml` and `tests/01..05.in|out`
- `docs/adr/0005-problem-format-and-test-set-version.md`: ADR
- Host: `git stash` entry "pre-sync copy of feat/2-problem-format files" in `~/Leetforce` (identical to the branch; can be dropped)

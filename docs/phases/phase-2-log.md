# Phase 2 working log: Judge engine

**Branch:** `phase/2-judge-engine`
**Range:** `phase-2-start..phase-2-done`
**Status:** in progress

## Units of work
- [x] `docs/2-runtimes`: JDK on the dev host, setup script updated
- [x] `feat/2-problem-format`: `problem.yaml`, loader, test-set version hash, sample problem
- [x] `feat/2-verdicts`: verdict classification from host-measured facts
- [x] `feat/2-checkers`: host-side output comparison
- [ ] `feat/2-drivers-python-go`
- [ ] `feat/2-drivers-cpp-java`
- [ ] `feat/2-judge-cli`: `judge run problems/<slug> <file>`
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
2. Tests: 21 table-driven cases (CRLF, blank lines, split and joined tokens, case, order, empty output, trailing newline rules in exact mode) plus the unknown-mode error.
3. Mistake: the first commit was rejected by commitlint (a body line over 72 characters); the branch was pushed with no new commit, then the commit was redone with shorter lines and pushed. Nothing was lost.
4. Workflow change: this time the host got the code with `git fetch`/`git pull` of the pushed unit branch, not `scp`, so no untracked copies or stash entries were created. Host: `make fmt lint` 0 issues, `go test ./judge/checker` ok, working tree clean. No sandbox code changed.

## File and path index
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

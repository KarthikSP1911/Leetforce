# Phase 2 working log: Judge engine

**Branch:** `phase/2-judge-engine`
**Range:** `phase-2-start..phase-2-done`
**Status:** in progress

## Units of work
- [x] `docs/2-runtimes`: JDK on the dev host, setup script updated
- [ ] `feat/2-problem-format`: `problem.yaml`, loader, test-set version hash, sample problem
- [ ] `feat/2-verdicts`: verdict classification from host-measured facts
- [ ] `feat/2-checkers`: host-side output comparison
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

### Host checkout moved (2026-10-02)
Owner said "retry". Claude (host): `git checkout -- judge/sandbox/run.go` (discarded the whitespace-only gofmt edit), `git fetch`, `git checkout phase/2-judge-engine`, `git pull --ff-only`. Result: clean, at `a7dddac`, tracking `origin/phase/2-judge-engine`. Resolves the blocked step above.

## File and path index
- `docs/phases/phase-2-log.md`: this log
- `scripts/setup-dev-host.sh`: now also installs the JDK

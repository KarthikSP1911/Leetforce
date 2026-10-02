# 0007. Verdicts come only from host-measured facts and a host-side comparison

**Status:** accepted (Phase 2)

## Context
A verdict decides whether a user's program counts as correct, so it must not be something the program can write. Phase 1 gave the host trustworthy facts about a run (exit status and signal from nsjail's log, CPU time and peak memory from the cgroup, out-of-memory and pids events, output caps) and treated stdout, stderr and the result descriptor as untrusted data. Phase 2 has to turn those facts into AC, WA, TLE, MLE, RE, CE and OLE.

## Decision
- `verdict.Classify` looks only at `sandbox.Result`'s host-side fields. Order: **OLE** (an output cap was passed), then **MLE** (kernel OOM kill, peak memory over the limit, or the language's `OOMExitCode`), then **TLE** (wall-time kill, measured CPU time over the limit, or SIGXCPU), then **RE** (any other signal, non-zero exit, or a refused fork). Otherwise the run `Completed` and the output is compared.
- **TLE is decided from measured CPU time, not from the signal.** A CPU-time kill arrives as SIGKILL, which is indistinguishable from a crash. The kernel kill limit is set one second above the problem's time limit so a runaway always measures over the limit; the verdict then compares the measurement with the exact limit.
- **Output is compared on the host** by `checker.Check` (`tokens` or `exact`) on captured stdout. The expected output never enters the sandbox.
- **Compile errors** are CE: any non-zero exit, signal, timeout, OOM or output flood in the compile sandbox. A host failure (the sandbox cannot start) is an error returned to the caller, never a verdict.
- **Per-test details** (input, expected, actual, stderr) are recorded only for sample tests and only when the caller sets `Options.Detail` (Run). Hidden tests never get a detail.
- **A runtime that fails before the kernel sees the memory** (the JVM refuses a large allocation with `OutOfMemoryError`) may declare an exit status that means "out of memory" (`OOMExitCode`, 3 for Java with `ExitOnOutOfMemoryError`). That is still a host-measured exit status; a program that exits 3 on purpose only turns its own RE into MLE.
- The overall verdict is the first non-AC test, with the largest time and memory.

## Alternatives
- **A harness inside the sandbox reports the verdict over fd 4.** Rejected: anything the program can reach, it can forge (Phase 1 forgery tests; `TestJudgeIgnoresForgedVerdict`).
- **Parse stderr for "OutOfMemoryError" or "Killed".** Rejected: program-controlled text.
- **Classify TLE from SIGKILL plus wall time.** Rejected: confuses crashes with timeouts and depends on scheduling.
- **Judge all tests and report the worst verdict.** Not chosen: costs time and the usual judge behaviour (and LeetCode's) is to stop at the first failure; `ContinueOnFail` exists for local tooling.

## Consequences
- Every verdict in every language is covered by `TestJudgeVerdicts` (28 cases) and the unit tests of `Classify`.
- Two limits are accepted: the CPU time used includes runtime start-up, so Java gets a larger time limit in `problem.yaml` (the sample problem uses 2000 ms instead of 1000 ms); and CPU time is measured per cgroup, so multi-threaded programs are charged for all threads, which the one-CPU quota makes equal to wall time at most.
- Runtime stderr and hidden-test data stay out of Submit by construction, but compile errors are returned on Submit (the compiler's message about the user's own source). This differs from the literal wording in `CLAUDE.md` and is recorded as an open decision for the owner.

# 0006. Compile inside the sandbox and return the artifact over fd 4

**Status:** accepted (Phase 2, units 5-6)

## Context
Compilers run on untrusted source: they can be made to read files, consume memory, run for a long time or exploit bugs, so they must be sandboxed like the program. A compile step produces a file (a Go binary, later a C++ binary and a Java jar) that the test runs then need. The sandbox only has a read-only host view and a private tmpfs `/tmp` that disappears when the run ends, so the artifact has to get from one sandbox to the next.

## Decision
- The compile step runs in `sandbox.Run` with only the job's `src/` directory visible, read-only. The compile command writes the single built file to **fd 4** (`ResultFD`), for example `go build -o /tmp/main ... && cat /tmp/main >&4`. The host reads it as `Result.ResultData` (capped by `CompileLimits.MaxArtifactBytes`) and stores it as `<job dir>/bin/main`.
- Test runs bind the whole job directory read-only at the same path. No sandbox ever receives a writable host directory, so **no change to the sandbox package was needed** and the Phase 1 adversarial suite is unaffected.
- The artifact is untrusted bytes, treated as data until it is executed, and then only inside a sandbox. A compile "succeeds" only on exit 0, no signal, no timeout, no OOM and no output flood.
- Python has no artifact; it gets a syntax check (`compile()` of the source, nothing written) so a syntax error is CE, not RE.
- Job directories live under `/var/tmp`, never `/tmp`, because every sandbox mounts its own tmpfs on `/tmp`, which would hide a bind mount placed there. The engine refuses a work root under `/tmp`.
- The Go compile uses `GOROOT=/usr/local/go` explicitly (the sandbox has no `/proc`), `GOTELEMETRY=off`, `GOFLAGS=-p=1`, and a 64 MiB file-size limit (the runtime archive exceeds the 8 MiB default).

## Alternatives
- **Writable bind mount of a per-job host directory.** Rejected: it adds a writable host path to the sandbox (a new sandbox change needing the full adversarial suite, disk-fill and symlink questions, host-side quota). fd 4 reuses a channel that already exists, is size-capped by construction, and writes nothing on the host until the host chooses to.
- **Compile and run in one sandbox invocation.** Rejected: it cannot separate CE from RE cleanly, mixes compile and run limits, and would recompile for every test.
- **Compile on the host (outside the sandbox).** Rejected: violates "untrusted code never runs outside the sandbox".
- **Shared warm Go build cache, writable.** Rejected: a shared writable cache lets one submission poison builds of others.

## Consequences
Measured on the dev host (t3.micro, 0.9 GiB RAM, 2 vCPU burstable), Go compiles in 9-17 s inside the sandbox with a cold cache (a cold build outside the sandbox peaked at 257 MB resident; the sandbox limit is 400 MiB), so one Go submission costs 10-20 s of compile before its tests. This is acceptable for Phase 2 and for local use. Before real traffic (Phase 13 sizing, Phase 16 load test) it needs a warm read-only cache or a prebuilt standard library baked into the runner image; both avoid a writable shared cache. Tests: `make test-sandbox` runs the Go verdict matrix serially in about 100 s.

## C++ and Java specifics (unit 6)
- **C++:** `g++ -O2 -pipe -std=c++17 -static`, so the run needs nothing from the host but the kernel. A compile plus the five tests takes about 1 s on the dev host.
- **Java:** `javac` then `jar cfe` into one jar, returned over fd 4 like the other artifacts. The source file is stored as `Main.java`, so a submission must declare `public class Main`. Java takes about 10 s per submission on the dev host (two JVM starts for the compile, one per test).
- **No `/proc` and no `/etc/alternatives` in the sandbox.** `java`, `javac` and `jar` are run by their real paths under `/usr/lib/jvm/java-21-openjdk-amd64`. glibc resolves the JDK's `$ORIGIN` rpath through `/proc/self/exe`, so `LD_LIBRARY_PATH` points at the JDK `lib` directories. The Debian JDK links its configuration into `/etc/java-21-openjdk`, which each language can now declare as an extra read-only bind (`Language.Binds`).
- **Java memory verdict.** A JVM refuses a large allocation with a catchable `OutOfMemoryError` before the kernel ever sees the memory, so the cgroup facts alone call it a crash (measured: a 400 MB array on a 256 MB limit ended as RE with 18 MB peak). The heap is therefore set 64 MiB under the memory limit (half of it below 128 MiB) and the JVM runs with `-XX:+ExitOnOutOfMemoryError`, which ends it with status 3. `Language.OOMExitCode` carries that status into `verdict.Limits.OOMExitCode`, and `Classify` reads it as MLE. This is still a host-measured exit status; the worst a program can do is exit 3 on purpose and turn its own RE into MLE. A program that grows past the limit before the heap fills is OOM-killed by the cgroup, which is MLE as before.
- **Alternative rejected for Java MLE:** a heap larger than the limit so the cgroup always kills first. It fails for single allocations above the heap maximum (still RE) and makes ordinary garbage-heavy programs use more memory than they need.
- Java runs with C1 only (`-XX:TieredStopAtLevel=1`) and the serial GC to keep memory and thread count low; a CPU-heavy Java solution is slower than with C2. Revisit with real problems.

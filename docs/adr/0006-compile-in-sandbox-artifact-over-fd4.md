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

# Phase 1: Sandbox core

**Branch:** `phase/1-sandbox-core`
**Range:** `phase-1-start..phase-1-done`
**Status:** working draft (report is completed at the end of the phase)

## Units of work
- [ ] `docs/1-dev-environment`: ADR 0003, `scripts/setup-dev-host.sh`, README cost table
- [ ] `feat/1-go-workspace`: `go.work`, `judge/go.mod`, `.golangci.yml`, `Makefile`, `.env.example`
- [ ] `feat/1-nsjail-wrapper`: `judge/sandbox` Spec/Run, bounded output capture
- [ ] `feat/1-cgroup-limits`: cgroup v2 memory/pids/cpu limits, whole-cgroup kill, measurements
- [ ] `feat/1-result-channel`: dedicated fd for the harness result
- [ ] `test/1-adversarial`: `make test-adversarial` suite
- [ ] `docs/1-adrs-report`: ADR 0004, phase report, phase summary

## Exit criteria (from PLAN.md)
Fork bomb, memory bomb, infinite loop, output flood, network access, and file-system escape attempts are all contained; `make test-adversarial` passes.

## Environment
Developed on an Ubuntu 24.04 x86_64 EC2 t3.micro (see ADR 0003). Verified facts: cgroup v2 (`cgroup2fs`) with `cpu memory pids` controllers, `kernel.apparmor_restrict_unprivileged_userns = 1`, Go 1.27.1, nsjail built from `google/nsjail` commit `4ff54a6`.

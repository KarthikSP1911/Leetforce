# LeetForce progress

**Current phase:** 11 - Observability
**Status:** in review
**Resume point:** answer the Phase 11 review questions in chat, then merge `phase/11-observability` into `main`, tag `phase-11-done`, and start Phase 12
**Open decisions:** Phase 11 review pending: decisions A (notifier for critical alerts; none now) and B (queue sampled every 60 s, about 216k Redis commands a month; Upstash plan limit unchecked); the stack runs on the PC, not the dev host (disk 96% full); the first live-flow test run failed once for an unidentified reason and passed on re-run; the dashboard is not yet seen by the owner in a browser. Phase 10 review skipped by the owner (two phases in one session): decisions A (rejudge runs automatically at API start when a version changes) and B (old test bundles are kept) stay on Claude defaults; Phase 9 and 10 gates ran once at the end; the Phase 9 pages are unverified in a browser; the dev host is still up and billable. Phase 9 review skipped by the owner (two phases in one session): decisions A (keep the limit numbers in ADR 0017) and B (no email verification or password reset yet) stay on Claude defaults; the new login pages are unverified in a browser; Trivy not run this session. Phase 8 review skipped by the owner: understanding questions unanswered; decisions A (anonymous browser id until Phase 9, ADR 0016) and B (no rate limit on POST /runs until Phase 9) stay on Claude defaults; the dev host is still up and billable. Phase 7 review skipped by the owner: understanding questions unanswered; decisions A (keep --lf-success token; light-mode Easy 3.30:1), B (Monaco from CDN, ADR 0015) and C (dev host left running) stay on Claude defaults. Phase 6 review skipped by the owner: understanding questions unanswered; decisions A (ADR 0013 stays PROPOSED: nsjail default, gVisor opt-in), B (shared runner and program uid, ADR 0014) and C (Phase 5 decisions) stay on Claude's defaults. Phase 6: confirm ADR 0013 (nsjail default, gVisor opt-in) and the shared uid in ADR 0014. Phase 5 review skipped by the owner: understanding questions unanswered; decisions A (RustFS instead of MinIO), B (runner no-database guard allows two interface-only `database/sql` packages) and C (Neon plan and Upstash usage, 15-minute reaper) stay on Claude's defaults. Carried over: Phase 4 understanding questions unanswered; Neon plan and limits unchecked; Phase 2 decisions A and C on defaults; runner privilege model (Phase 6); `web/AGENTS.md` and `web/CLAUDE.md` stay untracked

| Phase | Name | Status | Sessions | Summary |
|---|---|---|---|---|
| 0 | Foundation | done | 1 | [summary](phases/phase-0-summary.md) |
| 1 | Sandbox core | done | 1 | [summary](phases/phase-1-summary.md) |
| 2 | Judge engine | done | 1 | [summary](phases/phase-2-summary.md) |
| 3 | Queue and runner | done | 1 | [summary](phases/phase-3-summary.md) |
| 4 | API and database | done | 1 | [summary](phases/phase-4-summary.md) |
| 5 | Live status and storage | done | 1 | [summary](phases/phase-5-summary.md) |
| 6 | Sandbox hardening | done | 1 | [summary](phases/phase-6-summary.md) |
| 7 | Web: problems and workspace | done | 1 | [summary](phases/phase-7-summary.md) |
| 8 | Web: run, submit, results | done | 1 | [summary](phases/phase-8-summary.md) |
| 9 | Auth and limits | done | 1 | [summary](phases/phase-9-summary.md) |
| 10 | Problem pipeline | done | 1 | [summary](phases/phase-10-summary.md) |
| 11 | Observability | in review | 1 | [summary](phases/phase-11-summary.md) |

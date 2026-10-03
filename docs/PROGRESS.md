# LeetForce progress

**Current phase:** 8 - Web: run, submit, results
**Status:** in review
**Resume point:** phase 8 review with the owner (questions in docs/phases/phase-8-summary.md), then merge phase/8-web-run-submit to main and tag phase-8-done
**Open decisions:** Phase 7 review skipped by the owner: understanding questions unanswered; decisions A (keep --lf-success token; light-mode Easy 3.30:1), B (Monaco from CDN, ADR 0015) and C (dev host left running) stay on Claude defaults. Phase 6 review skipped by the owner: understanding questions unanswered; decisions A (ADR 0013 stays PROPOSED: nsjail default, gVisor opt-in), B (shared runner and program uid, ADR 0014) and C (Phase 5 decisions) stay on Claude's defaults. Phase 6: confirm ADR 0013 (nsjail default, gVisor opt-in) and the shared uid in ADR 0014. Phase 5 review skipped by the owner: understanding questions unanswered; decisions A (RustFS instead of MinIO), B (runner no-database guard allows two interface-only `database/sql` packages) and C (Neon plan and Upstash usage, 15-minute reaper) stay on Claude's defaults. Carried over: Phase 4 understanding questions unanswered; Neon plan and limits unchecked; Phase 2 decisions A and C on defaults; runner privilege model (Phase 6); `web/AGENTS.md` and `web/CLAUDE.md` stay untracked

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
| 8 | Web: run, submit, results | in review | 1 | [summary](phases/phase-8-summary.md) |

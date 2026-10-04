# Phase 16 summary: Launch readiness

## TL;DR
- Contests (Phase 14) and the leaderboard (Phase 15) were built by other sessions and never tested. They are now merged with Phase 16 and the first full test run on the real stack passed.
- The system was reviewed for security (18 findings: 7 fixed with tests, 11 accepted in writing), load tested (24 of 24 submissions got a verdict) and given a backup and restore routine with a written runbook.
- The website now has a landing page, professional fonts, custom dropdowns and scrollbars, aligned panes, SVG artwork and gentle animations. The cloud itself was not set up: the cloud tests from Phase 13 are still unproven.

## Where this phase fits
```
 browser --> web (Next.js) --> API (Gin) --> Redis Streams --> runner --> sandbox --> verdict --> browser
   [built]      [built]         [built]        [built]         [built]    [built]     [built]     [built]

 Added in phases 14 and 15 (merged and tested in this phase):
   API: contests (register, hidden problems, timed window)  +  standings and leaderboard (Redis cache, version counter)
 Added in this phase (checks around the flow, nothing new in the flow itself):
   security fixes   load tester   backup and restore drill   landing page and UI polish
 Still to come (not in the plan):
   cloud exit tests from Phase 13 (apply, runner-loss), TLS in front of the API, protect the main branch
```
- This phase depends on everything before it: it only tests and protects what exists.
- It unblocks a public launch, once the owner decides the open items in `docs/launch-checklist.md`.
- There is no Phase 17: the plan ends here.

## What I built and why

### Merge of contests and leaderboard (`phase/14-contests`, `phase/15-leaderboard`)
- **What:** both phases were built at the same time by separate sessions, so Phase 15 used a temporary stand-in ("stub") for the scoring function that Phase 14 was still writing. I merged both and deleted the stand-in.
- **Why:** the two had never been run together, and nothing proved they agreed.
- **How it works:** the merge showed a real bug. The scorer ranks by "time since contest start" (`Event.Elapsed`), and the adapter never filled it in, so every penalty was 0 and ties were never broken. Phase 15's own tests caught it once the real scorer was plugged in (`api/internal/leaderboard/score_contract.go`).
- **Alternatives considered:** none; the plan said Phase 16 merges them.

### Security review (`feat/16-security-review`, `docs/security-review.md`)
- **What:** a read of the whole repository against the project's security rules, plus a Trivy scan for vulnerable dependencies, secrets and misconfigurations.
- **Why:** a judge runs strangers' code, so leaks and abuse matter most. The rules are: hidden tests never leave the server, runners never touch the database, limits per user and per IP, secrets never committed.
- **How it works:** every finding has an ID, a severity, a location and a status. Fixed ones have a test that fails without the fix: one IP could take every live-status connection (SEC-01), the login limit could be dodged by alternating email and username (SEC-02), POSTs accepted any content type (SEC-03), an open redirect after sign-in (SEC-04), idle connections were never closed (SEC-05). Two more came from my own review of Phases 14 and 15: the public standings listed a contest's problems before it started (SEC-16), and a backup file was readable by everyone (SEC-17).
- **Accepted, with reasons:** for example, sign-up reveals whether an email is taken (SEC-07), and anyone can lock a known account's login for ten minutes (SEC-08). Two need you: TLS in front of the API (SEC-06) and protecting the `main` branch (SEC-11).
- **Alternatives considered:** a paid scanner (not needed; Trivy is free).

### Load test (`tools/loadtest`, [ADR 0024](../adr/0024-load-test-tool.md))
- **What:** a small Go program that acts as many users: sign up, list problems, Run, Submit, and follow the live status until the verdict. It also has a contest mode.
- **Why:** to see the whole pipeline work with several users at once, and to have a repeatable way to measure later.
- **How it works:** `make test-loadtest-local` starts an API and a runner on a free port, runs both modes, checks that every accepted submission reached a verdict, and deletes the accounts it made. Result: 24 of 24 submissions got a verdict, no errors, a verdict took about 7 to 8 seconds on a small one-CPU machine with one runner.
- **Alternatives considered:** k6 (more features, but an extra install and a second language).
- **Honest limit:** this shows it works, not how far it scales.

### Backup and restore drill (`scripts/backup-neon.sh`, `restore-drill-neon.sh`, `backup-s3-bundles.sh`, `docs/runbook-backup-restore.md`)
- **What:** a way to copy the database, prove the copy restores, and clean up after itself.
- **Why:** a backup you have never restored is only a hope.
- **How it works:** dump the real database (read-only), restore it into a throwaway database, compare table row counts and the migration version, then drop the throwaway. It passed (8 tables, equal counts). One catch: Neon runs PostgreSQL 18, so the tools must be version 18 too, which I ran through a container.
- **Limits:** the cloud storage bucket is empty, so that part only tested the mechanics; the temporary-Neon-branch mode was not run (you chose the local scratch option).

### Docs and cost review (`README.md`, `docs/cost-review.md`, `docs/FLOW.md`)
- **What:** a README with a system diagram and a high-level design, the cost of everything that runs or could run, and a check of the flow document against the code.
- **Why:** so someone new, or you in six months, can see how the parts fit and what they cost.
- **Honest limit:** the dollar amounts are estimates from memory of AWS list prices and are marked UNVERIFIED.

### Web polish (`feat/16-web-polish`, `feat/16-home-art`, `feat/16-web-motion`, two fixes)
- **What:** a real landing page, sign-in and sign-up that fit the window, Inter and JetBrains Mono fonts, a custom dropdown and thin themed scrollbars, panes with matching header heights, a user avatar, SVG artwork, and Framer Motion animations that respect the "reduce motion" setting.
- **Why:** your requests. One lesson: the dev server on your PC never started its JavaScript (every button was dead), so I served a production build instead.

## How it works now, step by step
Same as Phase 15, with contests. For a contest submission:
1. You sign in, open `/contest/<slug>` and press Register.
2. The contest problems become visible to you once the contest starts (and stay hidden from everyone else).
3. You submit from the contest page; the API checks the time window and your registration, saves the submission with the contest id, and queues it.
4. A runner judges it in a sandbox and sends the verdict back; the API stores it once, even if the message is delivered twice.
5. The ranking counter goes up, so the next read of the standings recomputes: most problems solved first, then the lowest penalty (minutes to the first correct answer plus 20 for each wrong attempt before it).

## Key concepts
- **Stub:** a temporary stand-in for code another team has not finished. It lets work go on in parallel, and must be removed when the real code arrives.
- **Oracle (in a test):** an independent, simple calculation of the right answer to compare the real code against. Ours was wrong once: it counted submissions after the contest ended.
- **Restore drill:** proving a backup works by restoring it somewhere safe and checking it.
- **Idempotent:** doing something twice has the same effect as doing it once. That is why a duplicate verdict changes nothing.
- **Race detector:** a Go tool (`-race`) that finds code where two things touch the same data at once; the leaderboard test runs with it.

## Try it yourself
```bash
# the website (this PC; the production build used during the phase)
#   http://192.168.1.182:3101
# on the dev host: ssh leetforce-dev, cd ~/Leetforce
make test-mock-contest              # expect: PASS: a full mock contest ran end to end
make test-leaderboard-concurrent    # expect: PASS three times
make test-loadtest-local            # expect: PASS: load test finished and every accepted submission reached a verdict
scripts/backup-neon.sh --dry-run    # expect: prerequisites ok, nothing written
```
Read `docs/security-review.md` for the findings and `docs/launch-checklist.md` for what is still yours to decide.

## Trade-offs and risks
- **No TLS yet (SEC-06).** Fine while the API only accepts your own address; not fine before a public launch.
- **Cloud untested.** The AMI, the cluster and the runner-loss test were never run, so "survives a lost runner in the cloud" is still a claim, not a fact. The local version of that test passes.
- **Small load only.** One runner on one CPU. The first real traffic could expose limits (Redis commands on the free plan are the likeliest, and are unchecked).
- **Parallel phases.** Building 14 and 15 without running them cost a few fixes at the end (listed in the log). It was quicker overall, but the real lesson is that tests written against a stub need running against the real thing early.
- **Revisit** if a runner count above one makes the queue or the database the bottleneck.

## Review questions
Skipped by the owner (standing override for Phases 14 to 16).

## Review Q&A
Skipped by owner. No questions were answered; none are recorded as answered.

## Open decisions
- Apply the cloud (billable) and run the runner-loss test; tag M4 afterwards. Phase 13 item.
- TLS for the API, and protecting `main` (SEC-06, SEC-11). Matters before any public launch.
- Upload test bundles to the AWS bucket, then re-run the S3 drill.
- Neon: read the restore window and plan limits; rotate the password that was shown in chat in Phase 13.
- Delete the stale test stack still running on the dev host (an API and a runner from before this session).

## Handoff
- **State:** merged into `main` with tags `phase-14-done`, `phase-15-done`, `phase-16-done` and `M5`; the branches `phase/14-contests`, `phase/15-leaderboard` and `phase/16-launch-readiness` are kept. The website is served from this PC on port 3101 and the API on 18090 until you stop them. The demo contest was removed for the gate and was not recreated (the dev host stopped answering at the very end); the `tester1` test account is still in the real database and should be deleted. To make a contest to look at, run `scripts/seed-mock-contest.sh` on the dev host.
- **Next phase:** none in the plan. The next work is the launch checklist (`docs/launch-checklist.md`).
- **Next session prompt:**
  ```
  Continue LeetForce. Read CLAUDE.md, docs/PROGRESS.md, docs/launch-checklist.md and docs/phases/phase-16-summary.md. The plan (Phases 0 to 16) is complete. Ask me which launch-checklist items to do first (cloud apply, TLS, branch protection, rotating the Neon password) and show me the plan before running anything billable.
  ```

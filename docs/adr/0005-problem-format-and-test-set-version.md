# 0005. Problem format: stdin/stdout tests, YAML spec, content-hash test-set version

**Status:** accepted (Phase 2)

## Context
The judge needs a definition of a problem that an author can write by hand, that can be validated, and that identifies exactly which tests a submission was judged against (CLAUDE.md: submissions record the test-set version so they can be rejudged after a test fix).

## Decision
- A problem is a directory: `problem.yaml` plus `tests/NAME.in` and `tests/NAME.out` pairs. Programs read stdin and write stdout in all four languages. Function-signature templates (LeetCode style) are not part of Phase 2.
- `problem.yaml` fields: `slug`, `title`, `difficulty` (easy/medium/hard), `tags`, `checker` (`tokens` default, or `exact`), `limits.default` and optional `limits.overrides.<language>` (`time_ms`, `memory_mb`), and `samples` (names of the visible tests; all others are hidden). Unknown fields and unknown languages are errors. `memory_mb` is unsigned, so a negative value fails at parse time.
- The test-set version is `ts-` plus the first 16 hex characters of a SHA-256 over the checker name and every test's name, input and expected output in name order, each field length-prefixed. Limits, titles and the sample flags are not hashed, because they do not change what a correct answer is. The hash is computed from file contents, so it is identical on every machine (`.gitattributes` forces LF).
- YAML is parsed with `gopkg.in/yaml.v3` (strict mode, `KnownFields`).

## Alternatives
- **Sequential version numbers kept by an author or the database.** Rejected: easy to forget to bump, and two copies of a problem can disagree. A content hash cannot drift from the tests.
- **JSON or TOML instead of YAML.** Not chosen: `docs/PLAN.md` names `problem.yaml`, and YAML allows comments.
- **Function-signature problems with generated drivers.** Deferred: it needs a type map and templates per language and a harness protocol. Stdin/stdout lets Phase 2 finish the judge itself first; it can be added later without changing the sandbox.
- **Standard-library-only parsing.** Not possible for YAML; yaml.v3 is the de facto Go library.

## Consequences
- Any edit to a test, or to the checker, changes the version, so rejudge-by-version (Phase 10) can find affected submissions.
- Per-language limits let Java get more time and memory without loosening Python or C++.
- Phase 4 stores `TestSetVer` with each submission.
- `gopkg.in/yaml.v3` is the first third-party dependency of the `judge` module.

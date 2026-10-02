# 0011. Problem tests live in an S3 bucket as bundles keyed by test-set version; RustFS stands in for MinIO locally

**Status:** accepted (Phase 5). The owner delegated the choices ("do as u wish"); they are Claude's recommendations and can be changed. It replaces the Phase 4 review decision "MinIO in Docker on the dev host", which turned out to be impossible (see Context).

## Context
Until Phase 4 a runner read hidden tests from a `problems/` directory next to it. That cannot work once runners are separate machines (Phase 13), and it cannot honour "rejudge against the exact test set a submission was accepted against": the directory only holds the current tests. The submission row already records `test_set_version` (ADR 0005); nothing used it to fetch tests.

The Phase 4 plan was MinIO in Docker. On the dev host `docker compose pull` failed with "pull access denied": MinIO no longer publishes container images (Docker Hub `minio/minio` and quay.io were checked, including the old `RELEASE.2025-04-22T22-12-26Z` tag).

## Decision
- **Bundle:** one object per problem version, `problems/<slug>/<test-set-version>.tar.gz`, holding `problem.yaml` and `tests/` only (reference solutions never travel). `problem.Pack` is deterministic (sorted entries, zero timestamps, fixed modes) and `problem.Unpack` accepts only those two kinds of path, rejects links, directories and path escapes, and stops at 64 MiB. `sample-sum` (the only problem so far) packs from 589,239 to 213,938 bytes.
- **Who publishes:** the API, at startup, before it listens (`catalog.Publish`), so every version a submission can be stamped with is already in the bucket. The object carries its SHA-256 as metadata; an identical bundle is not rewritten.
- **Who fetches:** the runner (`runner/internal/problems`). The job now carries `test_set_version`; the runner downloads that bundle, **recomputes the version from the unpacked tests and refuses a mismatch** (permanent error), so a runner can never judge against tests the submission was not stamped with. `LEETFORCE_S3_ENDPOINT` unset keeps the old directory mode for development and the earlier exit tests.
- **Runner cache:** unpacked under `LEETFORCE_PROBLEM_CACHE`, one directory per **bundle hash**, not per version. The version deliberately excludes limits and titles (ADR 0005), so `problem.yaml` can change under an unchanged version; keying by hash makes the runner refetch it. Each job costs one metadata request (`Stat`); an unchanged bundle is read from disk. Unpacking goes to a temp directory and is renamed into place, so a concurrent job never sees a half-written entry.
- **Failure policy:** bad slug, missing or malformed version, corrupt bundle, version mismatch: permanent, the job ends `IE` at once. Storage down or bundle not yet uploaded: retried like any host failure and ends `IE` after the attempts.
- **Local object store:** RustFS 1.0.0 (Apache-2.0, S3 API, pinned tag) in `docker-compose.yml`, bound to localhost; `trivy image` reported 0 HIGH/CRITICAL. The code uses only the plain S3 API through the minio-go client library, so the server is replaceable and production uses real S3. Env: `LEETFORCE_S3_ENDPOINT|ACCESS_KEY|SECRET_KEY|BUCKET|USE_TLS`.
- **Runner dependency guard narrowed:** the S3 client pulls in `google/uuid` and `rs/xid`, which import `database/sql/driver` (only the `Valuer` interface). `runner/nodb_test.go` now skips exactly `database/sql/driver` and `database/sql/internal`; `database/sql` itself, pgx, lib/pq, gorm, sqlx and Gin stay forbidden, so the rule "runners never connect to the database" is still enforced. The change is called out here because it touches a security guard.

## Alternatives
- **Garage** (`dxflrs/garage`): also available and S3 compatible, but needs a config file and layout commands before first use; RustFS is one container with env credentials.
- **Build MinIO from source:** heavy on a 1 GB host and a binary we would then maintain.
- **Skip object storage until the cloud (S3 only):** would leave Phase 5's end-to-end on one machine without the storage half and the cloud path untested.
- **Key the cache by version only:** simpler, but serves stale limits after a `problem.yaml` edit.
- **Runner fetches from the API over HTTP:** puts hidden tests behind an API endpoint, the opposite of "hidden data never leaves"; the bucket is reachable only by the API (write) and runners (read) once Phase 12 sets credentials per role.
- **Drop the dependency and write a minimal SigV4 client:** about 300 lines of security-sensitive code we would own. minio-go was scanned (0 findings) and is a thin client.

## Consequences
- Verified: `storage` round trip against the real RustFS (identical put is a no-op, changed bytes under the same version are replaced, metadata hash preserved); `make test-live-e2e` runs a runner with `LEETFORCE_PROBLEMS_DIR=/nonexistent` and no `DATABASE_URL` to AC 5/5, so the tests can only have come from the bucket.
- One credential pair (`LEETFORCE_S3_ACCESS_KEY`/`SECRET_KEY`) is used by both the API and the runner today. Separate read-only credentials for runners belong with infrastructure as code (Phase 12); until then the localhost-only server limits exposure.
- Old bundles are never deleted (object per version); a retention rule is deferred to the problem pipeline (Phase 10).
- RustFS is young software. If it misbehaves, Garage or real S3 replaces it with an env change.

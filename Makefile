# LeetForce developer commands.
# Go components are separate modules joined by go.work (ADR 0002); add each new
# module (runner, api) to GO_MODULES when it is created.

GO_MODULES := judge queue runner api storage

.PHONY: test-obs-e2e dev-obs down-obs validate-problems test-rejudge-e2e test-auth-e2e test-live-e2e test-api-e2e build-api migrate-up migrate-down migrate-status dev down fmt lint test build-judge build-runner test-crash test-matrix test-sandbox test-adversarial bench-sandbox

# Phase 11 exit test: leetforce_ metrics follow a live submission flow, and a
# lost runner shows as waiting, ageing jobs (dev host; real DB, throwaway Redis prefix).
test-obs-e2e:
	scripts/test-obs-e2e.sh

# Observability stack (Prometheus, Grafana, Loki, Alloy) on the machine that
# runs Docker, reading the dev host's /metrics through scripts/obs-tunnel.sh.
dev-obs:
	docker compose -f observability/docker-compose.yml up -d

down-obs:
	docker compose -f observability/docker-compose.yml down

# Local Redis and S3 (RustFS) (needs Docker and LEETFORCE_S3_SECRET_KEY in .env).
dev:
	docker compose --env-file .env up -d

down:
	docker compose down

fmt:
	@for m in $(GO_MODULES); do (cd $$m && golangci-lint fmt ./...) || exit 1; done

lint:
	@for m in $(GO_MODULES); do (cd $$m && go vet ./... && golangci-lint run ./...) || exit 1; done

test:
	@for m in $(GO_MODULES); do (cd $$m && go test -count=1 ./...) || exit 1; done

# Builds the local judge CLI to bin/judge. It needs root to run, for example:
#   sudo -n bin/judge run problems/sample-sum problems/sample-sum/solutions/python/ac.py
build-judge:
	@mkdir -p bin
	cd judge && go build -o ../bin/judge ./cmd/judge

# Phase 10: validates every problem under problems/ (spec, statement, starters,
# test pairs) and judges every solutions/<lang>/<verdict>.<ext> against its
# promised verdict. Needs the sandbox (root, nsjail, cgroup v2). One problem:
#   make validate-problems DIR=problems/fizz-count
# Structure only, no sandbox: bin/judge validate -structure-only problems
DIR ?= problems
validate-problems: build-judge
	sudo -n bin/judge validate $(DIR)

# Builds the runner to bin/runner and the queue tool to bin/lfq. The runner needs
# root and LEETFORCE_REDIS_URL, for example:
#   sudo -n env LEETFORCE_REDIS_URL=... bin/runner
build-runner:
	@mkdir -p bin
	cd runner && go build -o ../bin/runner ./cmd/runner
	cd queue && go build -o ../bin/lfq ./cmd/lfq

# Phase 3 exit criterion: kill a runner mid-job, another runner reclaims the job
# and exactly one verdict is recorded. Uses LEETFORCE_REDIS_URL (.env), real
# sandbox, a throwaway key prefix; about a minute.
test-crash:
	scripts/test-crash-reclaim.sh

# Phase 2 exit criterion: judges the sample problem with one solution per
# verdict (AC, WA, TLE, MLE, RE, OLE, CE) in each of Python, C++, Java and Go
# (28 runs, about 3 minutes on the dev host, mostly cold Go and Java compiles).
test-matrix:
	sudo -n env "PATH=$$PATH" go test -p 1 -timeout 20m -count=1 -v -run 'TestVerdictMatrixIsComplete|TestJudgeVerdicts' ./judge/engine/

# Runs real programs inside nsjail (functional sandbox tests, including the
# engine's verdict tests). Same host requirements as test-adversarial; plain
# `make test` skips these. Packages run one at a time (-p 1): they share one
# cgroup root and a memory cap, and a Go compile alone uses most of it.
test-sandbox:
	sudo -n env "PATH=$$PATH" go test -p 1 -timeout 20m -count=1 -v ./judge/...

# Sandbox containment suite: hostile programs (fork, memory and output bombs,
# network and file-system escapes) that must all be contained. Needs Linux,
# cgroup v2, nsjail, systemd and passwordless sudo (see ADR 0003). It builds the
# test binary as the normal user, then runs it as root (nsjail and cgroup writes
# need it) inside a systemd scope that caps the whole suite's memory and tasks,
# so a containment failure cannot take down the dev host. Also runs the
# ordinary sandbox tests. Single test: make test-adversarial RUN=TestAdversarialForkBomb
ADV_BIN := bin/sandbox-adversarial.test
RUN ?= .

test-adversarial:
	@mkdir -p bin
	go test -tags adversarial -c -o $(ADV_BIN) ./judge/sandbox
	sudo -n systemd-run --scope --quiet -p MemoryMax=600M -p MemorySwapMax=0 -p TasksMax=1500 \
		./$(ADV_BIN) -test.v -test.count=1 -test.run '$(RUN)'

# Phase 4 exit test: API, Redis, runner, ingest and Postgres end to end (needs DATABASE_URL and LEETFORCE_REDIS_URL in .env).
test-api-e2e:
	scripts/test-api-e2e.sh

# Phase 5 exit test: queued, judging, verdict over SSE with tests read from the
# S3 bucket (RustFS from make dev); nothing hidden in any response; the reaper
# re-queues an orphaned submission. Needs psql, DATABASE_URL, LEETFORCE_REDIS_URL
# and LEETFORCE_S3_* in .env; deletes the rows it creates. About two minutes.
test-live-e2e:
	scripts/test-live-e2e.sh

test-auth-e2e:
	scripts/test-auth-e2e.sh

# Phase 10 exit test: fix a test set, restart the API, the old submission is
# rejudged against the new version and its verdict replaced once. Needs psql,
# DATABASE_URL and LEETFORCE_REDIS_URL in .env, sudo and nsjail; deletes the
# rows it creates. About two minutes.
test-rejudge-e2e:
	scripts/phase10-e2e.sh

# Builds bin/api (needs DATABASE_URL and LEETFORCE_REDIS_URL to run).
build-api:
	@mkdir -p bin
	cd api && go build -o ../bin/api ./cmd/api

# Database migrations (goose, SQL files in api/migrations). The URL comes from
# .env: LEETFORCE_MIGRATE_DATABASE_URL (Neon direct endpoint) or DATABASE_URL.
migrate-up migrate-down migrate-status:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	url="$${LEETFORCE_MIGRATE_DATABASE_URL:-$$DATABASE_URL}"; \
	[ -n "$$url" ] || { echo "set DATABASE_URL in .env"; exit 1; }; \
	goose -dir api/migrations postgres "$$url" $(patsubst migrate-%,%,$@)

# Phase 6: nsjail vs gVisor (cold start, per-job overhead, CPU-bound, syscall-heavy,
# memory overhead, Go/Java/C++ compile+run) with median and p95 over repeated,
# interleaved runs. Needs root, nsjail, runsc (scripts/setup-gvisor.sh) and gcc.
# Run it on an otherwise idle host; it drops the page cache once per backend for
# the cold-start sample. Pass BENCH_ARGS="-reps 5 -skip-langs" for a quick pass.
bench-sandbox:
	@mkdir -p bin
	cd judge && go build -o ../bin/sandbox-bench ./cmd/sandbox-bench
	sudo -n env "PATH=$$PATH" ./bin/sandbox-bench $(BENCH_ARGS)

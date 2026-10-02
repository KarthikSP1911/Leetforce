# LeetForce developer commands.
# Go components are separate modules joined by go.work (ADR 0002); add each new
# module (runner, api) to GO_MODULES when it is created.

GO_MODULES := judge

.PHONY: dev down fmt lint test build-judge test-matrix test-sandbox test-adversarial

# Local Redis and MinIO (needs Docker and LEETFORCE_MINIO_PASSWORD in .env).
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

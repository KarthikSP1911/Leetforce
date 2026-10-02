# LeetForce developer commands.
# Go components are separate modules joined by go.work (ADR 0002); add each new
# module (runner, api) to GO_MODULES when it is created.

GO_MODULES := judge

.PHONY: fmt lint test test-sandbox test-adversarial

fmt:
	@for m in $(GO_MODULES); do (cd $$m && golangci-lint fmt ./...) || exit 1; done

lint:
	@for m in $(GO_MODULES); do (cd $$m && go vet ./... && golangci-lint run ./...) || exit 1; done

test:
	@for m in $(GO_MODULES); do (cd $$m && go test -count=1 ./...) || exit 1; done

# Runs real programs inside nsjail (functional sandbox tests). Same host
# requirements as test-adversarial; plain `make test` skips these.
test-sandbox:
	sudo -n env "PATH=$$PATH" go test -count=1 -v ./judge/...

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

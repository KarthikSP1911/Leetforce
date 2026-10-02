# LeetForce developer commands.
# Go components are separate modules joined by go.work (ADR 0002); add each new
# module (runner, api) to GO_MODULES when it is created.

GO_MODULES := judge

.PHONY: fmt lint test test-adversarial

fmt:
	@for m in $(GO_MODULES); do (cd $$m && golangci-lint fmt ./...) || exit 1; done

lint:
	@for m in $(GO_MODULES); do (cd $$m && go vet ./... && golangci-lint run ./...) || exit 1; done

test:
	@for m in $(GO_MODULES); do (cd $$m && go test -count=1 ./...) || exit 1; done

# Sandbox containment suite. Needs Linux, cgroup v2, nsjail and passwordless sudo
# (see ADR 0003). Runs as root because nsjail and cgroup writes need it.
test-adversarial:
	sudo -n env "PATH=$$PATH" go test -tags adversarial -count=1 -v ./judge/...

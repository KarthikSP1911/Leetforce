# LeetForce runner image (Phase 17, ADR 0029). WRITTEN, NOT BUILT: nothing here has been run.
# Build from the repository root (the Go workspace is outside runner/):
#   docker build -f runner/Dockerfile -t ghcr.io/karthiksp1911/leetforce-runner:<sha> .
#
# The image holds the runner binary, nsjail and the four language toolchains at the exact
# paths judge/lang/lang.go expects. It holds no problems and no tests: the runner fetches
# test bundles from S3 at run time. The container starts as root only so that
# docker-entrypoint.sh can make its cgroup writable; the runner itself runs as uid 10001
# with every capability removed (ADR 0014 model, ADR 0029).

# Same commit as ansible/roles/runner_host/defaults/main.yml (runner_host_nsjail_commit).
# scripts/check-nsjail-pin.sh compares the two.
ARG NSJAIL_COMMIT=4ff54a6e0d5b65a0e1633d6e2fd1425ba6c882ce
ARG UBUNTU=ubuntu:24.04
ARG GOLANG=golang:1.27-bookworm

# ---- nsjail, built from the pinned commit (kafel is a submodule) ----
FROM ${UBUNTU} AS nsjail
ARG NSJAIL_COMMIT
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates git build-essential autoconf bison flex libtool pkg-config \
      libprotobuf-dev protobuf-compiler libnl-route-3-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src/nsjail
RUN git init -q . \
    && git remote add origin https://github.com/google/nsjail \
    && git fetch -q --depth 1 origin "${NSJAIL_COMMIT}" \
    && git checkout -q FETCH_HEAD \
    && git submodule update --init --recursive --depth 1 \
    && make -j"$(nproc)" \
    && strip nsjail

# ---- runner binary (static) ----
FROM ${GOLANG} AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

# Module files first so dependency downloads are cached between source changes.
COPY go.work go.work.sum ./
COPY runner/go.mod runner/go.sum runner/
COPY judge/go.mod judge/go.sum judge/
COPY queue/go.mod queue/go.sum queue/
COPY storage/go.mod storage/go.sum storage/
COPY telemetry/go.mod telemetry/go.sum telemetry/
# The API and the load tester are not part of the runner's dependency graph.
RUN go work edit -dropuse=./api -dropuse=./tools/loadtest && cd runner && go mod download

COPY runner runner
COPY judge judge
COPY queue queue
COPY storage storage
COPY telemetry telemetry
RUN go build -ldflags="-s -w" -o /out/runner ./runner/cmd/runner

# ---- final image ----
FROM ${UBUNTU}
# python3, g++ and the JDK come from Ubuntu so the paths match lang.go:
# /usr/bin/python3, g++ on PATH, /usr/lib/jvm/java-21-openjdk-amd64 and /etc/java-21-openjdk.
# util-linux provides setpriv and mount for the entrypoint; the two lib packages are nsjail's
# run-time libraries.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates python3 g++ openjdk-21-jdk-headless util-linux \
      libprotobuf32t64 libnl-route-3-200 libnl-3-200 \
    && rm -rf /var/lib/apt/lists/*

# Go at /usr/local/go (lang.go: GOROOT=/usr/local/go, GOTOOLCHAIN=local). The official image
# carries the same Go the module builds with; the sandbox mounts /usr, so it is visible there.
COPY --from=build /usr/local/go /usr/local/go
COPY --from=nsjail /src/nsjail/nsjail /usr/local/bin/nsjail
COPY --from=build /out/runner /usr/local/bin/runner
COPY runner/docker-entrypoint.sh /usr/local/bin/leetforce-entrypoint

RUN chmod 0755 /usr/local/bin/leetforce-entrypoint /usr/local/bin/nsjail /usr/local/bin/runner \
    && groupadd --system --gid 10001 lfrunner \
    && useradd --system --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin lfrunner

# Per-pod scratch space (job directories, problem cache) is mounted over /tmp and /var/tmp by the chart.
ENV LEETFORCE_PROBLEM_CACHE=/var/tmp/leetforce-problems \
    LEETFORCE_METRICS_ADDR=0.0.0.0:9101 \
    PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

# The default user is the unprivileged one, so a plain `docker run` never runs as root. The pod
# sets runAsUser: 0 for the entrypoint's setup step (k8s/charts/leetforce-runner); the entrypoint
# then drops to this user before it starts the runner.
USER lfrunner:lfrunner
EXPOSE 9101
ENTRYPOINT ["/usr/local/bin/leetforce-entrypoint"]

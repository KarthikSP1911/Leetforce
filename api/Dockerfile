# LeetForce API image. Build from the repository root (the Go workspace and problems/ are
# outside api/):  docker build -f api/Dockerfile -t ghcr.io/karthiksp1911/leetforce-api .
#
# The image contains problems/ because the API publishes the problem bundles to S3 at
# start-up, and that includes the hidden tests. Keep the GHCR package PRIVATE.

FROM golang:1.27-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

# Module files first so dependency downloads are cached between source changes.
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum api/
COPY judge/go.mod judge/go.sum judge/
COPY queue/go.mod queue/go.sum queue/
COPY storage/go.mod storage/go.sum storage/
# The runner module is not part of the API's dependency graph.
RUN go work edit -dropuse=./runner && cd api && go mod download

COPY api api
COPY judge judge
COPY queue queue
COPY storage storage
RUN go build -ldflags="-s -w" -o /out/api ./api/cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /app/api
COPY problems /app/problems
ENV LEETFORCE_API_ADDR=:8080 LEETFORCE_PROBLEMS_DIR=/app/problems
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]

# This is a multi-stage Dockerfile that builds all services in the project.
# Use 'docker build --target <service-name>' to build a specific service.
# Available targets: cni, daemon, operator, lb-control

# ============================================================================
# Build stages
# ============================================================================

# Common base build stage for all services
FROM golang:1.26 AS base-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.work go.work.sum ./
COPY cni/go.mod cni/go.sum cni/
COPY api/go.mod api/go.sum api/
COPY daemon/go.mod daemon/go.sum daemon/
COPY operator/go.mod operator/go.sum operator/

# Common builder for CNI (loom) - golang:1.24
FROM golang:1.26 AS cni-builder
ARG TARGETOS
ARG TARGETARCH

COPY --from=base-builder /workspace /workspace
WORKDIR /workspace
RUN cd cni && go mod download && mkdir bin

COPY cni/ cni/

RUN cd cni && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/loom cmd/main.go

# ============================================================================
# Daemon builder - golang:1.25
FROM golang:1.26 AS daemon-builder
ARG TARGETOS
ARG TARGETARCH

COPY --from=base-builder /workspace /workspace
WORKDIR /workspace
RUN cd daemon && go mod download && mkdir bin

COPY api/ api/
COPY daemon/ daemon/

RUN cd daemon && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/daemon cmd/main.go

# ============================================================================
# Operator builder - golang:1.24
FROM golang:1.26 AS operator-builder
ARG TARGETOS
ARG TARGETARCH

COPY --from=base-builder /workspace /workspace
WORKDIR /workspace
RUN cd operator && go mod download && mkdir bin

COPY api/ api/
COPY operator/ operator/

RUN cd operator && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/manager cmd/main.go

# ============================================================================
# Final images
# ============================================================================

# CNI image (loom)
FROM alpine:3.19 AS cni
WORKDIR /
COPY --from=cni-builder /workspace/cni/bin/loom .
ENTRYPOINT ["/loom"]

# Daemon image
FROM alpine:3.20 AS daemon
RUN apk add --no-cache iptables iptables-legacy
WORKDIR /
COPY --from=daemon-builder /workspace/daemon/bin/daemon .
ENTRYPOINT ["/daemon"]

# Operator image (manager)
FROM gcr.io/distroless/static:nonroot AS operator
WORKDIR /
COPY --from=operator-builder /workspace/operator/bin/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]



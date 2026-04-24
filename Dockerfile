# This is a multi-stage Dockerfile that builds all services in the project.
# Use 'docker build --target <service-name>' to build a specific service.
# Available targets: cni, daemon, operator, lb-control

# ============================================================================
# Build stages
# ============================================================================

# Common builder for CNI (loom) - golang:1.24
FROM golang:1.24 AS cni-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY cni/go.mod cni/go.mod
COPY cni/go.sum cni/go.sum
RUN cd cni && go mod download

COPY cni/ cni/

RUN cd cni && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o ../loom cmd/main.go

# ============================================================================
# Daemon builder - golang:1.25
FROM golang:1.25 AS daemon-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum api/
COPY daemon/go.mod daemon/go.sum daemon/
RUN cd daemon && go mod download

COPY api/ api/
COPY daemon/ daemon/

RUN cd daemon && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o ../daemon cmd/main.go

# ============================================================================
# Operator builder - golang:1.24
FROM golang:1.24 AS operator-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum api/
COPY operator/go.mod operator/go.sum operator/
RUN cd operator && go mod download

COPY api/ api/
COPY operator/ operator/

RUN cd operator && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o ../manager cmd/main.go

# ============================================================================
# lb-control builder - golang:1.26
FROM golang:1.26 AS lb-control-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum api/
COPY examples/load-balancer/lb-control/go.mod examples/load-balancer/lb-control/go.mod
COPY examples/load-balancer/lb-control/go.sum examples/load-balancer/lb-control/go.sum
RUN cd examples/load-balancer/lb-control && go mod download

COPY api/ api/
COPY examples/load-balancer/lb-control/ examples/load-balancer/lb-control/

RUN cd examples/load-balancer/lb-control && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o ../../../lb-control cmd/main.go

# ============================================================================
# Final images
# ============================================================================

# CNI image (loom)
FROM alpine:3.19 AS cni
WORKDIR /
COPY --from=cni-builder /workspace/loom .
ENTRYPOINT ["/loom"]

# Daemon image
FROM alpine:3.20 AS daemon
RUN apk add --no-cache iptables iptables-legacy
WORKDIR /
COPY --from=daemon-builder /workspace/daemon .
ENTRYPOINT ["/daemon"]

# Operator image (manager)
FROM gcr.io/distroless/static:nonroot AS operator
WORKDIR /
COPY --from=operator-builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]

# lb-control image
FROM alpine:3.20 AS lb-control
WORKDIR /
COPY --from=lb-control-builder /workspace/lb-control .
ENTRYPOINT ["/lb-control"]




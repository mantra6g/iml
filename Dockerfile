# This is a multi-stage Dockerfile that builds all services in the project.
# Use 'docker build --target <service-name>' to build a specific service.
# Available targets: cni, daemon, operator, bmv2

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
# Daemon pre-builder - ubuntu:24.04
FROM ubuntu:24.04 AS daemon-ebpf-builder

# Install build dependencies
# clang/llvm for BPF compilation
# libbpf-dev for the BPF headers
# linux-headers-generic for asm/types.h and other kernel definitions
RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY --from=base-builder /workspace /workspace
WORKDIR /workspace
COPY daemon/ebpf/ daemon/ebpf/

# Find the architecture-specific include path dynamically and compile.
# We include /usr/include/$(uname -m)-linux-gnu to resolve 'asm/types.h'.
RUN ARCH=$(uname -m | sed 's/x86_64/x86_64-linux-gnu/') && \
    clang -O2 -g -target bpf \
    -I/usr/include/$ARCH \
    -I/usr/include \
    -c daemon/ebpf/src/recalc_csum.c -o daemon/ebpf/recalc_csum.o

# ============================================================================
# Daemon builder - golang:1.25
FROM golang:1.26 AS daemon-builder
ARG TARGETOS
ARG TARGETARCH

COPY --from=daemon-ebpf-builder /workspace /workspace
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
# BMv2 driver builder — standalone module (not in go.work), final image needs
# p4c at runtime to compile P4 source files downloaded via URL.
# ============================================================================
FROM golang:1.26 AS bmv2-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY targets/bmv2/go.mod targets/bmv2/go.sum ./
RUN go mod download

COPY targets/bmv2/cmd/ cmd/
COPY targets/bmv2/api/ api/
COPY targets/bmv2/handlers/ handlers/
COPY targets/bmv2/controllers/ controllers/
COPY targets/bmv2/http/ http/
COPY targets/bmv2/managers/ managers/
COPY targets/bmv2/pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o driver cmd/main.go

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

# BMv2 driver image
# p4lang/p4c:latest provides p4c at runtime for compiling .p4 source files.
# libboost libraries are required by p4c's runtime dependencies.
FROM p4lang/p4c:1.2.5.13 AS bmv2
RUN apt-get update && apt-get install -y --no-install-recommends \
    libboost-iostreams-dev \
    libboost-graph-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /
COPY --from=bmv2-builder /workspace/driver .
ENTRYPOINT ["/driver"]



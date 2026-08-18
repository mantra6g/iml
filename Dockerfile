ARG GOVERSION=1.26.1-alpine
ARG ENVTEST_K8S_VERSION=1.34.1
ARG UBUNTUVERSION=24.04
ARG ALPINEVERSION=3.20
ARG P4CVERSION=1.2.5.15
#GOOS=${TARGETOS:-linux}
#GOARCH=${TARGETARCH}
ARG MOD=cni
ARG MODPATH=cni

# BASE

FROM golang:${GOVERSION} AS base

WORKDIR /workspace
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum api/
COPY cni/go.mod cni/go.sum cni/
COPY daemon/go.mod daemon/go.sum daemon/
COPY operator/go.mod operator/go.sum operator/
COPY tools/go.mod tools/go.sum tools/
COPY targets/bmv2/go.mod targets/bmv2/go.sum targets/bmv2/
COPY dpcs/go.mod dpcs/go.sum dpcs/
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    go mod download all

FROM base AS test-base
ARG ENVTEST_K8S_VERSION
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    go tool setup-envtest use ${ENVTEST_K8S_VERSION} --bin-dir /assets -p path

# PREBUILD

FROM ubuntu:${UBUNTUVERSION} AS daemon-ebpf-builder
RUN DEBIAN_FRONTEND=noninteractive \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY daemon/ebpf/ daemon/ebpf/

RUN ARCH=$(uname -m | sed 's/x86_64/x86_64-linux-gnu/') && \
    clang -O2 -g -target bpf \
    -I/usr/include/$ARCH \
    -I/usr/include \
    -c daemon/ebpf/src/recalc_csum.c -o daemon/ebpf/recalc_csum.o

FROM base AS daemon-prebuilder
COPY --from=daemon-ebpf-builder /workspace /workspace

FROM base AS operator-prebuilder
FROM base AS cni-prebuilder
FROM base AS targets-bmv2-prebuilder
FROM base AS dpcs-prebuilder
FROM ${MOD}-prebuilder AS prebuilder

# TEST

FROM prebuilder AS test-runner
ARG MOD
ARG MODPATH

ARG ENVTEST_K8S_VERSION
ENV GINKGO_NO_COLOR=TRUE

COPY --from=test-base /assets /assets
COPY api/ api/
COPY ${MODPATH}/ ${MODPATH}

RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    go mod download all && \
    KUBEBUILDER_ASSETS=$(go tool setup-envtest use -i ${ENVTEST_K8S_VERSION} --bin-dir /assets -p path) \
    GOPROXY=off \
    go tool gotestsum --no-color --junitfile ./artifacts/${MOD}.unit-tests.xml -- -coverprofile=coverage.out ./${MODPATH}/... || true && \
    go tool cover -html=./coverage.out -o ./artifacts/${MOD}.coverage.html && \
    go tool gocover-cobertura < ./coverage.out > ./artifacts/${MOD}.coverage.xml

FROM scratch AS test
COPY --from=test-runner /workspace/artifacts/ /

# BUILD

FROM prebuilder AS builder
ARG MODPATH

COPY api/ api/
COPY ${MODPATH}/ ${MODPATH}/
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    go mod download all && \
    GOPROXY=off CGO_ENABLED=0 /usr/local/go/bin/go build -o bin/ ./${MODPATH}/...

# RUNTIME

FROM alpine:${ALPINEVERSION} AS daemon-preruntime
RUN apk add --no-cache iptables iptables-legacy

FROM p4lang/p4c:${P4CVERSION} AS targets-bmv2-preruntime
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libboost-iostreams-dev \
    libboost-graph-dev \
    && rm -rf /var/lib/apt/lists/*

FROM gcr.io/distroless/static-debian13:nonroot AS operator-preruntime
USER 65532:65532

FROM gcr.io/distroless/static-debian13:nonroot AS dpcs-preruntime
USER 65532:65532

FROM alpine:${ALPINEVERSION} AS cni-preruntime

FROM ${MOD}-preruntime AS preruntime

FROM preruntime AS runtime
ARG BIN
WORKDIR /
COPY --from=builder /workspace/bin/${BIN} /app
ENTRYPOINT ["/app"]

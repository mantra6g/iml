ARG GOVERSION=1.25.3
ARG ENVTEST_K8S_VERSION=1.34.1
ARG UBUNTUVERSION=24.04
ARG ALPINEVERSION=3.20
#GOOS=${TARGETOS:-linux}
#GOARCH=${TARGETARCH}
ARG MOD=cni

# BASE

FROM golang:${GOVERSION} AS base

WORKDIR /workspace
COPY go.work ./
COPY api/go.mod api/go.sum api/
COPY cni/go.mod cni/go.sum cni/
COPY daemon/go.mod daemon/go.sum daemon/
COPY operator/go.mod operator/go.sum operator/
COPY tools/go.mod tools/go.sum tools/
#RUN --mount=type=cache,target=/go/pkg/mod \
RUN go mod download

FROM base AS test-base
ARG ENVTEST_K8S_VERSION
#RUN --mount=type=cache,target=/assets \
RUN go tool setup-envtest use ${ENVTEST_K8S_VERSION} --bin-dir /assets -p path

# BUILD

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

FROM base AS pre-daemon-builder
COPY --from=daemon-ebpf-builder /workspace /workspace

FROM base AS operator-prebuilder
FROM base AS cni-prebuilder
FROM pre-daemon-builder AS daemon-prebuilder
FROM ${MOD}-prebuilder AS prebuilder

FROM prebuilder AS builder
ARG MOD

COPY api/ api/
COPY ${MOD}/ ${MOD}/
#RUN --mount=type=cache,target=/go/pkg/mod \
#    --mount=type=cache,target=/root/.cache/go-build \
RUN GOPROXY=off CGO_ENABLED=0 go build -o bin/ ./${MOD}/...

# TEST

FROM prebuilder AS test-runner
ARG MOD
ARG ENVTEST_K8S_VERSION
ENV GINKGO_NO_COLOR=TRUE

COPY --from=test-base /assets /assets
COPY api/ api/
COPY ${MOD}/ ${MOD}/
#RUN --mount=type=cache,target=/go/pkg/mod \
#    --mount=type=cache,target=/root/.cache/go-build \
#    --mount=type=cache,target=/assets \
RUN KUBEBUILDER_ASSETS=$(go tool setup-envtest use -i ${ENVTEST_K8S_VERSION} --bin-dir /assets -p path) \
    GOPROXY=off go tool gotestsum --no-color --junitfile ./artifacts/${MOD}.unit-tests.xml -- -coverprofile=coverage.out ./${MOD}/... || true

RUN go tool cover -html=./coverage.out -o ./artifacts/${MOD}.coverage.html && \
    go tool gocover-cobertura < ./coverage.out > ./artifacts/${MOD}.coverage.xml

FROM scratch AS test
COPY --from=test-runner /workspace/artifacts/ /

# RUNTIME

FROM alpine:${ALPINEVERSION} AS pre-daemon-runtime
RUN apk add --no-cache iptables iptables-legacy

FROM scratch AS operator-preruntime
FROM scratch AS cni-preruntime
FROM pre-daemon-runtime AS daemon-preruntime
FROM ${MOD}-preruntime AS preruntime

FROM preruntime AS runtime
ARG BIN
COPY --from=builder /workspace/bin/${BIN} .
ENTRYPOINT ["/${BIN}"]

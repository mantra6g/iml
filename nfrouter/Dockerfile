FROM alpine:latest

RUN apk add --no-cache \
    build-base       \
    clang            \
    llvm-dev         \
    libbpf-dev       \
    linux-headers

WORKDIR /app

COPY . .

RUN make build

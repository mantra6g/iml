FROM golang:1.26 AS proxy-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download && mkdir bin

COPY cmd/main.go cmd/main.go
COPY pkg/ pkg/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/proxy cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache iptables ip6tables

WORKDIR /
COPY --from=proxy-builder /workspace/bin/proxy .
ENTRYPOINT ["/proxy"]

FROM golang:1.26 AS fw-controller-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download && mkdir bin

COPY cmd/main.go cmd/main.go
COPY reconcilers/ reconcilers/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/fw-controller cmd/main.go

FROM alpine:3.20

WORKDIR /
COPY --from=fw-controller-builder /workspace/bin/fw-controller .
ENTRYPOINT ["/fw-controller"]

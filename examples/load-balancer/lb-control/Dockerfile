FROM golang:1.26 AS lb-controller-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download && mkdir bin

COPY cmd/main.go cmd/main.go
COPY utils/ utils/
COPY reconcilers/ reconcilers/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/lb-controller cmd/main.go

FROM alpine:3.20

WORKDIR /
COPY --from=lb-controller-builder /workspace/bin/lb-controller .
ENTRYPOINT ["/lb-controller"]

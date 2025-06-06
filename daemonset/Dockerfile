FROM golang:alpine AS builder

WORKDIR /app
COPY . .

RUN go mod tidy
RUN GOOS=linux GOARCH=amd64 go build -o imldaemon

FROM alpine:latest
WORKDIR /app

COPY --from=builder /app/imldaemon .
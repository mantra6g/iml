FROM golang:alpine AS builder

WORKDIR /app
COPY . .

# Install build tools for CGO
RUN apk add --no-cache gcc musl-dev

RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o iml-daemon

ENTRYPOINT [ "./iml-daemon" ]
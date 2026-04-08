FROM golang:alpine AS builder

WORKDIR /app
COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o daemon

ENTRYPOINT [ "./daemon" ]

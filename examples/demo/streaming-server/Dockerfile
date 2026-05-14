FROM alpine:3.20

RUN apk add --no-cache tcpdump iperf3

ENTRYPOINT ["iperf3", "-s"]

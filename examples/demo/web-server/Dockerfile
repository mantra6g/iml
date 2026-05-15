FROM jrottenberg/ffmpeg:6.1-alpine

WORKDIR /app
COPY . /app

EXPOSE 3000/udp
EXPOSE 3000/tcp

CMD [ "-stream_loop", "-1", "-re", "-i", "fns2.mkv", "-c:v", "libx264", "-c:a", "aac", "-f", "mpegts", "-listen", "1", "http://[::]:3000" ]


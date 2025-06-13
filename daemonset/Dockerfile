FROM python:3.11-alpine AS builder

WORKDIR /app

COPY . .
RUN pip3 install -r requirements.txt

ENTRYPOINT ["python3", "proxy-daemon.py"]
FROM python:3.11-slim

# PYTHONDONTWRITEBYTECODE prevents Python from writing .pyc files to disk
# PYTHONUNBUFFERED ensures that Python output is sent straight to terminal (e.g.
# for logging) without being buffered, which is useful for real-time logs
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    libpq-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --upgrade pip && pip install --no-cache-dir -r requirements.txt

COPY . .

CMD [ "gunicorn", "--bind", "0.0.0.0:5000", "nfvo:app" ]
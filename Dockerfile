FROM python:3.12-slim

# Set environment variables to avoid interactive prompts during installs
ENV DEBIAN_FRONTEND=noninteractive

# Install curl, gnupg, and other required tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    iproute2 \
    curl \
    gnupg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Helm
RUN curl https://baltocdn.com/helm/signing.asc | gpg --dearmor -o /usr/share/keyrings/helm.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | \
    tee /etc/apt/sources.list.d/helm-stable-debian.list && \
    apt-get update && \
    apt-get install -y helm && \
    rm -rf /var/lib/apt/lists/*

# Set a working directory
WORKDIR /app

# Move the graph-charts to 
COPY graph-chart ./graph-chart
COPY requirements.txt .
COPY nfvo-api.py .

# Install dependencies
RUN pip install -r requirements.txt

# Default command
ENTRYPOINT ["python3", "nfvo-api.py"]
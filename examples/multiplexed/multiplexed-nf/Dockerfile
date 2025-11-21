# A first stage to build the P4 program using p4c
FROM p4lang/p4c:1.2.5.9 AS build

WORKDIR /p4app

COPY multiplexed.p4 .

# Install missing dependency for p4c
RUN apt-get update && apt-get install -y \
    libboost-iostreams1.71.0

RUN p4c \
    --target bmv2 \
    --arch v1model \
    --std p4-16 \
    multiplexed.p4 \
    --p4runtime-files build/multiplexed.p4info.txt \
    --output build

# The final stage to run the control-plane program with the compiled P4
FROM python:3.11-slim

WORKDIR /app

COPY --from=build /p4app/build/ ./
COPY requirements.txt .
COPY control-plane.py .

RUN pip install --no-cache-dir -r requirements.txt

ENV PYTHONUNBUFFERED=1

# Entrypoint is a control plane that loads the P4 program into the switch via p4runtime
ENTRYPOINT ["python3", "-u", "control-plane.py",     \
                "--p4info", "multiplexed.p4info.txt", \
                "--json", "multiplexed.json",  \
                "--main-interface", "iml0", \
                "--sleep", "60"]

# ENTRYPOINT [ "simple_switch_grpc", "--no-p4", "-i 1@iml0", "--log-console", "--", "--grpc-server-addr", "0.0.0.0:9559", "--cpu-port", "111"]

# This CMD can be overridden to specify different interfaces. 
# The default is to use 1@in0 and 2@out0
# CMD ["-i 1@iml0"]
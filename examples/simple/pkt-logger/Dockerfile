FROM p4lang/p4c:1.2.5.9 AS build

WORKDIR /p4app

COPY . .

# Install missing dependency for p4c
RUN apt-get update && apt-get install -y \
    libboost-iostreams1.71.0

RUN p4c \
    --target bmv2 \
    --arch v1model \
    --std p4-16 \
    logger.p4 -o build/

# The entrypoint ALWAYS runs the simple_switch with the generated JSON file
ENTRYPOINT [ "simple_switch", \
                "build/logger.json", \
                "--log-level", "info", \
                "--log-console"]

# This CMD can be overridden to specify different interfaces. 
# The default is to use 1@in0 and 2@out0
CMD ["-i 1@iml0"]
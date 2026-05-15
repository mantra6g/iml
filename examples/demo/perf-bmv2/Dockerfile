ARG PARENT_VERSION=latest
FROM p4lang/pi:${PARENT_VERSION}
ARG BMV2_VER=1.15.2
ARG IMAGE_TYPE=build

ARG CC=gcc
ARG CXX=g++
ARG sswitch_grpc=yes

ENV BM_DEPS automake \
            build-essential \
            clang-8 \
            clang-10 \
            cmake \
            curl \
            git \
            lcov \
            libgmp-dev \
            libpcap-dev \
            libboost-dev \
            libboost-program-options-dev \
            libboost-system-dev \
            libboost-filesystem-dev \
            libboost-thread-dev \
            libtool \
            pkg-config

ENV BM_RUNTIME_DEPS libboost-program-options1.71.0 \
                    libboost-system1.71.0 \
                    libboost-filesystem1.71.0 \
                    libboost-thread1.71.0 \
                    libgmp10 \
                    libpcap0.8 \
                    python3 \
                    python-is-python3 \
                    tzdata \
                    vim \
                    ca-certificates \
                    build-essential \
                    curl \
                    git \
                    iproute2 \
                    iputils-ping \
                    traceroute \
                    tcpdump \
                    iperf3 \
                    dnsutils \
                    net-tools \
                    ethtool

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends $BM_DEPS $BM_RUNTIME_DEPS

RUN git clone --depth 1 --branch ${BMV2_VER} https://github.com/p4lang/behavioral-model.git  /behavioral-model/
WORKDIR /behavioral-model/

RUN ./autogen.sh && \
    ./configure 'CXXFLAGS=-g -O3' 'CFLAGS=-g -O3' --disable-logging-macros --disable-elogger --with-pdfixed --with-pi --disable-debugger --with-thrift && \
    make -j$(nproc) && \
    make install-strip && \
    ldconfig && \
    apt-get purge -y $BM_DEPS && \
    apt-get autoremove -y --purge && \
    rm -rf /behavioral-model /var/cache/apt/* /var/lib/apt/lists/*


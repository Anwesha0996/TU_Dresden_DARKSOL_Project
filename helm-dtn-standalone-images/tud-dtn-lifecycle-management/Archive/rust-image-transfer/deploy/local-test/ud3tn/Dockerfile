# µD3TN node image for the local test harness.
#
# Builds the µD3TN daemon (`ud3tn`) plus its official Python AAP2 tools
# (`aap2-config`, `aap2-send`, `aap2-receive`, `aap2-bdm-ud3tn-routing`, ...)
# from a pinned upstream tag, mirroring upstream's own
# `dockerfiles/ud3tn-with-python-tools` but cloning the pinned ref instead of
# building from a local checkout (we don't vendor the ud3tn C source tree in
# this repo).
#
# Source: https://gitlab.com/d3tn/ud3tn
# Pinned: tag v0.15.0, commit b27c1e28a14ed3c75bf96890f793638fc2f168fb
# See: ../../../doc/rust-image-transfer/00-aap2-notes.md
#
# `iproute2` and `iptables` are included so `tc`/netem-based link
# disruption can be injected from inside the container (see ../README.md).

FROM python:3.13

ARG UD3TN_REF=v0.15.0

RUN apt-get update && \
    apt-get dist-upgrade -y && \
    apt-get install -y --no-install-recommends \
        git \
        build-essential \
        sqlite3 \
        libsqlite3-dev \
        libjansson-dev \
        iproute2 \
        iptables \
        iputils-ping \
        ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN git clone --branch "${UD3TN_REF}" --depth 1 https://gitlab.com/d3tn/ud3tn.git /ud3tn \
    && cd /ud3tn && git submodule update --init --recursive --depth 1

# No ARCH= override: per config.mk.example, omitting ARCH makes the build use
# `-march=native` on x86/x86_64 and skip -march entirely elsewhere (e.g. arm64),
# which is what we want since this image is built for whatever the host/CI
# architecture actually is (upstream's own Dockerfile hardcodes ARCH=x86-64,
# which fails to compile on arm64 hosts such as Apple Silicon).
RUN cd /ud3tn \
    && make posix posix-lib -j4 \
    && cp /ud3tn/build/posix/ud3tn /usr/local/bin/ud3tn -v \
    && cp /ud3tn/build/posix/libud3tn.so /usr/local/lib/libud3tn.so -v \
    && make clean

RUN pip install --no-cache-dir /ud3tn/pyd3tn /ud3tn/python-ud3tn-utils

ENTRYPOINT [ "ud3tn" ]

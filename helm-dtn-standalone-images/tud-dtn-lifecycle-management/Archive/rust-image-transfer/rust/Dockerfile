# Builds both aap2-image-sender and aap2-image-receiver from the workspace, with
# skopeo installed for runtime (both binaries shell out to it). Used by
# deploy/demo/docker-compose.yml (Phase 9) to run the Rust path as real
# containers rather than host processes, so the space-side container(s)
# can be network-isolated the same way earth-node/space-node are.
FROM rust:1-slim-bookworm AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
        pkg-config libssl-dev protobuf-compiler ca-certificates && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY . .
RUN cargo build --locked --release -p aap2-image-sender -p aap2-image-receiver

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        skopeo iproute2 iptables iputils-ping ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/target/release/aap2-image-sender /usr/local/bin/aap2-image-sender
COPY --from=builder /build/target/release/aap2-image-receiver /usr/local/bin/aap2-image-receiver

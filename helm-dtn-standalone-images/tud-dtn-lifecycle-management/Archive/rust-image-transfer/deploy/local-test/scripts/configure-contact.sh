#!/usr/bin/env bash
# Configures a µD3TN contact so earth-node knows it can reach space-node.
#
# Contacts are runtime-only state (µD3TN does not persist them across
# restarts - see doc/rust-image-transfer/00-aap2-notes.md and the µD3TN
# posix_quick_start_guide.md), so this needs to be re-run any time
# earth-node is restarted/recreated (`docker compose up` after a rebuild,
# `docker compose restart earth-node`, etc.) before any bundle will move.
#
# Usage: ./configure-contact.sh [duration_seconds]
set -euo pipefail
DURATION="${1:-3600}"

echo "==> Configuring contact: earth-node -> space-node (tcpclv3), duration=${DURATION}s"
docker exec earth-node aap2-config \
    --tcp 127.0.0.1 4242 \
    -s 1 "${DURATION}" 100000 \
    dtn://space.dtn/ tcpclv3:space-node:4556

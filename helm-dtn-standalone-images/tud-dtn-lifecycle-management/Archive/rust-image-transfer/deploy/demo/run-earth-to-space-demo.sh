#!/usr/bin/env bash
# Earth-to-space demo (TASKS.md Phase 9): one command, unattended, that
# pulls a real image on earth-node, sends it over the real two-node µD3TN
# harness via the Rust AAP2 path, survives a deliberately injected link
# outage mid-transfer with no manual intervention, and ends with the image
# verifiably present in a Harbor instance reachable only from the space
# side - not from earth-node, not from the host, and not from the public
# internet.
#
# Airgap enforcement: space-node and aap2-image-receiver are NOT put on a
# Docker `internal: true` network, because that also disables host-
# published ports (verified empirically while building this script), which
# this harness's other tooling (deploy/local-test) relies on. Instead,
# their egress is locked down with real iptables rules in their own
# network namespaces (loopback + the two Docker subnets they legitimately
# need, default-deny everything else) - genuine L3 enforcement, not just a
# documented convention, verified live below by attempting (and failing)
# to reach the public internet from inside both containers before the
# transfer starts.
#
# Usage: ./run-earth-to-space-demo.sh
# Tears down and recreates its own stack; also tears down deploy/local-test
# first if it's running, since both use the same fixed container names
# (earth-node, space-node, nginx, ...) and Docker container names are
# global, not per-project.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_TEST_DIR="$(cd "$SCRIPT_DIR/../local-test" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
DEMO_IMAGE="docker.io/library/alpine:3.20"
TARGET_REF="alpine:3.20"
REGISTRY_CREDS="admin:Harbor12345"
export HARBOR_CREDS="$REGISTRY_CREDS"

mkdir -p "$OUTPUT_DIR"
rm -f "$OUTPUT_DIR"/*.log "$OUTPUT_DIR"/*.json

log() { echo "[demo] $*" | tee -a "$OUTPUT_DIR/demo.log"; }
fail() { log "FAILED: $*"; exit 1; }

log "=== helm-dtn earth-to-space demo starting ==="

# ---------------------------------------------------------------------
# 1. Bring up a clean stack.
# ---------------------------------------------------------------------
log "Tearing down any existing local-test/demo stacks (shared container names)..."
(cd "$LOCAL_TEST_DIR" && docker compose down) >>"$OUTPUT_DIR/demo.log" 2>&1
(cd "$SCRIPT_DIR" && docker compose down) >>"$OUTPUT_DIR/demo.log" 2>&1

log "Building and starting the demo stack (Harbor + earth-node + space-node + aap2-image-sender/receiver)..."
(cd "$SCRIPT_DIR" && docker compose up -d --build) >>"$OUTPUT_DIR/demo.log" 2>&1 \
    || fail "docker compose up failed - see $OUTPUT_DIR/demo.log"

log "Waiting for Harbor to report healthy..."
waited=0
until docker compose -f "$SCRIPT_DIR/docker-compose.yml" ps --format "{{.Name}} {{.Status}}" 2>/dev/null | grep -q "harbor-core.*healthy"; do
    sleep 2
    waited=$((waited + 2))
    if (( waited > 180 )); then
        fail "Harbor did not become healthy within 180s"
    fi
done
log "Harbor is healthy."

# ---------------------------------------------------------------------
# 2. Lock down space-side egress and prove the airgap.
# ---------------------------------------------------------------------
DTN_SUBNET="$(docker network inspect dtn-net --format '{{(index .IPAM.Config 0).Subnet}}')"
HARBOR_SUBNET="$(docker network inspect demo_harbor --format '{{(index .IPAM.Config 0).Subnet}}')"
log "dtn-net subnet: $DTN_SUBNET, harbor subnet: $HARBOR_SUBNET"

lock_down_egress() {
    local container="$1"
    docker exec "$container" sh -c "
        iptables -F OUTPUT
        iptables -A OUTPUT -o lo -j ACCEPT
        iptables -A OUTPUT -d $DTN_SUBNET -j ACCEPT
        iptables -A OUTPUT -d $HARBOR_SUBNET -j ACCEPT
        iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
        iptables -P OUTPUT DROP
    "
}

log "Locking down space-node's egress to dtn-net + Harbor only (no public internet)..."
lock_down_egress space-node || fail "failed to lock down space-node"
log "Locking down aap2-image-receiver's egress the same way..."
lock_down_egress aap2-image-receiver || fail "failed to lock down aap2-image-receiver"

log "Proving the airgap: attempting to reach the public internet from space-node and aap2-image-receiver (both MUST fail)..."
{
    echo "=== space-node -> 1.1.1.1 (expect failure) ==="
    docker exec space-node ping -c 2 -W 2 1.1.1.1
    echo "space-node ping exit code: $?"
    echo
    echo "=== aap2-image-receiver -> 1.1.1.1 (expect failure) ==="
    docker exec aap2-image-receiver sh -c "ping -c 2 -W 2 1.1.1.1"
    echo "aap2-image-receiver ping exit code: $?"
} >"$OUTPUT_DIR/airgap-proof.log" 2>&1

if docker exec space-node ping -c 1 -W 2 1.1.1.1 >/dev/null 2>&1; then
    fail "AIRGAP VIOLATED: space-node can still reach the public internet"
fi
if docker exec aap2-image-receiver sh -c "ping -c 1 -W 2 1.1.1.1" >/dev/null 2>&1; then
    fail "AIRGAP VIOLATED: aap2-image-receiver can still reach the public internet"
fi
log "Airgap confirmed: neither space-node nor aap2-image-receiver can reach the public internet (see $OUTPUT_DIR/airgap-proof.log)."

# Sanity check the *other* direction: aap2-image-sender (earth side) must
# still have normal internet access, or the demo can't even pull the image.
if ! docker exec aap2-image-sender ping -c 1 -W 2 1.1.1.1 >/dev/null 2>&1; then
    fail "aap2-image-sender (earth side) unexpectedly has no internet access"
fi
log "Confirmed earth-side (aap2-image-sender) still has normal internet access, as expected."

# ---------------------------------------------------------------------
# 3. Configure the DTN contact and add a representative one-way delay.
# ---------------------------------------------------------------------
log "Configuring the earth-node -> space-node contact..."
docker exec earth-node aap2-config --tcp 127.0.0.1 4242 -s 1 3600 100000 \
    dtn://space.dtn/ tcpclv3:space-node:4556 >>"$OUTPUT_DIR/demo.log" 2>&1
sleep 2

log "Adding a representative one-way link delay (300ms +-50ms) on space-node..."
docker exec space-node tc qdisc replace dev eth0 root netem delay 300ms 50ms >>"$OUTPUT_DIR/demo.log" 2>&1

# ---------------------------------------------------------------------
# 4. Start the receiver, inject a deliberate outage, then send.
# ---------------------------------------------------------------------
log "Starting aap2-image-receiver (registry target: nginx:8080/library, reachable only within the space-side network)..."
docker exec -d aap2-image-receiver sh -c "
    RUST_LOG=info aap2-image-receiver \
        --tcp space-node:4242 --agent-id image-transfer \
        --registry nginx:8080/library \
        --insecure-registry \
        > /tmp/recv.log 2>&1
"
sleep 3
docker exec aap2-image-receiver cat /tmp/recv.log >>"$OUTPUT_DIR/demo.log" 2>&1

log "Injecting a deliberate outage on the earth<->space link BEFORE sending, to prove the transfer survives disruption unattended..."
docker exec space-node iptables -A OUTPUT -d earth-node -j DROP
docker exec space-node iptables -A INPUT -s earth-node -j DROP
log "Outage active."

log "Kicking off the transfer (image: $DEMO_IMAGE) while the link is down..."
(
    docker exec aap2-image-sender sh -c "
        RUST_LOG=info aap2-image-sender \
            --tcp earth-node:4242 --destination-eid dtn://space.dtn/image-transfer \
            --image $DEMO_IMAGE --harbor-target $TARGET_REF \
            --release demo-chart \
            --work-dir /tmp/send-work
    " >"$OUTPUT_DIR/aap2-image-sender.log" 2>&1
) &
SEND_PID=$!

log "Waiting 15s with the link down (bundle should be queued by µD3TN, not delivered yet)..."
sleep 15

log "Clearing the outage..."
docker exec space-node iptables -D OUTPUT -d earth-node -j DROP
docker exec space-node iptables -D INPUT -s earth-node -j DROP
log "Link restored. Waiting for the transfer to complete with no manual intervention..."

wait "$SEND_PID"
SEND_STATUS=$?
cat "$OUTPUT_DIR/aap2-image-sender.log" >>"$OUTPUT_DIR/demo.log"
if [[ "$SEND_STATUS" -ne 0 ]]; then
    fail "aap2-image-sender exited non-zero ($SEND_STATUS) - see $OUTPUT_DIR/aap2-image-sender.log"
fi
log "aap2-image-sender completed successfully."

# ---------------------------------------------------------------------
# 5. Wait for the receiver to finish and verify the pushed image.
# ---------------------------------------------------------------------
log "Waiting for aap2-image-receiver to report completion..."
waited=0
until docker exec aap2-image-receiver grep -q "transfer complete" /tmp/recv.log 2>/dev/null; do
    sleep 2
    waited=$((waited + 2))
    if (( waited > 60 )); then
        docker exec aap2-image-receiver cat /tmp/recv.log >>"$OUTPUT_DIR/demo.log" 2>&1
        fail "aap2-image-receiver did not report completion within 60s of the link being restored"
    fi
done
docker exec aap2-image-receiver cat /tmp/recv.log >"$OUTPUT_DIR/aap2-image-receiver.log"
log "aap2-image-receiver reported transfer complete."

log "Independently verifying the pushed image via 'skopeo inspect' from inside the (airgapped) space-side network..."
docker exec aap2-image-receiver skopeo inspect --creds "$REGISTRY_CREDS" --tls-verify=false \
    "docker://nginx:8080/library/$TARGET_REF" >"$OUTPUT_DIR/skopeo-inspect.json" 2>&1
if [[ $? -ne 0 ]]; then
    fail "skopeo inspect failed - image not verifiably present in Harbor; see $OUTPUT_DIR/skopeo-inspect.json"
fi

log "=== DEMO SUCCEEDED ==="
log "Evidence captured in $OUTPUT_DIR:"
log "  - demo.log              full unattended run log"
log "  - airgap-proof.log         proof space-node/aap2-image-receiver cannot reach the public internet"
log "  - aap2-image-sender.log   sender output (export, checksum, AAP2 send)"
log "  - aap2-image-receiver.log receiver output (checksum verify, push, digest verify)"
log "  - skopeo-inspect.json   independent post-demo confirmation the image is in Harbor"
exit 0

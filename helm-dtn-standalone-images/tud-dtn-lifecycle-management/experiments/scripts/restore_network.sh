#!/usr/bin/env bash
set -euo pipefail

state_file=${INTERRUPTION_STATE_FILE:?INTERRUPTION_STATE_FILE is required}
[[ -s "$state_file" ]] || { echo "interruption state is missing: $state_file" >&2; exit 1; }
space_ip=$(cat "$state_file")
earth_container=${EARTH_CONTAINER:-earth-node}
space_tcpcl_host=${SPACE_TCPCL_HOST:-${SPACE_CONTAINER:-space-node}}
contact_capacity=${CONTACT_CAPACITY_BYTES:-1000000000}
post_contact_settle=${POST_CONTACT_SETTLE_SECONDS:-15}
tools_image=${NETWORK_TOOLS_IMAGE:-dtn-experiment-nettools:latest}
docker run --rm --network "container:$earth_container" --cap-add NET_ADMIN "$tools_image" \
  iptables -D OUTPUT -p tcp -d "$space_ip" --dport 4556 -j DROP
docker exec "$earth_container" aap2-config --tcp 127.0.0.1 4242 \
  -s 1 7200 "$contact_capacity" dtn://space.dtn/ "tcpclv3:$space_tcpcl_host:4556"
# A scheduled contact is known to the router before TCPCL has necessarily
# completed its handshake. Recalling SQLite earlier can make the BPA discard
# the injected bundle as "link inactive", especially with Lunar RTT.
sleep "$post_contact_settle"
# Explicitly recall disk-backed bundles after the restored contact is known.
# For short outages, writing a large ADU to SQLite may finish after the link is
# restored, so wait for a stored row instead of issuing the recall too early.
for _ in $(seq 1 120); do
  stored=$(docker exec "$earth_container" sqlite3 /data/earth.sqlite \
    'SELECT COUNT(*) FROM bundles;' 2>/dev/null || echo 0)
  [[ "${stored:-0}" -gt 0 ]] && break
  sleep 1
done
docker exec "$earth_container" aap2-storage-agent --tcp 127.0.0.1 4242 \
  --storage-agent-eid dtn://earth.dtn/sqlite push --dest-eid-glob 'dtn://space.dtn/*'
rm -f "$state_file"

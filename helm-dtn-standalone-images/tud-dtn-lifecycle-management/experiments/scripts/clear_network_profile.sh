#!/usr/bin/env bash
set -euo pipefail

tools_image=${NETWORK_TOOLS_IMAGE:-dtn-experiment-nettools:latest}
earth=${EARTH_CONTAINER:-earth-node}
space=${SPACE_CONTAINER:-space-node}
state=${NETWORK_PROFILE_STATE_FILE:-}
earth_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' "$earth")
space_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' "$space")

clear_one() {
  local container=$1 peer_ip=$2
  docker run --rm --network "container:$container" --cap-add NET_ADMIN "$tools_image" sh -eu -c '
    iface=$(ip route get "$1" | sed -n "s/.* dev \([^ ]*\).*/\1/p" | head -n1)
    test -z "$iface" || tc qdisc del dev "$iface" root 2>/dev/null || true
  ' sh "$peer_ip"
}
clear_one "$earth" "$space_ip"
clear_one "$space" "$earth_ip"
[[ -z "$state" ]] || rm -f "$state"


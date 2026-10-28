#!/usr/bin/env bash
set -euo pipefail

delay=${NETWORK_ONE_WAY_DELAY:?NETWORK_ONE_WAY_DELAY is required}
rate=${NETWORK_BANDWIDTH:?NETWORK_BANDWIDTH is required}
loss=${NETWORK_PACKET_LOSS:-0%}
tools_image=${NETWORK_TOOLS_IMAGE:-dtn-experiment-nettools:latest}
earth=${EARTH_CONTAINER:-earth-node}
space=${SPACE_CONTAINER:-space-node}
state=${NETWORK_PROFILE_STATE_FILE:?NETWORK_PROFILE_STATE_FILE is required}

earth_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' "$earth")
space_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' "$space")
[[ -n "$earth_ip" && -n "$space_ip" ]] || { echo "cannot resolve dtn-net node addresses" >&2; exit 1; }

apply_one() {
  local container=$1 peer_ip=$2
  docker run --rm --network "container:$container" --cap-add NET_ADMIN "$tools_image" sh -eu -c '
    peer=$1; delay=$2; rate=$3; loss=$4
    iface=$(ip route get "$peer" | sed -n "s/.* dev \([^ ]*\).*/\1/p" | head -n1)
    test -n "$iface"
    tc qdisc del dev "$iface" root 2>/dev/null || true
    tc qdisc add dev "$iface" root handle 1: prio
    # The large queue is required for the bandwidth-delay product of the Mars
    # profile; the default 1000 packets would cause unintended packet loss.
    tc qdisc add dev "$iface" parent 1:3 handle 30: netem limit 500000 delay "$delay" rate "$rate" loss "$loss"
    tc filter add dev "$iface" protocol ip parent 1:0 prio 3 u32 match ip dst "$peer/32" flowid 1:3
    tc -s qdisc show dev "$iface"
  ' sh "$peer_ip" "$delay" "$rate" "$loss"
}

apply_one "$earth" "$space_ip"
apply_one "$space" "$earth_ip"
printf 'earth_ip=%s\nspace_ip=%s\ndelay=%s\nrate=%s\nloss=%s\n' "$earth_ip" "$space_ip" "$delay" "$rate" "$loss" >"$state"

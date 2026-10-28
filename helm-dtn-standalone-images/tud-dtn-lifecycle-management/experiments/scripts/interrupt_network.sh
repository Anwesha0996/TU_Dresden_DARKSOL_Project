#!/usr/bin/env bash
set -euo pipefail

earth_container=${EARTH_CONTAINER:-earth-node}
space_container=${SPACE_CONTAINER:-space-node}
state_file=${INTERRUPTION_STATE_FILE:?INTERRUPTION_STATE_FILE is required}
tools_image=${NETWORK_TOOLS_IMAGE:-dtn-experiment-nettools:latest}
space_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' "$space_container")
[[ -n "$space_ip" ]] || { echo "cannot determine Space dtn-net IP" >&2; exit 1; }

# Enter Earth's network namespace and block only TCPCLv3 to Space. AAP2 and all
# sender, receiver, and µD3TN processes remain running.
docker run --rm --network "container:$earth_container" --cap-add NET_ADMIN "$tools_image" \
  iptables -I OUTPUT 1 -p tcp -d "$space_ip" --dport 4556 -j DROP
printf '%s\n' "$space_ip" >"$state_file"

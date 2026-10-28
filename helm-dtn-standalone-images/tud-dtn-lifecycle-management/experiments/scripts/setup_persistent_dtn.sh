#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
data_dir="$repo_dir/experiments/store-forward/persistent"
mkdir -p "$data_dir"

docker rm -f sf-earth-node sf-space-node >/dev/null 2>&1 || true

# Each measurement starts with an empty queue. Preserve any previous database
# for diagnosis instead of deleting it or mixing old bundles into a new run.
if [[ -f "$data_dir/earth.sqlite" ]]; then
  mv "$data_dir/earth.sqlite" "$data_dir/earth.previous.$(date -u +%Y%m%dT%H%M%SZ).sqlite"
fi

docker run -d --name sf-earth-node --hostname sf-earth-node \
  --network dtn-net --cap-add NET_ADMIN -p 34242:4242 \
  -v "$data_dir:/data" local-test-earth-node \
  --node-id dtn://earth.dtn/ --aap2-host 0.0.0.0 --aap2-port 4242 \
  --cla 'sqlite:/data/earth.sqlite;tcpclv3:*,4556' \
  --lifetime 86400 --log-level 4 --allow-remote-config >/dev/null

docker run -d --name sf-space-node --hostname sf-space-node \
  --network dtn-net --cap-add NET_ADMIN -p 44242:4242 \
  local-test-space-node \
  --node-id dtn://space.dtn/ --aap2-host 0.0.0.0 --aap2-port 4242 \
  --cla 'tcpclv3:*,4556' --log-level 4 --allow-remote-config >/dev/null

# Opening an existing SQLite file can take several seconds for a large stored
# bundle. Do not continue until the AAP2 listener is actually accepting.
for _ in $(seq 1 30); do
  if docker exec sf-earth-node bash -lc 'exec 3<>/dev/tcp/127.0.0.1/4242' >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
sleep 1

docker ps --filter name=sf- --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'

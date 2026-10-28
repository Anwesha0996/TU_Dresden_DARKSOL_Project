#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/.." && pwd)
images_file=${IMAGES_FILE:-$repo_dir/examples/talos-v1.13.6-kubernetes-v1.36.2-images.txt}
registry=${REGISTRY:-127.0.0.1:5001/library}
earth_aap2=${EARTH_AAP2:-127.0.0.1:14242}
space_aap2=${SPACE_AAP2:-127.0.0.1:24242}
destination_eid=${DESTINATION_EID:-dtn://space.dtn/image-transfer}
contact_duration=${CONTACT_DURATION:-3600}
contact_rate=${CONTACT_RATE:-10000000}

mkdir -p "$repo_dir/evidence"
receiver_log="$repo_dir/evidence/receiver.log"
sender_log="$repo_dir/evidence/sender.log"
verify_log="$repo_dir/evidence/verify.log"
rm -f "$receiver_log" "$sender_log" "$verify_log"

docker start earth-node space-node go-demo-registry >/dev/null
docker exec earth-node aap2-config \
  --tcp 127.0.0.1 4242 \
  -s 1 "$contact_duration" "$contact_rate" \
  dtn://space.dtn/ tcpclv3:space-node:4556

REGISTRY_CREDS=${REGISTRY_CREDS:-demo:demo} "$repo_dir/bin/image-transfer-receive" \
  -tcp "$space_aap2" -registry "$registry" -insecure-registry \
  -work-dir /tmp/standalone-image-receiver >"$receiver_log" 2>&1 &
receiver_pid=$!
trap 'kill "$receiver_pid" 2>/dev/null || true' EXIT
sleep 2

"$repo_dir/bin/image-transfer-send" \
  -tcp "$earth_aap2" -destination-eid "$destination_eid" \
  -images-file "$images_file" -arch amd64 2>&1 | tee "$sender_log"

expected=$(grep -Ev '^[[:space:]]*(#|$)' "$images_file" | wc -l)
for _ in $(seq 1 1800); do
  completed=$(grep -c "transfer complete" "$receiver_log" || true)
  if [ "$completed" -ge "$expected" ]; then break; fi
  sleep 1
done

completed=$(grep -c "transfer complete" "$receiver_log" || true)
if [ "$completed" -ne "$expected" ]; then
  cat "$receiver_log"
  echo "received $completed successful registry pushes, expected $expected" >&2
  exit 1
fi

REGISTRY_CREDS=${REGISTRY_CREDS:-demo:demo} "$repo_dir/bin/image-transfer-verify" \
  -images-file "$images_file" -registry "$registry" \
  -insecure-registry -arch amd64 2>&1 | tee "$verify_log"

cat "$receiver_log"
echo "acceptance complete: $completed/$expected images transferred and digest-verified"

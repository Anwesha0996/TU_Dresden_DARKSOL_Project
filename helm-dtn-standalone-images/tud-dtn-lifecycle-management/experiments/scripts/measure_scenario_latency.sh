#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
scenario=${1:?usage: measure_scenario_latency.sh leo|lunar|mars}
case "$scenario" in
  leo) rtt=50ms; delay=25ms; rate=6mbit; timeout=10 ;;
  lunar) rtt=2.6s; delay=1.3s; rate=20mbit; timeout=15 ;;
  mars) rtt=1800s; delay=900s; rate=5mbit; timeout=2100 ;;
  *) echo "scenario must be leo, lunar, or mars" >&2; exit 2 ;;
esac

work="$repo_dir/experiments/scenarios/work/latency-$scenario"
state="$work/network-profile.state"
result="$repo_dir/experiments/scenarios/results/latency_validation.csv"
mkdir -p "$work" "$(dirname "$result")"
cleanup() {
  if [[ -s "$state" ]]; then
    NETWORK_PROFILE_STATE_FILE="$state" "$repo_dir/experiments/scripts/clear_network_profile.sh" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker restart earth-node space-node >/dev/null
sleep 2
NETWORK_PROFILE_STATE_FILE="$state" NETWORK_ONE_WAY_DELAY="$delay" \
  NETWORK_BANDWIDTH="$rate" NETWORK_PACKET_LOSS=0% \
  "$repo_dir/experiments/scripts/apply_network_profile.sh" >"$work/network-profile.log" 2>&1
space_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' space-node)
start=$(date -u +%FT%T.%3NZ)
set +e
output=$(docker run --rm --network container:earth-node dtn-experiment-nettools:latest \
  ping -c 1 -W "$timeout" "$space_ip" 2>&1)
status=$?
set -e
finish=$(date -u +%FT%T.%3NZ)
measured=$(sed -n 's/.*time=\([0-9.]*\) ms.*/\1/p' <<<"$output" | tail -n1)
success=false; [[ "$status" -eq 0 && -n "$measured" ]] && success=true

python3 - "$result" "$scenario" "$rtt" "$delay" "$rate" "$start" "$finish" "$measured" "$success" <<'PY'
import csv, os, sys
path, values = sys.argv[1], sys.argv[2:]
header = ["network_scenario","configured_rtt","configured_one_way_delay","configured_bandwidth","start_time","finish_time","measured_rtt_ms","success"]
new = not os.path.exists(path) or os.path.getsize(path) == 0
with open(path, "a", newline="", encoding="utf-8") as f:
    w = csv.writer(f)
    if new: w.writerow(header)
    w.writerow(values)
PY
printf '%s\n' "$output" >"$work/ping.log"
echo "scenario=$scenario configured_rtt=$rtt measured_rtt_ms=${measured:-missing} success=$success"
[[ "$success" == true ]]


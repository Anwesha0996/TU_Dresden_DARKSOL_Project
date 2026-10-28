#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
image=${1:-flannel}
scenario=${2:-leo}
duration=${3:-10}
repetition=${4:-1}

[[ "$scenario" == leo || "$scenario" == lunar || "$scenario" == mars ]] || { echo 'scenario must be leo, lunar, or mars' >&2; exit 2; }
[[ "$duration" =~ ^[1-9][0-9]*$ ]] || { echo 'duration must be positive seconds' >&2; exit 2; }

case "$scenario" in
  leo) rtt=50ms; delay=25ms; rate=15mbit; source='LEO measurement profile; persistent DTN store-forward' ;;
  lunar) rtt=2.6s; delay=1.3s; rate=20mbit; source='Lunar propagation profile; persistent DTN store-forward' ;;
  mars)
    # This is explicitly a contact-only profile. Record what tc/netem actually
    # applies; keep the 1800 s Mars RTT only as reference provenance.
    rtt=0ms
    delay=0ms
    rate=5mbit
    source='Mars contact-disruption feasibility profile: 5 Mbit/s and DTN outage applied; literal 900 s one-way delay omitted because installed TCPCL cannot establish under that delay'
    ;;
esac

run_id=$(printf '%s_%s_persistent_sf_%ss_run%02d' "$image" "$scenario" "$duration" "$repetition")
base="$repo_dir/experiments/store-forward"
mkdir -p "$base/results" "$base/logs/$scenario" "$base/work/$run_id"

# Keep the receiver alive well beyond the planned outage. Windows sleep or WSL
# suspension extends real wall-clock interruption time even though the bundle
# remains safely stored in SQLite.
EARTH_CONTAINER=sf-earth-node SPACE_CONTAINER=sf-space-node SPACE_TCPCL_HOST=sf-space-node \
EARTH_AAP2=127.0.0.1:34242 SPACE_AAP2=127.0.0.1:44242 RESTART_DTN_NODES=false \
INITIAL_CLA_ADDRESS=sqlite: \
NETWORK_SCENARIO="$scenario" NETWORK_RTT="$rtt" NETWORK_ONE_WAY_DELAY="$delay" \
NETWORK_BANDWIDTH="$rate" NETWORK_PACKET_LOSS=0% NETWORK_PROFILE_SOURCE="$source" \
INTERRUPT_DURATION_SECONDS="$duration" INTERRUPT_AFTER_SECONDS=0 \
CONTACT_START_SECONDS=1 CONTACT_DURATION_SECONDS="$((duration + 2))" \
COMPLETION_TIMEOUT_SECONDS="$((duration + 86400))" RUN_ID_OVERRIDE="$run_id" \
EXPERIMENT_ID="persistent-store-forward-$(date -u +%Y%m%dT%H%M%SZ)" \
RAW_RESULTS_FILE="$base/results/persistent_store_forward_results.csv" \
LOG_FILE="$base/logs/$scenario/$run_id.log" WORK_DIR="$base/work/$run_id" \
"$repo_dir/experiments/scripts/run_single.sh" "$image" interruption "$repetition"

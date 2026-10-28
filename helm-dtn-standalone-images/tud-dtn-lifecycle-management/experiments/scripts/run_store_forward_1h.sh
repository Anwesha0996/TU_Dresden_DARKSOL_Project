#!/usr/bin/env bash
set -euo pipefail

# Separate improved experiment: queue the original ADU at Earth µD3TN against
# a known future contact, hold the TCPCL network path down for one hour, then
# restore the path and announce a fresh contact. The sender is not run again.

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
image=${1:-flannel}
scenario=${2:-leo}
repetition=${3:-1}
duration=3600

[[ "$scenario" == leo || "$scenario" == lunar ]] || {
  echo "store-forward validation supports leo or lunar; literal Mars RTT cannot establish TCPCL" >&2
  exit 2
}

case "$scenario" in
  leo)   rtt=50ms; delay=25ms; rate=6mbit; source='Ma et al. Starlink measurement; scheduled-contact store-forward' ;;
  lunar) rtt=2.6s; delay=1.3s; rate=20mbit; source='DSN RTT + NASA LLCD uplink; scheduled-contact store-forward' ;;
esac

run_id=$(printf '%s_%s_storeforward%ss_run%02d' "$image" "$scenario" "$duration" "$repetition")
results_dir="$repo_dir/experiments/store-forward/results"
logs_dir="$repo_dir/experiments/store-forward/logs/$scenario"
work_dir="$repo_dir/experiments/store-forward/work/$run_id"
mkdir -p "$results_dir" "$logs_dir" "$work_dir"

# The initial route is deliberately scheduled after the outage. This gives the
# BPA a future route while preventing an immediate TCPCL dispatch. The standard
# restore helper also announces a fresh two-hour contact when the path returns.
NETWORK_SCENARIO="$scenario" NETWORK_RTT="$rtt" NETWORK_ONE_WAY_DELAY="$delay" \
NETWORK_BANDWIDTH="$rate" NETWORK_PACKET_LOSS=0% NETWORK_PROFILE_SOURCE="$source" \
INTERRUPT_DURATION_SECONDS="$duration" INTERRUPT_AFTER_SECONDS=0 \
CONTACT_START_SECONDS="$((duration + 10))" CONTACT_DURATION_SECONDS=7200 \
COMPLETION_TIMEOUT_SECONDS=21600 RUN_ID_OVERRIDE="$run_id" \
EXPERIMENT_ID="store-forward-$(date -u +%Y%m%dT%H%M%SZ)" \
RAW_RESULTS_FILE="$results_dir/store_forward_results.csv" \
LOG_FILE="$logs_dir/$run_id.log" WORK_DIR="$work_dir" \
"$repo_dir/experiments/scripts/run_single.sh" "$image" interruption "$repetition"


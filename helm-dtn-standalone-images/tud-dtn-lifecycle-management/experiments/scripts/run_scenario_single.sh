#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
scenario=${2:?usage: run_scenario_single.sh IMAGE SCENARIO INTERRUPTION_SECONDS REPETITION}
image=${1:?image is required}
duration=${3:?interruption duration in seconds is required}
repetition=${4:?repetition is required}
[[ "$scenario" == leo || "$scenario" == lunar || "$scenario" == mars ]] || { echo "scenario must be leo, lunar, or mars" >&2; exit 2; }
[[ "$duration" =~ ^[1-9][0-9]*$ ]] || { echo "duration must be positive integer seconds" >&2; exit 2; }

case "$scenario" in
  leo)   rtt=50ms; delay=25ms; rate=6mbit; source='Ma et al. Starlink measurement' ;;
  lunar) rtt=2.6s; delay=1.3s; rate=20mbit; source='DSN RTT + NASA LLCD uplink' ;;
  mars)  rtt=1800s; delay=900s; rate=5mbit; source='DSN RTT + JPL optical profile' ;;
esac

label="${duration}s"
run_id=$(printf '%s_%s_interrupt%s_run%02d' "$image" "$scenario" "$label" "$repetition")
NETWORK_SCENARIO="$scenario" NETWORK_RTT="$rtt" NETWORK_ONE_WAY_DELAY="$delay" \
NETWORK_BANDWIDTH="$rate" NETWORK_PACKET_LOSS=0% NETWORK_PROFILE_SOURCE="$source" \
INTERRUPT_DURATION_SECONDS="$duration" RUN_ID_OVERRIDE="$run_id" \
INTERRUPT_AFTER_SECONDS=0 \
COMPLETION_TIMEOUT_SECONDS="$((duration + 10800))" CONTACT_DURATION_SECONDS="$((duration + 10800))" \
RAW_RESULTS_FILE="$repo_dir/experiments/scenarios/results/scenario_results.csv" \
LOG_FILE="$repo_dir/experiments/scenarios/logs/$scenario/$run_id.log" \
WORK_DIR="$repo_dir/experiments/scenarios/work/$run_id" \
"$repo_dir/experiments/scripts/run_single.sh" "$image" interruption "$repetition"

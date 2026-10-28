#!/usr/bin/env bash
set -uo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
experiment_dir="$repo_dir/experiments"
config=${IMAGES_CONFIG:-$experiment_dir/config/images.yaml}
image_name=${1:?usage: run_single.sh IMAGE_NAME CONDITION REPETITION}
condition=${2:?condition must be normal or interruption}
repetition=${3:?repetition number is required}
[[ "$condition" == normal || "$condition" == interruption ]] || { echo "condition must be normal or interruption" >&2; exit 2; }
[[ "$repetition" =~ ^[1-9][0-9]*$ ]] || { echo "repetition must be positive" >&2; exit 2; }

registry=${REGISTRY:-127.0.0.1:5001/library/talos}
registry_creds=${REGISTRY_CREDS:-demo:demo}
earth_aap2=${EARTH_AAP2:-127.0.0.1:14242}
space_aap2=${SPACE_AAP2:-127.0.0.1:24242}
earth_container=${EARTH_CONTAINER:-earth-node}
space_container=${SPACE_CONTAINER:-space-node}
space_tcpcl_host=${SPACE_TCPCL_HOST:-$space_container}
restart_dtn_nodes=${RESTART_DTN_NODES:-true}
destination_eid=${DESTINATION_EID:-dtn://space.dtn/image-transfer}
interrupt_after=${INTERRUPT_AFTER_SECONDS:-2}
interrupt_duration=${INTERRUPT_DURATION_SECONDS:-10}
completion_timeout=${COMPLETION_TIMEOUT_SECONDS:-1800}
arch=${ARCH:-amd64}
network_scenario=${NETWORK_SCENARIO:-}
network_rtt=${NETWORK_RTT:-}
network_delay=${NETWORK_ONE_WAY_DELAY:-}
network_bandwidth=${NETWORK_BANDWIDTH:-}
network_loss=${NETWORK_PACKET_LOSS:-0%}
network_profile_source=${NETWORK_PROFILE_SOURCE:-}
contact_duration=${CONTACT_DURATION_SECONDS:-7200}
contact_start=${CONTACT_START_SECONDS:-1}
contact_capacity=${CONTACT_CAPACITY_BYTES:-1000000000}
initial_cla_address=${INITIAL_CLA_ADDRESS:-tcpclv3:$space_tcpcl_host:4556}

record=$(awk -v wanted="$image_name" '
  /^  - name:/ {name=$0; sub(/^  - name:[[:space:]]*/, "", name); active=(name==wanted)}
  active && /^    category:/ {category=$0; sub(/^    category:[[:space:]]*/, "", category)}
  active && /^    source:/ {source=$0; sub(/^    source:[[:space:]]*/, "", source)}
  active && /^    target:/ {target=$0; sub(/^    target:[[:space:]]*/, "", target); print name "|" category "|" source "|" target; exit}
' "$config")
[[ -n "$record" ]] || { echo "image not found in $config: $image_name" >&2; exit 2; }
IFS='|' read -r image_name category source target <<<"$record"

run_id=${RUN_ID_OVERRIDE:-$(printf '%s_%s_run%02d' "$image_name" "$condition" "$repetition")}
experiment_id=${EXPERIMENT_ID:-$(date -u +%Y%m%dT%H%M%SZ)}
log_file=${LOG_FILE:-$experiment_dir/logs/$run_id.log}
raw_csv=${RAW_RESULTS_FILE:-$experiment_dir/results/evaluation_results.csv}
work_dir=${WORK_DIR:-$experiment_dir/work/$run_id}
mkdir -p "$(dirname "$log_file")" "$(dirname "$raw_csv")" "$work_dir"

for bin in image-transfer-send image-transfer-receive image-transfer-verify; do
  [[ -x "$repo_dir/bin/$bin" ]] || { echo "missing binary: $repo_dir/bin/$bin" >&2; exit 2; }
done
for command in docker python3 /usr/bin/time; do command -v "$command" >/dev/null || { echo "missing command: $command" >&2; exit 2; }; done

sender_log="$work_dir/sender.log"; receiver_log="$work_dir/receiver.log"
verify_log="$work_dir/verify.log"; time_log="$work_dir/sender-time.log"
resources_log="$work_dir/receiver-resources.csv"; state_file="$work_dir/interruption.state"
profile_state_file="$work_dir/network-profile.state"
printf '%s\n' 'timestamp_utc,rss_kib,cpu_percent' >"$resources_log"

receiver_pid=""; sampler_pid=""; interrupter_pid=""
cleanup() {
  [[ -n "$interrupter_pid" ]] && kill "$interrupter_pid" 2>/dev/null || true
  if [[ -s "$state_file" ]]; then
    INTERRUPTION_STATE_FILE="$state_file" "$experiment_dir/scripts/restore_network.sh" >/dev/null 2>&1 || true
  fi
  [[ -n "$sampler_pid" ]] && kill "$sampler_pid" 2>/dev/null || true
  [[ -n "$receiver_pid" ]] && kill "$receiver_pid" 2>/dev/null || true
  if [[ -n "$network_scenario" && -s "$profile_state_file" ]]; then
    NETWORK_PROFILE_STATE_FILE="$profile_state_file" "$experiment_dir/scripts/clear_network_profile.sh" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

# Every measurement gets a clean DTN routing/contact state. This avoids stale
# or overlapping test contacts influencing later repetitions in a long matrix.
docker start go-demo-registry >/dev/null
if [[ "$restart_dtn_nodes" == true ]]; then
  docker restart "$earth_container" "$space_container" >/dev/null
fi
# Let both AAP2 and TCPCL listeners become ready before the short-delay contact
# starts; otherwise the first TCPCL connection attempt can race node startup.
sleep 2
if [[ -n "$network_scenario" ]]; then
  NETWORK_PROFILE_STATE_FILE="$profile_state_file" NETWORK_ONE_WAY_DELAY="$network_delay" \
    NETWORK_BANDWIDTH="$network_bandwidth" NETWORK_PACKET_LOSS="$network_loss" \
    "$experiment_dir/scripts/apply_network_profile.sh" >"$work_dir/network-profile.log" 2>&1
  # Preserve the exact tc/netem inputs for later plotting. The operational
  # state file is removed during cleanup, but this immutable snapshot remains.
  cp "$profile_state_file" "$work_dir/network-profile-applied.state"
fi
docker exec "$earth_container" aap2-config --tcp 127.0.0.1 4242 \
  -s "$contact_start" "$contact_duration" "$contact_capacity" \
  dtn://space.dtn/ "$initial_cla_address" >/dev/null

REGISTRY_CREDS="$registry_creds" "$repo_dir/bin/image-transfer-receive" \
  -tcp "$space_aap2" -registry "$registry" -insecure-registry \
  -work-dir "$work_dir/receiver" >"$receiver_log" 2>&1 &
receiver_pid=$!
sleep 2
(
  while kill -0 "$receiver_pid" 2>/dev/null; do
    read -r rss cpu < <(ps -o rss=,%cpu= -p "$receiver_pid" 2>/dev/null) || break
    printf '%s,%s,%s\n' "$(date -u +%FT%T.%3NZ)" "${rss:-0}" "${cpu:-0}" >>"$resources_log"
    sleep 0.2
  done
) &
sampler_pid=$!

interruption_start=""; reconnection_time=""; completed_after_reconnection="false"; recorded_interrupt_duration=0
if [[ "$condition" == interruption ]]; then
  recorded_interrupt_duration=$interrupt_duration
  (
    deadline=$(( $(date +%s) + 300 ))
    while ! grep -q 'sending .* as one AAP2 ADU' "$sender_log" 2>/dev/null; do
      [[ $(date +%s) -ge "$deadline" ]] && exit 1
      sleep 0.05
    done
    sleep "$interrupt_after"
    if grep -q 'transfer complete:' "$receiver_log" 2>/dev/null; then
      date -u +%FT%T.%3NZ >"$work_dir/interruption-too-late.txt"
      exit 3
    fi
    date -u +%FT%T.%3NZ >"$work_dir/interruption-start.txt"
    INTERRUPTION_STATE_FILE="$state_file" "$experiment_dir/scripts/interrupt_network.sh"
    sleep "$interrupt_duration"
    if grep -q 'transfer complete:' "$receiver_log" 2>/dev/null; then
      date -u +%FT%T.%3NZ >"$work_dir/completed-before-reconnection.txt"
    fi
    INTERRUPTION_STATE_FILE="$state_file" "$experiment_dir/scripts/restore_network.sh"
    date -u +%FT%T.%3NZ >"$work_dir/reconnection.txt"
  ) >"$work_dir/interruption.log" 2>&1 &
  interrupter_pid=$!
fi

start_time=$(date -u +%FT%T.%3NZ); start_ns=$(date +%s%N)
/usr/bin/time -v -o "$time_log" "$repo_dir/bin/image-transfer-send" \
  -tcp "$earth_aap2" -destination-eid "$destination_eid" -image "$source" \
  -registry-target "$target" -arch "$arch" -work-dir "$work_dir/sender" \
  >"$sender_log" 2>&1
sender_status=$?

transfer_id=$(sed -n 's/^transfer_id=//p' "$sender_log" | tail -n1)
payload_bytes=$(sed -n 's/.*one AAP2 ADU (\([0-9][0-9]*\) bytes).*/\1/p' "$sender_log" | tail -n1)
source_digest=$(sed -n 's/^source_digest=//p' "$sender_log" | tail -n1)
error_message=""; destination_digest=""; digest_match=false; success=false

if [[ "$sender_status" -ne 0 ]]; then
  error_message="sender_exit_$sender_status: $(tail -n1 "$sender_log")"
else
  deadline=$(( $(date +%s) + completion_timeout ))
  while ! grep -q "transfer complete: id=$transfer_id " "$receiver_log" 2>/dev/null && [[ $(date +%s) -lt "$deadline" ]]; do
    if [[ "$network_scenario" == mars ]]; then
      tcpcl_failures=$(docker logs --since "$start_time" "$earth_container" 2>&1 | grep -c 'TCP: Failed to connect' || true)
      if [[ "$tcpcl_failures" -ge 1 ]]; then
        error_message="tcpcl_connection_timeout_under_mars_rtt"
        break
      fi
    fi
    sleep 1
  done
  completion=$(grep "transfer complete: id=$transfer_id " "$receiver_log" | tail -n1)
  if [[ -z "$completion" ]]; then
    [[ -n "$error_message" ]] || error_message="receiver_completion_timeout"
  else
    destination_digest=$(sed -n 's/.* digest=\(sha256:[0-9a-f]*\).*/\1/p' <<<"$completion")
    one_image_file="$work_dir/verify-image.txt"
    printf '%s=%s\n' "$source" "$target" >"$one_image_file"
    REGISTRY_CREDS="$registry_creds" "$repo_dir/bin/image-transfer-verify" \
      -images-file "$one_image_file" -registry "$registry" -insecure-registry -arch "$arch" \
      >"$verify_log" 2>&1
    verify_status=$?
    [[ "$verify_status" -eq 0 && "$source_digest" == "$destination_digest" ]] && digest_match=true
    if [[ "$verify_status" -eq 0 && "$digest_match" == true ]]; then success=true; else error_message="digest_verification_failed"; fi
  fi
fi

[[ -n "$interrupter_pid" ]] && wait "$interrupter_pid" 2>/dev/null || true
[[ -f "$work_dir/interruption-start.txt" ]] && interruption_start=$(cat "$work_dir/interruption-start.txt")
[[ -f "$work_dir/reconnection.txt" ]] && reconnection_time=$(cat "$work_dir/reconnection.txt")
interruption_effective=true
[[ -f "$work_dir/interruption-too-late.txt" ]] && interruption_effective=false
[[ "$condition" == interruption && -n "$reconnection_time" && "$success" == true && ! -f "$work_dir/completed-before-reconnection.txt" && "$interruption_effective" == true ]] && completed_after_reconnection=true
finish_time=$(date -u +%FT%T.%3NZ); finish_ns=$(date +%s%N)
duration=$(python3 -c "print(($finish_ns-$start_ns)/1e9)")
payload_mb=$(python3 -c "print(int('${payload_bytes:-0}')/1000000)")
throughput_mb_s=$(python3 -c "p=int('${payload_bytes:-0}'); d=float('$duration'); print(p/1000000/d if d else 0)")
throughput_mbit_s=$(python3 -c "p=int('${payload_bytes:-0}'); d=float('$duration'); print(p*8/1000000/d if d else 0)")
sender_rss_kib=$(sed -n 's/^[[:space:]]*Maximum resident set size (kbytes):[[:space:]]*//p' "$time_log" | tail -n1)
sender_memory_mb=$(python3 -c "print(int('${sender_rss_kib:-0}')/1024)")
sender_cpu_percent=$(sed -n 's/^[[:space:]]*Percent of CPU this job got:[[:space:]]*\([0-9.]*\)%.*/\1/p' "$time_log" | tail -n1)
receiver_rss_kib=$(awk -F, 'NR>1 && $2>m {m=$2} END {print m+0}' "$resources_log")
receiver_memory_mb=$(python3 -c "print(int('${receiver_rss_kib:-0}')/1024)")
receiver_cpu_percent=$(awk -F, 'NR>1 && $3>m {m=$3} END {print m+0}' "$resources_log")
latency_ms=""
if [[ -z "$network_scenario" ]]; then
  space_dtn_ip=$(docker inspect -f '{{(index .NetworkSettings.Networks "dtn-net").IPAddress}}' "$space_container")
  latency_ms=$(ping -c 3 -q "$space_dtn_ip" 2>/dev/null | sed -n 's#.* = [^/]*/\([^/]*\)/.*#\1#p')
fi

python3 - "$raw_csv" "$experiment_id" "$run_id" "$repetition" "$image_name" "$category" "$source" "$registry/$target" "$condition" "${payload_bytes:-0}" "$payload_mb" "$start_time" "$finish_time" "$duration" "$sender_memory_mb" "$receiver_memory_mb" "${sender_cpu_percent:-0}" "${receiver_cpu_percent:-0}" "${latency_ms:-}" "$source_digest" "$destination_digest" "$digest_match" "$success" "$error_message" "$interruption_start" "$recorded_interrupt_duration" "$reconnection_time" "$completed_after_reconnection" "$throughput_mb_s" "$throughput_mbit_s" "$network_scenario" "$network_rtt" "$network_delay" "$network_bandwidth" "$network_loss" "$network_profile_source" "$interruption_effective" <<'PY'
import csv, os, sys
path, values = sys.argv[1], sys.argv[2:]
header = ["experiment_id","run_id","repetition","image_name","image_category","source_image","destination_image","network_condition","payload_size_bytes","payload_size_mb","start_time","finish_time","duration_seconds","sender_peak_memory_mb","receiver_peak_memory_mb","sender_cpu_percent","receiver_peak_cpu_percent","network_latency_ms","source_digest","destination_digest","digest_match","success","error_message","interruption_start_time","interruption_duration_seconds","reconnection_time","completed_after_reconnection","throughput_mb_s","throughput_mbit_s"]
if values[29]:
    header += ["network_scenario","configured_rtt","configured_one_way_delay","configured_bandwidth","configured_packet_loss","network_profile_source","interruption_effective"]
else:
    values = values[:29]
new = not os.path.exists(path) or os.path.getsize(path) == 0
with open(path, "a", newline="", encoding="utf-8") as f:
    writer = csv.writer(f)
    if new: writer.writerow(header)
    writer.writerow(values)
PY

{
  echo "experiment_id=$experiment_id"; echo "run_id=$run_id"; echo "image_name=$image_name"
  echo "image_category=$category"; echo "source_image=$source"; echo "destination_image=$registry/$target"
  echo "network_condition=$condition"; echo "start_time=$start_time"; echo "finish_time=$finish_time"
  echo "network_scenario=$network_scenario"; echo "configured_rtt=$network_rtt"
  echo "configured_one_way_delay=$network_delay"; echo "configured_bandwidth=$network_bandwidth"
  echo "configured_packet_loss=$network_loss"; echo "network_profile_source=$network_profile_source"
  echo "payload_size_bytes=${payload_bytes:-0}"; echo "payload_size_mb=$payload_mb"; echo "duration_seconds=$duration"
  echo "sender_peak_memory_mb=$sender_memory_mb"; echo "receiver_peak_memory_mb=$receiver_memory_mb"
  echo "sender_cpu_percent=${sender_cpu_percent:-0}"; echo "receiver_peak_cpu_percent=${receiver_cpu_percent:-0}"
  echo "network_latency_ms=${latency_ms:-}"
  echo "source_digest=$source_digest"; echo "destination_digest=$destination_digest"
  echo "digest_match=$digest_match"; echo "success=$success"; echo "error_message=$error_message"
  echo "interruption_start_time=$interruption_start"; echo "interruption_duration_seconds=$recorded_interrupt_duration"
  echo "reconnection_time=$reconnection_time"; echo "interruption_effective=$interruption_effective"
  echo "completed_after_reconnection=$completed_after_reconnection"
  echo "throughput_mb_s=$throughput_mb_s"; echo "throughput_mbit_s=$throughput_mbit_s"
  echo; echo "===== SENDER ====="; cat "$sender_log"
  echo; echo "===== RECEIVER ====="; cat "$receiver_log"
  echo; echo "===== VERIFIER ====="; cat "$verify_log" 2>/dev/null || true
  echo; echo "===== SENDER RESOURCE USAGE ====="; cat "$time_log"
  echo; echo "===== RECEIVER RESOURCE SAMPLES ====="; cat "$resources_log"
  if [[ -f "$work_dir/network-profile.log" ]]; then
    echo; echo "===== APPLIED NETWORK PROFILE ====="; cat "$work_dir/network-profile.log"
  fi
} >"$log_file"

echo "run_id=$run_id success=$success log=$log_file"
[[ "$success" == true ]]

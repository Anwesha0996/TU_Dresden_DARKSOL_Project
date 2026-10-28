#!/usr/bin/env bash
set -uo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
base="$repo_dir/experiments/store-forward"
results="$base/results/persistent_store_forward_results.csv"
progress="$base/results/full_matrix_progress.log"
mkdir -p "$base/results"

images=(flannel coredns etcd kube-apiserver kube-controller-manager kube-scheduler kube-proxy kubelet pause talos-installer)
scenarios=(leo lunar mars)
durations=(10 300 3600)

is_successful() {
  local wanted=$1
  local wanted_duration=$2
  [[ -f "$results" ]] || return 1
  awk -F, -v id="$wanted" -v duration="$wanted_duration" '
    NR>1 && $2==id && $22=="true" && $25==duration && $27=="true" && $36 ~ /^true/ {found=1}
    END {exit !found}
  ' "$results"
}

run_case() {
  local image=$1 scenario=$2 duration=$3 repetition=$4
  local run_id
  run_id=$(printf '%s_%s_persistent_sf_%ss_run%02d' "$image" "$scenario" "$duration" "$repetition")
  if is_successful "$run_id" "$duration"; then
    printf '%s SKIP success %s\n' "$(date -u +%FT%TZ)" "$run_id" >>"$progress"
    return 0
  fi
  printf '%s START %s\n' "$(date -u +%FT%TZ)" "$run_id" >>"$progress"
  if "$repo_dir/experiments/scripts/setup_persistent_dtn.sh" && \
     "$repo_dir/experiments/scripts/run_persistent_store_forward.sh" "$image" "$scenario" "$duration" "$repetition"; then
    printf '%s PASS %s\n' "$(date -u +%FT%TZ)" "$run_id" >>"$progress"
  else
    status=$?
    printf '%s FAIL exit=%s %s\n' "$(date -u +%FT%TZ)" "$status" "$run_id" >>"$progress"
  fi
}

printf '%s MATRIX_START pid=%s\n' "$(date -u +%FT%TZ)" "$$" >>"$progress"

# Required gates before automating the full matrix.
for duration in 300 3600; do
  for scenario in "${scenarios[@]}"; do
    run_case flannel "$scenario" "$duration" 1
  done
done

# Full resumable matrix. Verified gate results are skipped automatically.
for image in "${images[@]}"; do
  for scenario in "${scenarios[@]}"; do
    for duration in "${durations[@]}"; do
      for repetition in $(seq 1 10); do
        run_case "$image" "$scenario" "$duration" "$repetition"
      done
    done
  done
done

printf '%s MATRIX_COMPLETE\n' "$(date -u +%FT%TZ)" >>"$progress"

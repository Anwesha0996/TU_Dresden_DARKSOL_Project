#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
condition=${1:-normal}
repetitions=${2:-10}
raw_csv=${RAW_RESULTS_FILE:-$repo_dir/experiments/results/evaluation_results.csv}
mapfile -t images < <(sed -n 's/^  - name:[[:space:]]*//p' "$repo_dir/experiments/config/images.yaml")
for image in "${images[@]}"; do
  for repetition in $(seq 1 "$repetitions"); do
    run_id=$(printf '%s_%s_run%02d' "$image" "$condition" "$repetition")
    if [[ -f "$raw_csv" ]] && awk -F, -v wanted="$run_id" 'NR>1 && $2==wanted {found=1} END {exit !found}' "$raw_csv"; then
      echo "skip existing run_id=$run_id"
      continue
    fi
    "$repo_dir/experiments/scripts/run_single.sh" "$image" "$condition" "$repetition"
  done
done

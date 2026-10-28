#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
image=${1:?usage: run_scenario_matrix.sh IMAGE [REPETITIONS]}
repetitions=${2:-10}
results="$repo_dir/experiments/scenarios/results/scenario_results.csv"
for scenario in leo lunar mars; do
  for duration in 10 300 3600; do
    for repetition in $(seq 1 "$repetitions"); do
      run_id=$(printf '%s_%s_interrupt%ss_run%02d' "$image" "$scenario" "$duration" "$repetition")
      if [[ -f "$results" ]] && awk -F, -v wanted="$run_id" 'NR>1 && $2==wanted {found=1} END {exit !found}' "$results"; then
        echo "skip existing run_id=$run_id"
        continue
      fi
      if ! "$repo_dir/experiments/scripts/run_scenario_single.sh" "$image" "$scenario" "$duration" "$repetition"; then
        echo "recorded failed run_id=$run_id; continuing matrix" >&2
      fi
    done
  done
done

python3 "$repo_dir/experiments/scripts/analyse_scenarios.py"
python3 "$repo_dir/experiments/scripts/plot_scenarios.py"
python3 "$repo_dir/experiments/scripts/plot_paper_figures.py"
python3 "$repo_dir/experiments/scripts/export_scenario_cases_xlsx.py"
python3 "$repo_dir/experiments/scripts/export_progress_brief_xlsx.py"
echo "scenario matrix complete: results and graphs regenerated"

# Standalone image-transfer evaluation

This directory evaluates the existing standalone sender and receiver. It does
not contain or replace image-transfer implementation code.

The image list is pinned to the 10 cluster images selected by
`space-exec-env/talos/config/scripts/02-mirror-container-images.sh`: the nine
`talosctl image default` images for Talos v1.12.4 plus its installer image.
Results use decimal MB (`bytes / 1,000,000`). Duration begins
before the sender pulls the source image and ends after the receiver pushes it
to Harbor and the independent verifier confirms the digest.

Validation stages:

```bash
# Gate 1
bash experiments/scripts/run_single.sh pause normal 1

# Gate 2
for n in 1 2 3; do bash experiments/scripts/run_single.sh pause normal "$n"; done

# Gate 3
bash experiments/scripts/run_all.sh normal 1

# Gate 4 (only after reviewing Gate 3)
bash experiments/scripts/run_all.sh normal 10

# Interruption gate
INTERRUPT_AFTER_SECONDS=2 INTERRUPT_DURATION_SECONDS=10 \
  bash experiments/scripts/run_single.sh pause interruption 1
```

Each run creates `experiments/logs/<image>_<condition>_runNN.log` and appends one
row to `experiments/results/evaluation_results.csv`. Generate grouped statistics with:

```bash
python3 experiments/scripts/analyse_results.py
python3 experiments/scripts/plot_results.py
```

The plotting command creates separate normal-network and interruption-network
dashboards, per-run high/low trend graphs, and a direct comparison under
`experiments/results/graphs/`.

The current CPU/interruption-aware result files are `evaluation_results.csv`,
`cluster10_evaluation_results.csv`, and `evaluation_summary.csv`. Earlier
baseline CSVs are preserved under `results/archive/`. Temporary OCI layouts,
raw per-process files, exploratory logs, and accidentally nested copied files
are preserved under `archive/`; they are not part of the final 200-run result.

The local test defaults use `127.0.0.1:5001/library/talos`. For Space Harbor:

```bash
REGISTRY=registry.space.lan:80/talos REGISTRY_CREDS='user:password' \
  bash experiments/scripts/run_single.sh pause normal 1
```

Interruption blocks only outbound Earth-to-Space TCPCLv3 traffic on port 4556.
It does not stop or pause the sender, receiver, or either µD3TN container.

Build the network-namespace helper once before interruption tests:

```bash
docker build -t dtn-experiment-nettools:latest experiments/network-tools
```

The sourced LEO/Lunar/Mars profiles and the 10-second, 5-minute, and 1-hour
interruption matrix are documented in `scenarios/README.md`. Start only with:

```bash
bash experiments/scripts/run_scenario_single.sh pause leo 10 1
```

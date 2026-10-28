#!/usr/bin/env python3
"""Compare measured OCI transfer performance across LEO/Lunar/Mars profiles."""

import re
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd


ROOT = Path(__file__).resolve().parents[1]
CSV = ROOT / "store-forward" / "results" / "persistent_store_forward_results.csv"
OUT = ROOT / "store-forward" / "results" / "graphs"
OUT.mkdir(parents=True, exist_ok=True)

SCENARIOS = ["leo", "lunar", "mars"]
COLORS = {"leo": "#0072B2", "lunar": "#E69F00", "mars": "#CC79A7"}
MARKERS = {"leo": "o", "lunar": "s", "mars": "D"}
DURATIONS = [10, 300, 3600]
LABELS = ["10 s", "5 min", "1 h"]


def truth(series):
    return series.astype(str).str.lower().eq("true")


def expected(run_id):
    match = re.search(r"_persistent_sf_(\d+)s_", str(run_id))
    return int(match.group(1)) if match else np.nan


df = pd.read_csv(CSV)
df["scenario"] = df["network_scenario"].astype(str).str.lower()
df["outage"] = pd.to_numeric(df["interruption_duration_seconds"], errors="coerce")
df["expected"] = df["run_id"].map(expected)
df = df[df.scenario.isin(SCENARIOS) & df.outage.isin(DURATIONS) & (df.expected == df.outage)].copy()
df["duration"] = pd.to_numeric(df["duration_seconds"], errors="coerce")
df["throughput"] = pd.to_numeric(df["throughput_mbit_s"], errors="coerce")
df["valid_success"] = truth(df["success"]) & truth(df["digest_match"])

fig, axes = plt.subplots(1, 3, figsize=(17, 5.6))
x = np.arange(len(DURATIONS))

for scenario in SCENARIOS:
    means_duration, std_duration = [], []
    means_throughput, success_rate, counts = [], [], []
    for duration in DURATIONS:
        group = df[(df.scenario == scenario) & (df.outage == duration)]
        successful = group[group.valid_success]
        means_duration.append(successful.duration.mean())
        std_duration.append(successful.duration.std(ddof=1) if len(successful) > 1 else 0)
        means_throughput.append(successful.throughput.mean())
        success_rate.append(100 * group.valid_success.mean() if len(group) else np.nan)
        counts.append(len(group))

    label = scenario.upper() if scenario != "mars" else "MARS (contact-only)"
    axes[0].errorbar(x, means_duration, yerr=std_duration, color=COLORS[scenario],
                     marker=MARKERS[scenario], linewidth=2.2, markersize=7,
                     capsize=4, label=label)
    axes[1].plot(x, means_throughput, color=COLORS[scenario], marker=MARKERS[scenario],
                 linewidth=2.2, markersize=7, label=label)
    axes[2].plot(x, success_rate, color=COLORS[scenario], marker=MARKERS[scenario],
                 linewidth=2.2, markersize=7, label=label)
    for index, (rate, count) in enumerate(zip(success_rate, counts)):
        if np.isfinite(rate):
            axes[2].annotate(f"n={count}", (index, rate), xytext=(0, 8),
                             textcoords="offset points", ha="center", fontsize=8,
                             color=COLORS[scenario])

axes[0].set_title("Transfer duration rises with outage", fontweight="bold")
axes[0].set_ylabel("Mean successful total duration (seconds)")
axes[0].set_yscale("log")
axes[0].text(.02, .03, "Points: mean | Error bars: ±1 standard deviation",
             transform=axes[0].transAxes, fontsize=8, color="#555")

axes[1].set_title("Effective throughput falls with waiting time", fontweight="bold")
axes[1].set_ylabel("Mean effective throughput (Mbit/s)")
axes[1].set_yscale("log")
axes[1].text(.02, .03, "Payload bits ÷ complete wall-clock duration",
             transform=axes[1].transAxes, fontsize=8, color="#555")

axes[2].set_title("Recorded attempt success rate", fontweight="bold")
axes[2].set_ylabel("Successful digest-verified attempts (%)")
axes[2].set_ylim(0, 112)

for ax in axes:
    ax.set_xticks(x, LABELS)
    ax.set_xlabel("Configured contact interruption")
    ax.grid(True, which="both", alpha=.25)

axes[0].legend(loc="upper left", fontsize=8)
fig.suptitle("Measured OCI image-transfer behavior across LEO, Lunar and Mars profiles",
             fontsize=17, fontweight="bold", color="#153B5B")
fig.text(.5, .012,
         "Live snapshot; only recorded rows are included. Mars used 5 Mbit/s with 0 ms applied propagation RTT (1800 s reference only).",
         ha="center", fontsize=9, color="#475569")
fig.tight_layout(rect=(0, .045, 1, .92))

for suffix in ("png", "svg"):
    fig.savefig(OUT / f"scenario_performance_comparison.{suffix}", dpi=220, bbox_inches="tight")
plt.close(fig)
print(f"Created {OUT / 'scenario_performance_comparison.png'} from {len(df)} recorded attempts")

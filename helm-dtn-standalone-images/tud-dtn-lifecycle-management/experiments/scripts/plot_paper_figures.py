#!/usr/bin/env python3
"""Generate publication-style experiment figures from real scenario results."""

import hashlib
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

ROOT = Path(__file__).resolve().parents[1] / "scenarios"
CSV = ROOT / "results" / "scenario_results.csv"
OUT = ROOT / "results" / "graphs" / "paper_style"
OUT.mkdir(parents=True, exist_ok=True)

df = pd.read_csv(CSV)
df["success_bool"] = df["success"].astype(str).str.lower().eq("true")
df["scenario"] = df["network_scenario"].str.upper()
order = ["LEO", "LUNAR", "MARS"]
durations = [10, 300, 3600]
duration_labels = {10: "10 s", 300: "5 min", 3600: "1 h"}
colors = {"LEO": "#0072B2", "LUNAR": "#E69F00", "MARS": "#CC79A7"}

plt.rcParams.update({
    "font.family": "DejaVu Sans", "font.size": 11, "axes.titlesize": 12,
    "axes.labelsize": 11, "legend.fontsize": 10, "figure.dpi": 150,
    "axes.grid": True, "grid.alpha": 0.25, "axes.spines.top": False,
    "axes.spines.right": False,
})


def save(fig, name):
    fig.savefig(OUT / f"{name}.png", bbox_inches="tight", dpi=220)
    fig.savefig(OUT / f"{name}.svg", bbox_inches="tight")
    plt.close(fig)


def stable_jitter(keys, width=0.12):
    values = []
    for key in keys:
        digest = hashlib.sha256(str(key).encode()).digest()
        values.append((int.from_bytes(digest[:2], "big") / 65535 - 0.5) * 2 * width)
    return np.asarray(values)


# Figure 1: inputs, explicitly distinguishing RTT from one-way delay.
profiles = pd.DataFrame({
    "Scenario": order, "RTT_ms": [50, 2600, 1_800_000],
    "One_way_ms": [25, 1300, 900_000], "Bandwidth": [6, 20, 5]
})
fig, axes = plt.subplots(1, 2, figsize=(11, 4.4))
x = np.arange(3)
axes[0].plot(x, profiles.RTT_ms, marker="o", linewidth=2.2, label="Round-trip time (RTT)")
axes[0].plot(x, profiles.One_way_ms, marker="s", linewidth=2.2, linestyle="--", label="One-way delay")
axes[0].set_yscale("log"); axes[0].set_xticks(x, order)
axes[0].set_ylabel("Configured propagation delay (ms, log scale)")
axes[0].set_title("Delay profile used between µD3TN nodes"); axes[0].legend()
axes[1].plot(x, profiles.Bandwidth, marker="o", linewidth=2.2, color="#009E73")
axes[1].set_xticks(x, order); axes[1].set_ylabel("Configured bandwidth (Mbit/s)")
axes[1].set_ylim(0, 22); axes[1].set_yticks([0, 5, 10, 15, 20])
axes[1].set_title("Bandwidth profile used between µD3TN nodes")
fig.suptitle("Figure 1 — Fixed link parameters (not unrestricted local-network values)", fontweight="bold")
fig.text(.5, -.01, "Lunar/Mars RTT guided by the supplied IMC DSN paper; other inputs have separate sources.", ha="center", fontsize=9)
fig.tight_layout(); save(fig, "fig01_network_profiles")


# Figure 2: success rate heatmap; missing is visibly N/A, never zero.
rate = np.full((3, 3), np.nan); counts = np.zeros((3, 3), dtype=int)
for i, scenario in enumerate(order):
    for j, duration in enumerate(durations):
        g = df[(df.scenario == scenario) & (df.interruption_duration_seconds == duration)]
        counts[i, j] = len(g)
        if len(g): rate[i, j] = 100 * g.success_bool.mean()
fig, ax = plt.subplots(figsize=(8.5, 4.8))
masked = np.ma.masked_invalid(rate)
im = ax.imshow(masked, vmin=0, vmax=100, cmap="RdYlGn", aspect="auto")
ax.set_xticks(range(3), [duration_labels[d] for d in durations]); ax.set_yticks(range(3), order)
ax.set_xlabel("Network interruption duration"); ax.set_ylabel("Space communication scenario")
ax.set_title("Figure 2 — Delivery success rate for each common interruption", fontweight="bold")
for i in range(3):
    for j in range(3):
        text = "N/A\n0 runs" if np.isnan(rate[i, j]) else f"{rate[i,j]:.0f}%\n{counts[i,j]}/10 runs"
        ax.text(j, i, text, ha="center", va="center", fontweight="bold", color="black")
cb = fig.colorbar(im, ax=ax, label="Successful runs (%)"); cb.set_ticks([0, 20, 40, 60, 80, 100])
fig.tight_layout(); save(fig, "fig02_success_rate_matrix")


def distribution_figure(subset, field, ylabel, title, name, log=False):
    fig, axes = plt.subplots(1, 3, figsize=(14, 4.8), sharey=True)
    for ax, duration in zip(axes, durations):
        part = subset[subset.interruption_duration_seconds == duration]
        for index, scenario in enumerate(order):
            values = pd.to_numeric(part.loc[part.scenario == scenario, field], errors="coerce").dropna().to_numpy()
            if not len(values):
                ax.text(index, 1 if log else 0, "N/A", ha="center", va="bottom", color="#666")
                continue
            keys = part.loc[part.scenario == scenario, "run_id"].iloc[:len(values)]
            jitter = stable_jitter(keys)
            ax.scatter(index + jitter, values, s=32, alpha=.72, color=colors[scenario], edgecolor="white", linewidth=.4, label="Individual run" if index == 0 else None)
            mean = values.mean(); sd = values.std(ddof=1) if len(values) > 1 else 0
            ax.errorbar(index, mean, yerr=sd, fmt="D", color="black", capsize=5, markersize=6, label="Mean ± SD" if index == 0 else None)
        ax.set_xticks(range(3), order); ax.set_xlabel("Scenario")
        ax.set_title(f"Interruption: {duration_labels[duration]}")
        if log: ax.set_yscale("log")
    axes[0].set_ylabel(ylabel)
    handles, labels = axes[0].get_legend_handles_labels()
    if handles: fig.legend(handles, labels, loc="upper center", bbox_to_anchor=(.5, .94), ncol=2)
    fig.suptitle(title, fontweight="bold", y=1.02)
    fig.text(.5, -.01, "Points are repetitions; diamond is mean; error bar is ±1 standard deviation. N/A means no applicable completed run.", ha="center", fontsize=9)
    fig.tight_layout(rect=(0, .04, 1, .91)); save(fig, name)


distribution_figure(df[df.success_bool], "duration_seconds", "Successful end-to-end duration (s, log scale)",
                    "Figure 3 — Successful transfer duration: scenarios compared at the same interruption", "fig03_success_duration_distribution", True)
distribution_figure(df[~df.success_bool], "duration_seconds", "Time until recorded failure (s, log scale)",
                    "Figure 4 — Failure timing (not successful transfer duration)", "fig04_failure_time_distribution", True)
distribution_figure(df[df.success_bool], "throughput_mbit_s", "Effective payload throughput (Mbit/s)",
                    "Figure 5 — Successful effective throughput", "fig05_success_throughput_distribution", False)


# Figure 6: sender and receiver CPU are distinct series and sampling methods.
fig, axes = plt.subplots(1, 3, figsize=(14, 4.8), sharey=True)
for ax, duration in zip(axes, durations):
    part = df[df.interruption_duration_seconds == duration]
    for scenario in order:
        g = part[part.scenario == scenario]
        if g.empty: continue
        sx = pd.to_numeric(g.sender_cpu_percent, errors="coerce").mean()
        rx = pd.to_numeric(g.receiver_peak_cpu_percent, errors="coerce").mean()
        pos = order.index(scenario)
        ax.plot([pos-.12, pos+.12], [sx, rx], color=colors[scenario], alpha=.45)
        ax.scatter(pos-.12, sx, marker="o", s=65, color=colors[scenario], edgecolor="black", label="Sender run-level CPU" if pos == 0 else None)
        ax.scatter(pos+.12, rx, marker="^", s=65, facecolor="white", edgecolor=colors[scenario], linewidth=2, label="Receiver sampled peak CPU" if pos == 0 else None)
    ax.set_xticks(range(3), order); ax.set_xlabel("Scenario"); ax.set_title(f"Interruption: {duration_labels[duration]}")
axes[0].set_ylabel("CPU utilization (%)")
handles, labels = axes[0].get_legend_handles_labels(); fig.legend(handles, labels, loc="upper center", bbox_to_anchor=(.5,.94), ncol=2)
fig.suptitle("Figure 6 — Sender and receiver CPU (separate measurement methods)", fontweight="bold", y=1.02)
fig.tight_layout(rect=(0,0,1,.9)); save(fig, "fig06_sender_receiver_cpu")


# Figure 7: memory comparison, also explicitly split by side.
fig, axes = plt.subplots(1, 3, figsize=(14, 4.8), sharey=True)
for ax, duration in zip(axes, durations):
    part = df[df.interruption_duration_seconds == duration]
    for scenario in order:
        g = part[part.scenario == scenario]
        if g.empty: continue
        sender = pd.to_numeric(g.sender_peak_memory_mb, errors="coerce").mean()
        receiver = pd.to_numeric(g.receiver_peak_memory_mb, errors="coerce").mean()
        pos = order.index(scenario)
        ax.plot([pos-.12, pos+.12], [sender, receiver], color=colors[scenario], alpha=.45)
        ax.scatter(pos-.12, sender, marker="o", s=65, color=colors[scenario], edgecolor="black", label="Sender peak" if pos == 0 else None)
        ax.scatter(pos+.12, receiver, marker="s", s=65, facecolor="white", edgecolor=colors[scenario], linewidth=2, label="Receiver peak" if pos == 0 else None)
    ax.set_xticks(range(3), order); ax.set_xlabel("Scenario"); ax.set_title(f"Interruption: {duration_labels[duration]}")
axes[0].set_ylabel("Peak resident memory (MB)")
handles, labels = axes[0].get_legend_handles_labels(); fig.legend(handles, labels, loc="upper center", bbox_to_anchor=(.5,.94), ncol=2)
fig.suptitle("Figure 7 — Sender and receiver peak memory", fontweight="bold", y=1.02)
fig.tight_layout(rect=(0,0,1,.9)); save(fig, "fig07_sender_receiver_memory")

print(OUT)

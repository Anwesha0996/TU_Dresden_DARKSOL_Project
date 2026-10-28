#!/usr/bin/env python3
"""Plot truthful timelines from validated persistent store-and-forward runs."""

import re
from datetime import datetime
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

ROOT = Path(__file__).resolve().parents[1]
CSV = ROOT / "store-forward" / "results" / "persistent_store_forward_results.csv"
WORK = ROOT / "store-forward" / "work"
OUT = ROOT / "store-forward" / "results" / "graphs" / "corrected_timelines"
OUT.mkdir(parents=True, exist_ok=True)

COLORS = {"leo": "#0072B2", "lunar": "#E69F00", "mars": "#CC79A7"}
DURATION_LABELS = {10: "10 s", 300: "5 min", 3600: "1 h"}
REFERENCE_RTT_MS = {"leo": 50.0, "lunar": 2600.0, "mars": 1_800_000.0}


def utc(value):
    return datetime.fromisoformat(str(value).replace("Z", "+00:00"))


def truth(value):
    return str(value).lower() == "true"


def expected_duration(run_id):
    match = re.search(r"_persistent_sf_(\d+)s_", str(run_id))
    return int(match.group(1)) if match else None


def unit_ms(value):
    """Convert a saved tc/netem delay value to milliseconds."""
    value = str(value).strip().lower()
    match = re.fullmatch(r"([0-9.]+)\s*(us|ms|s)", value)
    if not match:
        raise ValueError(f"unsupported recorded delay value: {value!r}")
    number, unit = float(match.group(1)), match.group(2)
    return number * {"us": .001, "ms": 1.0, "s": 1000.0}[unit]


def unit_mbit(value):
    """Convert a saved tc rate value to Mbit/s."""
    value = str(value).strip().lower()
    match = re.fullmatch(r"([0-9.]+)\s*(kbit|mbit|gbit)", value)
    if not match:
        raise ValueError(f"unsupported recorded bandwidth value: {value!r}")
    number, unit = float(match.group(1)), match.group(2)
    return number * {"kbit": .001, "mbit": 1.0, "gbit": 1000.0}[unit]


def applied_network_state(row):
    """Read values saved by apply_network_profile.sh for this exact run."""
    state_file = WORK / row.run_id / "network-profile-applied.state"
    state = {}
    if state_file.exists():
        for line in state_file.read_text(encoding="utf-8").splitlines():
            if "=" in line:
                key, value = line.split("=", 1)
                state[key.strip()] = value.strip()
    # The result-row values are a fallback for old runs whose work directory
    # was archived. Never substitute the reference RTT here.
    one_way = state.get("delay", str(row.configured_one_way_delay))
    rate = state.get("rate", str(row.configured_bandwidth))
    return {
        "one_way_delay": one_way,
        "applied_rtt_ms": 2 * unit_ms(one_way),
        "bandwidth_mbit": unit_mbit(rate),
        "source": "network-profile.state" if state else "result metadata (state file unavailable)",
    }


df = pd.read_csv(CSV)
df["expected_duration"] = df.run_id.map(expected_duration)
df["interruption_duration_seconds"] = pd.to_numeric(df.interruption_duration_seconds, errors="coerce")
valid = df[
    df.success.map(truth)
    & df.digest_match.map(truth)
    & df.completed_after_reconnection.map(truth)
    & df.interruption_effective.map(truth)
    & (df.expected_duration == df.interruption_duration_seconds)
].copy()


def timeline(row):
    scenario = row.network_scenario
    applied_state = applied_network_state(row)
    reference_rtt_ms = REFERENCE_RTT_MS[scenario]
    applied_rtt_ms = applied_state["applied_rtt_ms"]
    contact_only = reference_rtt_ms > 0 and applied_rtt_ms == 0
    run_id = row.run_id
    resource_file = WORK / run_id / "receiver-resources.csv"
    samples = pd.read_csv(resource_file) if resource_file.exists() else pd.DataFrame()

    start, finish = utc(row.start_time), utc(row.finish_time)
    outage_start, reconnect = utc(row.interruption_start_time), utc(row.reconnection_time)
    total = max((finish - start).total_seconds(), 1.0)
    outage_a = max((outage_start - start).total_seconds(), 0.0)
    outage_b = min((reconnect - start).total_seconds(), total)

    panel_count = 3 if contact_only else 4
    ratios = [1, 1.25, 1.25] if contact_only else [1, 1, 1.25, 1.25]
    fig, axes = plt.subplots(panel_count, 1, figsize=(12, 8 if contact_only else 9),
                             sharex=True, gridspec_kw={"height_ratios": ratios})
    color = COLORS[scenario]
    for ax in axes:
        ax.axvspan(outage_a, outage_b, color="#D55E00", alpha=.16, label="Link unavailable")
        ax.axvline(outage_a, color="#D55E00", linestyle="--", linewidth=1)
        ax.axvline(outage_b, color="#009E73", linestyle="--", linewidth=1)
        ax.grid(alpha=.25)

    # Link capacity is zero while the contact is unavailable.
    bw = applied_state["bandwidth_mbit"]
    axes[0].step([0, outage_a, outage_a, outage_b, outage_b, total],
                 [bw, bw, 0, 0, bw, bw], where="post", color=color, linewidth=2.2)
    axes[0].set_ylabel("Available link\n(Mbit/s)")
    axes[0].set_ylim(-.05 * bw, 1.2 * bw)

    # RTT is undefined during an outage. Leave a visible gap rather than
    # incorrectly drawing a zero-latency link.
    resource_cpu_axis = 1 if contact_only else 2
    resource_mem_axis = 2 if contact_only else 3
    if not contact_only:
        axes[1].plot([0, outage_a], [applied_rtt_ms, applied_rtt_ms], color="#7A5195", linewidth=2.2)
        axes[1].plot([outage_b, total], [applied_rtt_ms, applied_rtt_ms], color="#7A5195", linewidth=2.2)
        axes[1].set_ylim(0, applied_rtt_ms * 1.25)
        axes[1].text((outage_a + outage_b) / 2, applied_rtt_ms * .55, "RTT unavailable",
                     ha="center", color="#A33", fontweight="bold")
        axes[1].set_ylabel("Applied RTT\n(ms)")
    else:
        axes[0].text(.5, .08,
                     "Reference Mars RTT: 1800 s — propagation delay not emulated in this run",
                     transform=axes[0].transAxes, ha="center", color="#7A5195",
                     fontweight="bold", bbox={"facecolor": "white", "alpha": .85, "edgecolor": "#AAA"})

    if not samples.empty:
        samples["t"] = samples.timestamp_utc.map(lambda value: (utc(value) - start).total_seconds())
        samples = samples[(samples.t >= 0) & (samples.t <= total)]
        axes[resource_cpu_axis].plot(samples.t, pd.to_numeric(samples.cpu_percent, errors="coerce"),
                                     color="#D1495B", linewidth=1.2)
        axes[resource_mem_axis].plot(samples.t, pd.to_numeric(samples.rss_kib, errors="coerce") / 1024,
                                     color="#009E73", linewidth=1.2)
    axes[resource_cpu_axis].set_ylabel("Receiver CPU\n(%)")
    axes[resource_mem_axis].set_ylabel("Receiver RSS\n(MB)")
    axes[resource_mem_axis].set_xlabel("Elapsed time since transfer start (seconds)")

    for ax in axes:
        ax.set_xlim(0, total)
    axes[0].legend(loc="upper right")
    mode = " contact-only" if contact_only else ""
    axes[0].set_title(
        f"{scenario.title()} persistent-DTN{mode} timeline — {DURATION_LABELS[int(row.interruption_duration_seconds)]} interruption",
        fontweight="bold")
    if contact_only:
        rtt_note = "Applied propagation RTT: 0 ms | Reference RTT: 1800 s"
    else:
        rtt_note = f"Applied propagation RTT: {applied_rtt_ms:g} ms | Reference RTT: {reference_rtt_ms:g} ms"
    note = (f"{rtt_note} | Applied bandwidth: {bw:g} Mbit/s | "
            f"Payload: {float(row.payload_size_mb):.2f} MB | Total: {float(row.duration_seconds):.2f} s | "
            f"delivery after reconnection and digest match: yes | network values: {applied_state['source']}")
    fig.text(.5, .012, note, ha="center", fontsize=9)
    fig.tight_layout(rect=(0, .035, 1, 1))
    for suffix in ("png", "svg"):
        fig.savefig(OUT / f"timeline_{run_id}.{suffix}", dpi=200, bbox_inches="tight")
    plt.close(fig)


# Latest valid sample for every scenario and duration.
latest = valid.sort_values("finish_time").groupby(
    ["network_scenario", "interruption_duration_seconds"], as_index=False).tail(1)
for row in latest.itertuples(index=False):
    timeline(row)

# Separate parameter comparison: reference RTT and actually applied RTT must
# never be confused, especially for the Mars contact-only profile.
scenarios = ["leo", "lunar", "mars"]
x = np.arange(3)
reference = [REFERENCE_RTT_MS[s] for s in scenarios]
# Use the latest valid run metadata/state for each scenario. This comparison is
# descriptive; every individual timeline independently reloads its own state.
applied = []
for scenario in scenarios:
    rows = latest[latest.network_scenario == scenario]
    applied.append(applied_network_state(rows.iloc[-1])["applied_rtt_ms"] if len(rows) else np.nan)
fig, ax = plt.subplots(figsize=(9, 5))
ax.plot(x, reference, "o--", linewidth=2, label="Reference RTT")
ax.plot(x, np.maximum(applied, .1), "s-", linewidth=2, label="RTT actually applied")
ax.set_yscale("log")
ax.set_xticks(x, [s.upper() for s in scenarios])
ax.set_ylabel("Round-trip time (ms, logarithmic scale)")
ax.set_title("Reference RTT versus RTT applied in the DTN experiments", fontweight="bold")
ax.legend()
ax.grid(True, which="both", alpha=.25)
ax.text(2, .18, "Mars applied RTT plotted at 0.1 ms only for log-scale visibility; actual setting = 0 ms",
        ha="center", fontsize=8, color="#555")
fig.tight_layout()
for suffix in ("png", "svg"):
    fig.savefig(OUT / f"rtt_reference_vs_applied.{suffix}", dpi=220, bbox_inches="tight")
plt.close(fig)

print(f"Generated {len(latest)} corrected timelines in {OUT}")

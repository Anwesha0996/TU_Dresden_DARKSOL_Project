#!/usr/bin/env python3
"""Create a professor-facing overview of the OCI/DTN experiment method."""

from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.patches import FancyBboxPatch, FancyArrowPatch


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "store-forward" / "results" / "graphs"
OUT.mkdir(parents=True, exist_ok=True)


def box(ax, x, y, w, h, text, color, fontsize=9):
    patch = FancyBboxPatch(
        (x, y), w, h, boxstyle="round,pad=0.012,rounding_size=0.018",
        linewidth=1.5, edgecolor=color, facecolor=color + "18"
    )
    ax.add_patch(patch)
    ax.text(x + w / 2, y + h / 2, text, ha="center", va="center",
            fontsize=fontsize, fontweight="bold", wrap=True)


def arrow(ax, x1, y1, x2, y2):
    ax.add_patch(FancyArrowPatch((x1, y1), (x2, y2), arrowstyle="-|>",
                                 mutation_scale=14, linewidth=1.4, color="#4A5568"))


fig = plt.figure(figsize=(17, 14), facecolor="white")
grid = fig.add_gridspec(5, 1, height_ratios=[0.6, 2.2, 2.0, 1.75, 2.5], hspace=.54)

title = fig.add_subplot(grid[0]); title.axis("off")
title.text(.5, .72, "OCI image transfer through Helm-DTN / µD3TN",
           ha="center", fontsize=23, fontweight="bold", color="#153B5B")
title.text(.5, .20,
           "Nine Kubernetes runtime images + one Talos installer | AAP2 → BPv7 → TCPCL → Space registry",
           ha="center", fontsize=12, color="#475569")

# Transfer route
route = fig.add_subplot(grid[1]); route.set_xlim(0, 1); route.set_ylim(0, 1); route.axis("off")
route.set_title("1. Existing transfer route (Helm deploys/configures; it does not copy the image)",
                loc="left", fontsize=14, fontweight="bold")
items = [
    ("Source registry\nK8s / GHCR", "#0072B2"),
    ("Go sender\npull image", "#0072B2"),
    ("OCI layout\n+ TAR + metadata", "#009E73"),
    ("One AAP2 ADU\nBPv7 bundle", "#009E73"),
    ("Earth µD3TN\nSQLite storage", "#E69F00"),
    ("TCPCL contact\ninterrupted/restored", "#D55E00"),
    ("Space µD3TN\nGo receiver", "#7A5195"),
    ("Space registry\nSHA-256 match", "#7A5195"),
]
w, gap, x0, y, h = .105, .018, .015, .42, .27
for i, (label, color) in enumerate(items):
    x = x0 + i * (w + gap)
    box(route, x, y, w, h, label, color, 8.6)
    if i < len(items) - 1:
        arrow(route, x + w, y + h / 2, x + w + gap, y + h / 2)
route.text(.5, .16,
           "During contact loss the original ADU remains on Earth; after TCPCL handshake it is recalled and forwarded.",
           ha="center", fontsize=10.5, color="#334155",
           bbox=dict(boxstyle="round,pad=.4", facecolor="#FFF7ED", edgecolor="#F59E0B"))

# Scenarios and interruption conditions
middle = fig.add_subplot(grid[2]); middle.axis("off")
middle.set_title("2. Controlled communication profiles and outage durations",
                 loc="left", fontsize=14, fontweight="bold")
columns = ["Scenario", "Bandwidth", "Applied RTT", "Reference RTT", "Meaning"]
rows = [
    ["LEO", "15 Mbit/s", "50 ms", "50 ms", "Short-distance, low-delay profile"],
    ["Lunar", "20 Mbit/s", "2.6 s", "2.6 s", "1.3 s one-way propagation delay"],
    ["Mars", "5 Mbit/s", "0 ms", "1800 s", "Contact-only; Mars delay NOT emulated"],
]
table = middle.table(cellText=rows, colLabels=columns, cellLoc="center", colLoc="center",
                     bbox=[0, .37, 1, .55], colWidths=[.11, .14, .14, .15, .46])
table.auto_set_font_size(False); table.set_fontsize(10)
for (r, c), cell in table.get_celld().items():
    cell.set_edgecolor("#CBD5E1")
    if r == 0:
        cell.set_facecolor("#153B5B"); cell.get_text().set_color("white"); cell.get_text().set_weight("bold")
    elif r == 3:
        cell.set_facecolor("#FDF2F8")
middle.text(.02, .19, "10 s", fontsize=12, fontweight="bold", color="#0072B2")
middle.text(.115, .19, "brief loss / reconnection", fontsize=10)
middle.text(.38, .19, "5 min", fontsize=12, fontweight="bold", color="#E69F00")
middle.text(.475, .19, "medium disruption", fontsize=10)
middle.text(.70, .19, "1 h", fontsize=12, fontweight="bold", color="#D55E00")
middle.text(.775, .19, "persistent store-and-forward", fontsize=10)
middle.text(.5, .03, "Each condition targets 10 successful repetitions; failed attempts remain recorded.",
            ha="center", fontsize=10, style="italic", color="#475569")

# Explicit contact windows. These are deliberately normalized rather than
# presented as orbital/DSN schedules; actual timestamps live in the CSV.
contact = fig.add_subplot(grid[3])
contact.set_title("3. Contact-window model used for LEO, Lunar and Mars (schematic)",
                  loc="left", fontsize=14, fontweight="bold")
contact.set_xlim(0, 100); contact.set_ylim(-.7, 2.8)
contact.set_yticks([2, 1, 0], ["LEO", "Lunar", "Mars\ncontact-only"])
contact.set_xticks([0, 20, 80, 100], ["ADU submitted", "Interruption starts",
                                      "Contact restored", "Delivery/verify"])
contact.set_xlabel("Normalized experiment sequence — actual outage = 10 s, 300 s or 3600 s", labelpad=7)
for yval, color in zip([2, 1, 0], ["#0072B2", "#E69F00", "#CC79A7"]):
    contact.barh(yval, 20, left=0, height=.48, color=color, alpha=.85, label="Contact available" if yval == 2 else None)
    contact.barh(yval, 60, left=20, height=.48, color="#D55E00", alpha=.30,
                 hatch="//", edgecolor="#D55E00", label="Contact unavailable" if yval == 2 else None)
    contact.barh(yval, 20, left=80, height=.48, color=color, alpha=.85,
                 label="Contact restored" if yval == 2 else None)
contact.axvline(20, color="#D55E00", linestyle="--", linewidth=1.2)
contact.axvline(80, color="#009E73", linestyle="--", linewidth=1.2)
contact.grid(axis="x", alpha=.22)
contact.legend(ncol=3, loc="upper center", bbox_to_anchor=(.5, 1.01), frameon=False)

# Observed behavior / limitations
bottom = fig.add_subplot(grid[4]); bottom.axis("off")
bottom.set_title("4. What happened and what the result means", loc="left",
                 fontsize=14, fontweight="bold")
left_text = (
    "SHORT / MEDIUM OUTAGES\n"
    "• TCPCL reconnects after contact returns.\n"
    "• Receiver pushes OCI image to Space registry.\n"
    "• Matching digests prove byte-level integrity.\n"
    "• Most 10 s and 5 min conditions reached 10 successes.\n\n"
    "LONG OUTAGES\n"
    "• TCP session disappears; TCPCL must handshake again.\n"
    "• SQLite keeps the original BPv7 bundle.\n"
    "• Recall must wait until the TCPCL link is active.\n"
    "• Many 1 h runs passed, but often took >1 h wall time."
)
right_text = (
    "KEY LIMITATIONS\n"
    "• WSL/host suspension pauses Linux sleep timers.\n"
    "• Mars propagation RTT was not applied (contact-only).\n"
    "• Local CPU/RSS/throughput are not flight-hardware values.\n"
    "• TCPCL depends on TCP connection/timeout behavior.\n"
    "• One large ADU has no application-level partial resume.\n"
    "• Some historical reconnection markers are incomplete.\n"
    "• The full 10-image matrix is still running.\n\n"
    "CORRECT CLAIM\n"
    "Successful rows prove OCI delivery, registry push and digest\n"
    "integrity under the recorded applied network conditions."
)
box(bottom, .015, .06, .475, .80, left_text, "#009E73", 10)
box(bottom, .51, .06, .475, .80, right_text, "#D55E00", 10)

fig.text(.5, .012,
         "Helm-DTN provides the reproducible DTN environment; Go sender/receiver + AAP2/BPv7/µD3TN perform the transfer.",
         ha="center", fontsize=10, color="#475569")

for suffix in ("png", "svg"):
    fig.savefig(OUT / f"method_explanation_chart.{suffix}", dpi=200, bbox_inches="tight")
plt.close(fig)
print(f"Created {OUT / 'method_explanation_chart.png'}")

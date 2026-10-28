#!/usr/bin/env python3
import csv
import statistics
from collections import defaultdict
from pathlib import Path

root = Path(__file__).resolve().parents[1]
raw_path = root / "results" / "evaluation_results.csv"
summary_path = root / "results" / "evaluation_summary.csv"
scoped_raw_path = root / "results" / "cluster10_evaluation_results.csv"
with raw_path.open(newline="", encoding="utf-8") as f:
    rows = list(csv.DictReader(f))
config_path = root / "config" / "images.yaml"
configured_images = {
    line.split(":", 1)[1].strip()
    for line in config_path.read_text(encoding="utf-8").splitlines()
    if line.startswith("  - name:")
}
scoped_rows = [row for row in rows if row["image_name"] in configured_images]
with scoped_raw_path.open("w", newline="", encoding="utf-8") as f:
    writer = csv.DictWriter(f, fieldnames=rows[0].keys())
    writer.writeheader()
    writer.writerows(scoped_rows)
groups = defaultdict(list)
for row in scoped_rows:
    groups[(row["image_name"], row["network_condition"])].append(row)
fields = ["image_name","network_condition","run_count","success_count","failure_count","min_duration_seconds","max_duration_seconds","average_duration_seconds","standard_deviation_seconds","average_payload_size_bytes","peak_sender_memory_mb","peak_receiver_memory_mb","peak_sender_cpu_percent","peak_receiver_cpu_percent","average_network_latency_ms","digest_match_count","digest_mismatch_count","completed_after_reconnection_count","average_throughput_mb_s","average_throughput_mbit_s"]
with summary_path.open("w", newline="", encoding="utf-8") as f:
    writer = csv.DictWriter(f, fieldnames=fields); writer.writeheader()
    for (image, condition), items in sorted(groups.items()):
        successful = [r for r in items if r["success"].lower() == "true"]
        durations = [float(r["duration_seconds"]) for r in successful]
        writer.writerow({
            "image_name": image, "network_condition": condition, "run_count": len(items),
            "success_count": len(successful), "failure_count": len(items)-len(successful),
            "min_duration_seconds": min(durations) if durations else "", "max_duration_seconds": max(durations) if durations else "",
            "average_duration_seconds": statistics.mean(durations) if durations else "",
            "standard_deviation_seconds": statistics.stdev(durations) if len(durations)>1 else 0 if durations else "",
            "average_payload_size_bytes": statistics.mean(int(r["payload_size_bytes"]) for r in successful) if successful else "",
            "peak_sender_memory_mb": max((float(r["sender_peak_memory_mb"]) for r in successful), default=""),
            "peak_receiver_memory_mb": max((float(r["receiver_peak_memory_mb"]) for r in successful), default=""),
            "peak_sender_cpu_percent": max((float(r["sender_cpu_percent"]) for r in successful), default=""),
            "peak_receiver_cpu_percent": max((float(r["receiver_peak_cpu_percent"]) for r in successful), default=""),
            "average_network_latency_ms": statistics.mean(float(r["network_latency_ms"]) for r in successful if r["network_latency_ms"]) if any(r["network_latency_ms"] for r in successful) else "",
            "digest_match_count": sum(r["digest_match"].lower()=="true" for r in items),
            "digest_mismatch_count": sum(r["digest_match"].lower()!="true" for r in items),
            "completed_after_reconnection_count": sum(r["completed_after_reconnection"].lower()=="true" for r in items),
            "average_throughput_mb_s": statistics.mean(float(r["throughput_mb_s"]) for r in successful) if successful else "",
            "average_throughput_mbit_s": statistics.mean(float(r["throughput_mbit_s"]) for r in successful) if successful else "",
        })
print(summary_path)

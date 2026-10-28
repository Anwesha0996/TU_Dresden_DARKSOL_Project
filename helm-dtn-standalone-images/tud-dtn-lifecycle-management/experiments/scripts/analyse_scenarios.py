#!/usr/bin/env python3
import csv, statistics
from collections import defaultdict
from pathlib import Path

root = Path(__file__).resolve().parents[1] / "scenarios"
source = root / "results" / "scenario_results.csv"
target = root / "results" / "scenario_summary.csv"
with source.open(newline="", encoding="utf-8") as f:
    rows = list(csv.DictReader(f))
groups = defaultdict(list)
for row in rows:
    groups[(row["image_name"], row["network_scenario"], int(row["interruption_duration_seconds"]))].append(row)
fields = ["image_name","network_scenario","interruption_duration_seconds","run_count","success_count","failure_count","digest_match_count","effective_interruption_count","completed_after_reconnection_count","average_duration_seconds","duration_stddev_seconds","average_payload_size_mb","average_throughput_mbit_s","peak_sender_cpu_percent","peak_receiver_cpu_percent","peak_sender_memory_mb","peak_receiver_memory_mb","configured_rtt","configured_bandwidth","errors"]
with target.open("w", newline="", encoding="utf-8") as f:
    writer=csv.DictWriter(f,fieldnames=fields); writer.writeheader()
    for (image,scenario,outage),items in sorted(groups.items()):
        ok=[r for r in items if r["success"].lower()=="true"]
        durations=[float(r["duration_seconds"]) for r in ok]
        def average(field):
            values=[float(r[field]) for r in ok if r[field]]
            return statistics.mean(values) if values else ""
        def peak(field):
            values=[float(r[field]) for r in ok if r[field]]
            return max(values) if values else ""
        writer.writerow({"image_name":image,"network_scenario":scenario,"interruption_duration_seconds":outage,"run_count":len(items),"success_count":len(ok),"failure_count":len(items)-len(ok),"digest_match_count":sum(r["digest_match"].lower()=="true" for r in items),"effective_interruption_count":sum(r.get("interruption_effective","").lower()=="true" for r in items),"completed_after_reconnection_count":sum(r["completed_after_reconnection"].lower()=="true" for r in items),"average_duration_seconds":statistics.mean(durations) if durations else "","duration_stddev_seconds":statistics.stdev(durations) if len(durations)>1 else 0 if durations else "","average_payload_size_mb":average("payload_size_mb"),"average_throughput_mbit_s":average("throughput_mbit_s"),"peak_sender_cpu_percent":peak("sender_cpu_percent"),"peak_receiver_cpu_percent":peak("receiver_peak_cpu_percent"),"peak_sender_memory_mb":peak("sender_peak_memory_mb"),"peak_receiver_memory_mb":peak("receiver_peak_memory_mb"),"configured_rtt":items[0]["configured_rtt"],"configured_bandwidth":items[0]["configured_bandwidth"],"errors":"; ".join(sorted({r["error_message"] for r in items if r["error_message"]}))})
print(target)


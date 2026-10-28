#!/usr/bin/env python3
import csv
from datetime import datetime
from pathlib import Path

root=Path(__file__).resolve().parents[1]/"scenarios"
summary=root/"results"/"scenario_summary.csv"
out=root/"results"/"graphs"; out.mkdir(parents=True,exist_ok=True)
with summary.open(newline="",encoding="utf-8") as f: rows=list(csv.DictReader(f))
scenarios=["leo","lunar","mars"]
durations=[10,300,3600]
colors={"leo":"#2878B5","lunar":"#F2A104","mars":"#D1495B"}

def lookup(s,d,field):
    match=next((r for r in rows if r["network_scenario"]==s and int(r["interruption_duration_seconds"])==d),None)
    return float(match[field]) if match and match[field] else 0

def graph(filename,title,field,unit):
    W,H=1400,760; left,bottom,top,right=120,620,100,50; pw=W-left-right; ph=bottom-top
    vals=[lookup(s,d,field) for d in durations for s in scenarios]; ceiling=max(vals or [1])*1.15 or 1
    svg=[f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}">','<style>text{font-family:Arial}.title{font-size:25px;font-weight:bold}.axis{stroke:#222}.grid{stroke:#ddd}.label{font-size:14px}</style>',f'<rect width="{W}" height="{H}" fill="white"/><text x="{W/2}" y="42" text-anchor="middle" class="title">{title}</text>',f'<text x="{W/2}" y="68" text-anchor="middle">LEO, Lunar, Mars; common interruption durations; 10 repetitions intended</text>']
    for t in range(6):
        y=bottom-ph*t/5; v=ceiling*t/5
        svg.append(f'<line x1="{left}" y1="{y}" x2="{W-right}" y2="{y}" class="grid"/><text x="{left-10}" y="{y+5}" text-anchor="end">{v:.1f}</text>')
    group=pw/3; bw=group/5
    for di,d in enumerate(durations):
        for si,s in enumerate(scenarios):
            v=lookup(s,d,field); h=ph*v/ceiling; x=left+di*group+group/2+(si-1)*bw-bw/2
            svg.append(f'<rect x="{x}" y="{bottom-h}" width="{bw-4}" height="{h}" fill="{colors[s]}"><title>{s}: {v:.3f} {unit}</title></rect>')
        svg.append(f'<text x="{left+(di+.5)*group}" y="{bottom+28}" text-anchor="middle" class="label">{d} seconds</text>')
    for i,s in enumerate(scenarios):
        x=W-410+i*120; svg.append(f'<rect x="{x}" y="78" width="16" height="16" fill="{colors[s]}"/><text x="{x+22}" y="92">{s.upper()}</text>')
    svg.append(f'<line x1="{left}" y1="{top}" x2="{left}" y2="{bottom}" class="axis"/><line x1="{left}" y1="{bottom}" x2="{W-right}" y2="{bottom}" class="axis"/><text x="30" y="{(top+bottom)/2}" transform="rotate(-90 30 {(top+bottom)/2})" text-anchor="middle">{unit}</text><text x="{W/2}" y="700" text-anchor="middle">Interruption duration</text></svg>')
    path=out/filename; path.write_text("\n".join(svg),encoding="utf-8"); print(path)

graph("scenario_duration_comparison.svg","Average end-to-end transfer duration","average_duration_seconds","seconds")
graph("scenario_throughput_comparison.svg","Average effective throughput","average_throughput_mbit_s","Mbit/s")
graph("scenario_success_comparison.svg","Successful runs","success_count","successful runs")
graph("scenario_sender_cpu_comparison.svg","Peak sender CPU","peak_sender_cpu_percent","CPU %")
graph("scenario_receiver_memory_comparison.svg","Peak receiver memory","peak_receiver_memory_mb","MB")

raw_path=root/"results"/"scenario_results.csv"
with raw_path.open(newline="",encoding="utf-8") as f:
    raw_rows=list(csv.DictReader(f))

def parse_time(value):
    return datetime.fromisoformat(value.replace("Z","+00:00"))

def numeric_rate(value):
    value=value.strip().lower()
    if value.endswith("mbit"):
        return float(value[:-4])
    if value.endswith("kbit"):
        return float(value[:-4])/1000
    return float(value)

def detailed_timeline(row):
    run_id=row["run_id"]
    samples_path=root/"work"/run_id/"receiver-resources.csv"
    if not samples_path.exists():
        return
    with samples_path.open(newline="",encoding="utf-8") as f:
        samples=list(csv.DictReader(f))
    if not samples:
        return
    start=parse_time(row["start_time"])
    finish=parse_time(row["finish_time"])
    outage_start=parse_time(row["interruption_start_time"])
    reconnect=parse_time(row["reconnection_time"])
    duration=max((finish-start).total_seconds(),1)
    points=[]
    for sample in samples:
        t=(parse_time(sample["timestamp_utc"])-start).total_seconds()
        if 0 <= t <= duration:
            points.append((t,float(sample["cpu_percent"]),float(sample["rss_kib"])/1024))
    if not points:
        return

    W,H=1500,1120; left,right=130,80; plot_w=W-left-right
    panel_tops=[130,350,570,790]; panel_h=150
    outage_x1=left+plot_w*max(0,(outage_start-start).total_seconds())/duration
    outage_x2=left+plot_w*min(duration,(reconnect-start).total_seconds())/duration
    svg=[f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}">',
         '<style>text{font-family:Arial}.title{font-size:26px;font-weight:bold}.subtitle{font-size:15px}.axis{stroke:#222;stroke-width:1.5}.grid{stroke:#ddd}.series{fill:none;stroke-width:3}.note{font-size:13px}</style>',
         f'<rect width="{W}" height="{H}" fill="white"/><text x="{W/2}" y="42" text-anchor="middle" class="title">Transfer timeline: {run_id}</text>',
         f'<text x="{W/2}" y="70" text-anchor="middle" class="subtitle">Shaded area = network interruption; samples are receiver process measurements</text>']

    def panel(index,title,unit,values,color,maximum=None,step=False):
        top=panel_tops[index]; bottom=top+panel_h
        vmax=maximum if maximum is not None else max(values+[1])*1.1
        vmax=max(vmax,1)
        svg.append(f'<rect x="{outage_x1}" y="{top}" width="{max(1,outage_x2-outage_x1)}" height="{panel_h}" fill="#f8c7c7" opacity="0.7"/>')
        for tick in range(5):
            y=bottom-panel_h*tick/4; val=vmax*tick/4
            svg.append(f'<line x1="{left}" y1="{y}" x2="{W-right}" y2="{y}" class="grid"/><text x="{left-10}" y="{y+5}" text-anchor="end">{val:.1f}</text>')
        coords=[]
        if step:
            a=max(0,(outage_start-start).total_seconds()); b=min(duration,(reconnect-start).total_seconds())
            step_points=[(0,values[0]),(a,values[0]),(a,0),(b,0),(b,values[0]),(duration,values[0])]
            coords=[(left+plot_w*t/duration,bottom-panel_h*v/vmax) for t,v in step_points]
        else:
            coords=[(left+plot_w*t/duration,bottom-panel_h*v/vmax) for (t,_,_),v in zip(points,values)]
        svg.append(f'<polyline points="{" ".join(f"{x:.1f},{y:.1f}" for x,y in coords)}" class="series" stroke="{color}"/>')
        svg.append(f'<line x1="{left}" y1="{top}" x2="{left}" y2="{bottom}" class="axis"/><line x1="{left}" y1="{bottom}" x2="{W-right}" y2="{bottom}" class="axis"/>')
        svg.append(f'<text x="35" y="{top+panel_h/2}" transform="rotate(-90 35 {top+panel_h/2})" text-anchor="middle">{unit}</text><text x="{left}" y="{top-14}" font-weight="bold">{title}</text>')

    bandwidth=numeric_rate(row["configured_bandwidth"])
    panel(0,"Configured link bandwidth / availability","Mbit/s",[bandwidth],"#2878B5",max(bandwidth*1.15,1),True)
    rtt_ms=float(row["configured_rtt"].replace("ms","").replace("s",""))*(1 if row["configured_rtt"].endswith("ms") else 1000)
    # Latency is a configured constant while connected; no RTT exists while the link is interrupted.
    panel(1,"Configured round-trip latency (RTT)","milliseconds",[rtt_ms],"#7A5195",max(rtt_ms*1.15,1),True)
    cpu=[p[1] for p in points]; panel(2,"Receiver CPU over time","CPU %",cpu,"#D1495B")
    memory=[p[2] for p in points]; panel(3,"Receiver resident memory over time","MB",memory,"#2A9D8F")
    for tick in range(7):
        t=duration*tick/6; x=left+plot_w*t/duration
        svg.append(f'<line x1="{x}" y1="940" x2="{x}" y2="948" class="axis"/><text x="{x}" y="970" text-anchor="middle">{t:.0f}</text>')
    svg.append(f'<text x="{W/2}" y="1005" text-anchor="middle">Seconds since transfer start</text>')
    svg.append(f'<text x="{left}" y="1040" class="note">Sender peak CPU: {row["sender_cpu_percent"] or "N/A"}% (run-level peak; no sender time-series was collected)</text>')
    svg.append(f'<text x="{left}" y="1065" class="note">Interruption requested: {row["interruption_duration_seconds"]} s | total transfer: {float(row["duration_seconds"]):.2f} s | success: {row["success"]} | digest match: {row["digest_match"]}</text></svg>')
    path=out/f"timeline_{run_id}.svg"; path.write_text("\n".join(svg),encoding="utf-8"); print(path)

# Produce one detailed timeline for the latest completed successful run of each
# currently available scenario/duration condition. More files appear as the matrix progresses.
latest={}
for row in raw_rows:
    if row["success"].lower()=="true":
        latest[(row["network_scenario"],row["interruption_duration_seconds"])]=row
for row in latest.values():
    detailed_timeline(row)

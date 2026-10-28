#!/usr/bin/env python3
"""Generate dependency-free SVG graphs from measured experiment results."""
import csv
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "results" / "graphs"
OUT.mkdir(parents=True, exist_ok=True)
with (ROOT / "results" / "cluster10_evaluation_results.csv").open(newline="", encoding="utf-8") as f:
    rows = list(csv.DictReader(f))
groups = defaultdict(list)
for row in rows:
    groups[row["image_name"], row["network_condition"]].append(row)
images = sorted({row["image_name"] for row in rows})

def value(image, condition, field, peak=False):
    numbers = [float(r[field]) for r in groups[image, condition] if r[field]]
    return max(numbers) if peak else sum(numbers) / len(numbers)

def panel(svg, x, y, width, height, title, unit, series):
    svg.append(f'<text x="{x}" y="{y-18}" class="panel">{title}</text>')
    svg.append(f'<line x1="{x}" y1="{y}" x2="{x}" y2="{y+height}" class="axis"/>')
    svg.append(f'<line x1="{x}" y1="{y+height}" x2="{x+width}" y2="{y+height}" class="axis"/>')
    ceiling = max(max(numbers) for _, numbers, _ in series) * 1.12 or 1
    group_width, bar_width = width / len(images), min(28, width / len(images) * .7 / len(series))
    for tick in range(5):
        py, number = y + height - height*tick/4, ceiling*tick/4
        svg.append(f'<line x1="{x}" y1="{py}" x2="{x+width}" y2="{py}" class="grid"/>')
        svg.append(f'<text x="{x-8}" y="{py+4}" text-anchor="end" class="tick">{number:.1f}</text>')
    for series_index, (legend, numbers, color) in enumerate(series):
        for index, number in enumerate(numbers):
            bx = x+(index+.5)*group_width+(series_index-(len(series)-1)/2)*bar_width-bar_width/2
            bar_height = height*number/ceiling
            svg.append(f'<rect x="{bx:.1f}" y="{y+height-bar_height:.1f}" width="{bar_width-2:.1f}" height="{bar_height:.1f}" fill="{color}"><title>{images[index]}: {number:.3f} {unit}</title></rect>')
        lx = x+width-145*len(series)+series_index*145
        svg.append(f'<rect x="{lx}" y="{y-34}" width="14" height="14" fill="{color}"/><text x="{lx+20}" y="{y-22}" class="label">{legend}</text>')
    for index, image in enumerate(images):
        lx, label = x+(index+.5)*group_width, image.replace("kube-", "k-").replace("talos-", "t-")
        svg.append(f'<text x="{lx}" y="{y+height+18}" text-anchor="end" transform="rotate(-35 {lx} {y+height+18})" class="label">{label}</text>')
    svg.append(f'<text x="{x-58}" y="{y+height/2}" text-anchor="middle" transform="rotate(-90 {x-58} {y+height/2})" class="label">{unit}</text>')

def write_graph(filename, title, panels):
    svg = ['<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1120" viewBox="0 0 1600 1120">',
      '<style>text{font-family:Arial,sans-serif;fill:#17202a}.title{font-size:26px;font-weight:bold}.subtitle{font-size:16px;fill:#566573}.panel{font-size:19px;font-weight:bold}.label{font-size:13px}.tick{font-size:11px}.axis{stroke:#273746;stroke-width:1.5}.grid{stroke:#d5d8dc}</style>',
      '<rect width="1600" height="1120" fill="white"/>',
      f'<text x="800" y="42" text-anchor="middle" class="title">{title}</text>',
      '<text x="800" y="70" text-anchor="middle" class="subtitle">10 measured runs per image; averages and peaks calculated from the experiment CSV</text>']
    for args in panels:
        panel(svg, *args)
    svg.append("</svg>")
    path = OUT / filename
    path.write_text("\n".join(svg), encoding="utf-8")
    print(path)

def dashboard(condition):
    label = "Normal network" if condition == "normal" else "Temporary TCPCL interruption (10 seconds)"
    def vals(field, peak=False): return [value(i, condition, field, peak) for i in images]
    write_graph(f"{condition}_network_results.svg", f"Talos/Kubernetes image transfer — {label}", [
      (110,130,650,340,"Average end-to-end duration","seconds",[(label,vals("duration_seconds"),"#3572A5")]),
      (900,130,650,340,"Average effective throughput","Mbit/s",[(label,vals("throughput_mbit_s"),"#2A9D8F")]),
      (110,650,650,340,"Peak process memory","MB",[("Sender",vals("sender_peak_memory_mb",True),"#E9C46A"),("Receiver",vals("receiver_peak_memory_mb",True),"#F4A261")]),
      (900,650,650,340,"Peak measured process CPU","CPU %",[("Sender",vals("sender_cpu_percent",True),"#7B2CBF"),("Receiver",vals("receiver_peak_cpu_percent",True),"#C77DFF")])])

dashboard("normal")
dashboard("interruption")
write_graph("normal_vs_interruption.svg", "Normal vs temporary-interruption image transfer", [
 (110,130,1380,340,"Average end-to-end duration","seconds",[("Normal",[value(i,"normal","duration_seconds") for i in images],"#3572A5"),("Interrupted",[value(i,"interruption","duration_seconds") for i in images],"#E76F51")]),
 (110,650,1380,340,"Average effective throughput","Mbit/s",[("Normal",[value(i,"normal","throughput_mbit_s") for i in images],"#2A9D8F"),("Interrupted",[value(i,"interruption","throughput_mbit_s") for i in images],"#F4A261")])])

def line_panel(svg, x, y, width, height, title, unit, series):
    svg.append(f'<text x="{x}" y="{y-18}" class="panel">{title}</text>')
    maximum = max(max(numbers) for _, numbers, _ in series) * 1.08 or 1
    for tick in range(5):
        py, number = y+height-height*tick/4, maximum*tick/4
        svg.append(f'<line x1="{x}" y1="{py}" x2="{x+width}" y2="{py}" class="grid"/><text x="{x-8}" y="{py+4}" text-anchor="end" class="tick">{number:.2f}</text>')
    svg.append(f'<line x1="{x}" y1="{y}" x2="{x}" y2="{y+height}" class="axis"/><line x1="{x}" y1="{y+height}" x2="{x+width}" y2="{y+height}" class="axis"/>')
    for si, (legend, numbers, color) in enumerate(series):
        points = " ".join(f'{x+i*width/(len(numbers)-1):.1f},{y+height-height*n/maximum:.1f}' for i,n in enumerate(numbers))
        svg.append(f'<polyline points="{points}" fill="none" stroke="{color}" stroke-width="2"><title>{legend}</title></polyline>')
        lx=x+width-145*len(series)+si*145
        svg.append(f'<line x1="{lx}" y1="{y-28}" x2="{lx+16}" y2="{y-28}" stroke="{color}" stroke-width="3"/><text x="{lx+22}" y="{y-23}" class="label">{legend}</text>')
    svg.append(f'<text x="{x-58}" y="{y+height/2}" text-anchor="middle" transform="rotate(-90 {x-58} {y+height/2})" class="label">{unit}</text><text x="{x+width/2}" y="{y+height+28}" text-anchor="middle" class="label">Run sequence (grouped by image, 10 repetitions each)</text>')

def trend_graph(condition):
    ordered = sorted((r for r in rows if r["network_condition"] == condition), key=lambda r:(images.index(r["image_name"]),int(r["repetition"])))
    def nums(field): return [float(r[field]) for r in ordered]
    label = "Normal network" if condition == "normal" else "Temporary TCPCL interruption"
    svg=['<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1120" viewBox="0 0 1600 1120">','<style>text{font-family:Arial,sans-serif;fill:#17202a}.title{font-size:26px;font-weight:bold}.subtitle{font-size:16px;fill:#566573}.panel{font-size:19px;font-weight:bold}.label{font-size:13px}.tick{font-size:11px}.axis{stroke:#273746;stroke-width:1.5}.grid{stroke:#d5d8dc}</style>','<rect width="1600" height="1120" fill="white"/>',f'<text x="800" y="42" text-anchor="middle" class="title">Per-run performance trends — {label}</text>','<text x="800" y="70" text-anchor="middle" class="subtitle">Peaks show exactly when a measured value was high; valleys show when it was low</text>']
    line_panel(svg,110,130,650,340,"Transfer delay / duration","seconds",[("Duration",nums("duration_seconds"),"#E76F51")])
    line_panel(svg,900,130,650,340,"Process CPU usage","CPU %",[("Sender",nums("sender_cpu_percent"),"#7B2CBF"),("Receiver",nums("receiver_peak_cpu_percent"),"#C77DFF")])
    line_panel(svg,110,650,650,340,"Measured network latency","milliseconds",[("Latency",nums("network_latency_ms"),"#F4A261")])
    line_panel(svg,900,650,650,340,"Effective bandwidth / throughput","Mbit/s",[("Bandwidth",nums("throughput_mbit_s"),"#2A9D8F")])
    svg.append('</svg>')
    path=OUT/f"{condition}_run_trends.svg"; path.write_text("\n".join(svg),encoding="utf-8"); print(path)

trend_graph("normal")
trend_graph("interruption")

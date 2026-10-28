#!/usr/bin/env python3
"""Create a concise one-sheet Excel progress report from real experiment rows."""

import csv
import zipfile
from datetime import datetime
from pathlib import Path
from xml.sax.saxutils import escape

REPO = Path(__file__).resolve().parents[2]
ROOT = REPO / "experiments" / "scenarios"
RAW = ROOT / "results" / "scenario_results.csv"
OUTPUT = ROOT / "results" / "current_progress_brief.xlsx"


def column(index):
    value = ""
    while index:
        index, remainder = divmod(index - 1, 26)
        value = chr(65 + remainder) + value
    return value


def cell(reference, value, style=0):
    if value in (None, ""):
        return f'<c r="{reference}" s="{style}"/>'
    if isinstance(value, (int, float)):
        return f'<c r="{reference}" s="{style}"><v>{value}</v></c>'
    return f'<c r="{reference}" s="{style}" t="inlineStr"><is><t>{escape(str(value))}</t></is></c>'


with RAW.open(newline="", encoding="utf-8") as handle:
    data = list(csv.DictReader(handle))

planned = 90
recorded = len(data)
success = sum(row["success"].lower() == "true" for row in data)
failure = sum(row["success"].lower() == "false" for row in data)
pending = max(0, planned - recorded)

rows = [
    ["SPACE-NETWORK IMAGE TRANSFER — ABRIDGED PROGRESS REPORT", "", "", "", ""],
    ["Snapshot", datetime.now().astimezone().isoformat(timespec="seconds"), "", "", ""],
    ["Objective", "Evaluate the existing OCI/AAP2/BPv7/µD3TN image-transfer route under LEO, Lunar and Mars conditions.", "", "", ""],
    ["Representative image", "ghcr.io/siderolabs/flannel:v0.27.4", "", "", ""],
    ["Planned matrix", "3 scenarios × 3 interruption durations × 10 repetitions", "", "", ""],
    [],
    ["OVERALL PROGRESS", "Value", "", "", ""],
    ["Planned runs", planned, "", "", ""],
    ["Recorded runs", recorded, "", "", ""],
    ["Progress", f"{recorded / planned * 100:.1f}%", "", "", ""],
    ["Successful", success, "", "", ""],
    ["Failed", failure, "", "", ""],
    ["Pending", pending, "", "", ""],
    [],
    ["CONDITION", "RUNS", "SUCCESS", "FAILURE", "STATUS / OBSERVATION"],
]

notes = {
    ("leo", 10): "Short outage recovered",
    ("leo", 300): "Five-minute outage recovered",
    ("leo", 3600): "Receiver completion timeout after long outage",
    ("lunar", 10): "Recovered with configured 2.6 s RTT",
    ("lunar", 300): "Recovered after reconnection",
    ("lunar", 3600): "Receiver completion timeout after long outage",
    ("mars", 10): "TCPCL connection timeout before BPv7 delivery",
    ("mars", 300): "TCPCL connection timeout before BPv7 delivery",
    ("mars", 3600): "Still running/pending; same TCPCL limitation expected",
}
for scenario in ("leo", "lunar", "mars"):
    for duration in (10, 300, 3600):
        group = [r for r in data if r["network_scenario"] == scenario and int(r["interruption_duration_seconds"]) == duration]
        ok = sum(r["success"].lower() == "true" for r in group)
        failed = sum(r["success"].lower() == "false" for r in group)
        label = {10: "10 seconds", 300: "5 minutes", 3600: "1 hour"}[duration]
        rows.append([f"{scenario.upper()} — {label}", len(group), ok, failed, notes[(scenario, duration)]])

rows.extend([
    [],
    ["CONFIGURED PROFILE", "RTT", "ONE-WAY DELAY", "BANDWIDTH", "SOURCE"],
    ["LEO", "50 ms", "25 ms", "6 Mbit/s", "Starlink measurement study; not from the IMC DSN paper"],
    ["Lunar", "2.6 s", "1.3 s", "20 Mbit/s", "RTT from supplied IMC DSN paper; bandwidth from NASA LLCD"],
    ["Mars", "1,800 s", "900 s", "5 Mbit/s", "RTT from supplied IMC DSN paper; bandwidth from JPL optical profile"],
    [],
    ["KEY FINDING", "10-second and 5-minute LEO/Lunar outages recover; one-hour outages time out. Literal Mars RTT prevents TCPCL connection establishment.", "", "", ""],
    ["IMPORTANT", "A blank destination digest on failed runs means Harbor was not reached; it is not evidence of image corruption.", "", "", ""],
    ["LIMITATIONS", "One representative image; 900-run all-image scope unconfirmed. Some long runs include host sleep/WSL suspension. Mars tests characterize TCPCL, not DTN in general.", "", "", ""],
    ["NEXT TEST", "A separate one-hour scheduled-contact store-and-forward validation is queued after the baseline reaches 90 rows.", "", "", ""],
    [],
    ["DETAILED EVIDENCE", "experiments/scenarios/results/scenario_results.csv", "", "", ""],
    ["RUN LOGS", "experiments/scenarios/logs/", "", "", ""],
    ["GRAPHS", "experiments/scenarios/results/graphs/", "", "", ""],
    ["FULL NOTE", "experiments/scenarios/INTERIM_RESULTS_FOR_PROFESSOR.md", "", "", ""],
])

header_rows = {1, 7, 15, 26}
xml = ['<?xml version="1.0" encoding="UTF-8" standalone="yes"?>',
       '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">',
       '<sheetViews><sheetView workbookViewId="0"><pane ySplit="5" topLeftCell="A6" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>',
       '<cols><col min="1" max="1" width="34" customWidth="1"/><col min="2" max="2" width="92" customWidth="1"/><col min="3" max="4" width="16" customWidth="1"/><col min="5" max="5" width="65" customWidth="1"/></cols><sheetData>']
for row_number, values in enumerate(rows, 1):
    xml.append(f'<row r="{row_number}">')
    for col_number, value in enumerate(values, 1):
        style = 1 if row_number in header_rows else 0
        xml.append(cell(f"{column(col_number)}{row_number}", value, style))
    xml.append('</row>')
xml.append('</sheetData><mergeCells count="8"><mergeCell ref="A1:E1"/><mergeCell ref="B3:E3"/><mergeCell ref="B4:E4"/><mergeCell ref="B5:E5"/><mergeCell ref="B31:E31"/><mergeCell ref="B32:E32"/><mergeCell ref="B33:E33"/><mergeCell ref="B34:E34"/></mergeCells></worksheet>')

content_types = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>'''
root_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>'''
workbook = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Brief Progress" sheetId="1" r:id="rId1"/></sheets></workbook>'''
workbook_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>'''
styles = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1F4E78"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" alignment="wrapText"/><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1" alignment="wrapText"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>'''

with zipfile.ZipFile(OUTPUT, "w", zipfile.ZIP_DEFLATED) as archive:
    archive.writestr("[Content_Types].xml", content_types)
    archive.writestr("_rels/.rels", root_rels)
    archive.writestr("xl/workbook.xml", workbook)
    archive.writestr("xl/_rels/workbook.xml.rels", workbook_rels)
    archive.writestr("xl/styles.xml", styles)
    archive.writestr("xl/worksheets/sheet1.xml", "".join(xml))

print(OUTPUT)
print(f"recorded={recorded} success={success} failure={failure} pending={pending}")

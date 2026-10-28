#!/usr/bin/env python3
"""Export the 90-case space-network matrix and current real results to XLSX.

Uses only the Python standard library. Pending measurements remain blank.
"""

import csv
import zipfile
from datetime import datetime, timezone
from pathlib import Path
from xml.sax.saxutils import escape


REPO = Path(__file__).resolve().parents[2]
SCENARIOS = REPO / "experiments" / "scenarios"
RAW = SCENARIOS / "results" / "scenario_results.csv"
OUTPUT = SCENARIOS / "results" / "scenario_case_list.xlsx"


def col_name(index):
    result = ""
    while index:
        index, remainder = divmod(index - 1, 26)
        result = chr(65 + remainder) + result
    return result


def cell(ref, value, style=0):
    if value is None or value == "":
        return f'<c r="{ref}" s="{style}"/>'
    text = str(value)
    try:
        number = float(text)
        if text.lower() not in {"true", "false"} and not any(ch in text for ch in "T:Z/"):
            return f'<c r="{ref}" s="{style}"><v>{number}</v></c>'
    except ValueError:
        pass
    return f'<c r="{ref}" s="{style}" t="inlineStr"><is><t>{escape(text)}</t></is></c>'


def worksheet(rows, widths, freeze=True, autofilter=True):
    xml = ['<?xml version="1.0" encoding="UTF-8" standalone="yes"?>',
           '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">']
    if freeze:
        xml.append('<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>')
    xml.append('<cols>')
    for i, width in enumerate(widths, 1):
        xml.append(f'<col min="{i}" max="{i}" width="{width}" customWidth="1"/>')
    xml.append('</cols><sheetData>')
    for row_index, values in enumerate(rows, 1):
        xml.append(f'<row r="{row_index}">')
        for col_index, value in enumerate(values, 1):
            style = 1 if row_index == 1 else 0
            xml.append(cell(f'{col_name(col_index)}{row_index}', value, style))
        xml.append('</row>')
    xml.append('</sheetData>')
    if autofilter and rows:
        xml.append(f'<autoFilter ref="A1:{col_name(len(rows[0]))}{len(rows)}"/>')
    xml.append('</worksheet>')
    return "".join(xml)


with RAW.open(newline="", encoding="utf-8") as handle:
    actual_rows = list(csv.DictReader(handle))
actual = {row["run_id"]: row for row in actual_rows}

case_headers = [
    "Case ID", "Scenario", "Configured RTT", "One-way delay", "Configured bandwidth",
    "Packet loss", "Interruption (s)", "Interruption label", "Repetition", "Image",
    "Source image", "Destination image", "Status", "Success", "Error message",
    "Payload bytes", "Payload MB", "Start time", "Finish time", "Duration (s)",
    "Sender peak memory (MB)", "Receiver peak memory (MB)", "Sender CPU (%)",
    "Receiver peak CPU (%)", "Throughput (MB/s)", "Throughput (Mbit/s)",
    "Interruption start", "Reconnection time", "Completed after reconnection",
    "Source digest", "Destination digest", "Digest match"
]

profiles = {
    "leo": ("50 ms", "25 ms", "6 Mbit/s", "0%"),
    "lunar": ("2.6 s", "1.3 s", "20 Mbit/s", "0%"),
    "mars": ("1,800 s", "900 s", "5 Mbit/s", "0%"),
}
labels = {10: "10 seconds", 300: "5 minutes", 3600: "1 hour"}
case_rows = [case_headers]
for scenario in ("leo", "lunar", "mars"):
    for duration in (10, 300, 3600):
        for repetition in range(1, 11):
            run_id = f"flannel_{scenario}_interrupt{duration}s_run{repetition:02d}"
            row = actual.get(run_id)
            rtt, one_way, bandwidth, loss = profiles[scenario]
            status = "Pending"
            if row:
                status = "Completed - success" if row["success"].lower() == "true" else "Completed - failed"
            get = (lambda key: row.get(key, "") if row else "")
            case_rows.append([
                run_id, scenario.upper(), rtt, one_way, bandwidth, loss, duration, labels[duration],
                repetition, "flannel", "ghcr.io/siderolabs/flannel:v0.27.4",
                get("destination_image"), status, get("success"), get("error_message"),
                get("payload_size_bytes"), get("payload_size_mb"), get("start_time"),
                get("finish_time"), get("duration_seconds"), get("sender_peak_memory_mb"),
                get("receiver_peak_memory_mb"), get("sender_cpu_percent"),
                get("receiver_peak_cpu_percent"), get("throughput_mb_s"),
                get("throughput_mbit_s"), get("interruption_start_time"),
                get("reconnection_time"), get("completed_after_reconnection"),
                get("source_digest"), get("destination_digest"), get("digest_match")
            ])

parameter_rows = [["Scenario", "RTT", "One-way delay", "Bandwidth", "Loss", "Parameter provenance", "Important note"],
    ["LEO", "50 ms", "25 ms", "6 Mbit/s", "0%", "Ma et al. Starlink measurement study", "Not supplied by the IMC DSN paper"],
    ["Lunar", "2.6 s", "1.3 s", "20 Mbit/s", "0%", "RTT: supplied IMC DSN paper; bandwidth: NASA LLCD", "Paper supports RTT, not the bandwidth"],
    ["Mars", "1,800 s", "900 s", "5 Mbit/s", "0%", "RTT: supplied IMC DSN paper; bandwidth: JPL optical profile", "TCPCL times out before a literal Mars RTT completes"],
    ["All", "", "", "", "", "Interruption durations are experiment choices", "10 s, 300 s, and 3,600 s are not exact recommendations from the paper"]]

summary_rows = [["Generated UTC", datetime.now(timezone.utc).isoformat()],
                ["Planned cases", 90], ["Recorded cases", len(actual)],
                ["Pending cases", 90-len(actual)], ["Representative image", "ghcr.io/siderolabs/flannel:v0.27.4"],
                ["Scope", "3 scenarios x 3 interruption durations x 10 repetitions"],
                ["Warning", "This is an interim live export. Pending measurement cells are intentionally blank."]]
summary_rows.append([])
summary_rows.append(["Scenario", "Interruption (s)", "Recorded", "Success", "Failure"])
for scenario in ("leo", "lunar", "mars"):
    for duration in (10, 300, 3600):
        group = [r for r in actual_rows if r["network_scenario"] == scenario and int(r["interruption_duration_seconds"]) == duration]
        summary_rows.append([scenario.upper(), duration, len(group),
                             sum(r["success"].lower() == "true" for r in group),
                             sum(r["success"].lower() == "false" for r in group)])

store_forward_csv = REPO / "experiments" / "store-forward" / "results" / "store_forward_results.csv"
store_forward_log = REPO / "experiments" / "store-forward" / "queue.log"
store_forward_actual = []
if store_forward_csv.exists():
    with store_forward_csv.open(newline="", encoding="utf-8") as handle:
        store_forward_actual = list(csv.DictReader(handle))
sf = next((r for r in store_forward_actual if r["run_id"] == "flannel_leo_storeforward3600s_run01"), None)
queue_status = "Queued until baseline reaches 90 runs"
if sf:
    queue_status = "Completed - success" if sf["success"].lower() == "true" else "Completed - failed"
elif store_forward_log.exists():
    queue_text = store_forward_log.read_text(encoding="utf-8", errors="replace")
    if "starting store-forward validation" in queue_text or "store-forward started directly" in queue_text:
        queue_status = "Running"
sfget = lambda key: sf.get(key, "") if sf else ""
store_forward_rows = [
    ["ONE-HOUR STORE-AND-FORWARD VALIDATION", "Value", "Explanation"],
    ["Run ID", "flannel_leo_storeforward3600s_run01", "Separate from the baseline 90-run matrix"],
    ["Current status", queue_status, "No measurement is filled until a real run completes"],
    ["Scenario", "LEO", "50 ms RTT, 25 ms one-way delay, 6 Mbit/s"],
    ["Interruption", 3600, "TCPCL path unavailable for one hour"],
    ["Payload", "ghcr.io/siderolabs/flannel:v0.27.4", "Original ADU is submitted once"],
    ["Storage method", "µD3TN in-process bundle retention", "Does not prove persistence across a node restart"],
    ["Contact method", "Future contact + fresh contact after reconnection", "Original bundle should be forwarded when contact returns"],
    ["Application resend", "No", "Success must come from forwarding the original queued bundle"],
    ["Completion timeout", 21600, "Six hours"],
    [],
    ["REAL RESULT FIELD", "VALUE", ""],
    ["Success", sfget("success"), ""],
    ["Error message", sfget("error_message"), ""],
    ["Payload bytes", sfget("payload_size_bytes"), ""],
    ["Duration (s)", sfget("duration_seconds"), ""],
    ["Sender peak memory (MB)", sfget("sender_peak_memory_mb"), ""],
    ["Receiver peak memory (MB)", sfget("receiver_peak_memory_mb"), ""],
    ["Sender CPU (%)", sfget("sender_cpu_percent"), ""],
    ["Receiver peak CPU (%)", sfget("receiver_peak_cpu_percent"), ""],
    ["Throughput (Mbit/s)", sfget("throughput_mbit_s"), ""],
    ["Interruption start", sfget("interruption_start_time"), ""],
    ["Reconnection", sfget("reconnection_time"), ""],
    ["Completed after reconnection", sfget("completed_after_reconnection"), ""],
    ["Source digest", sfget("source_digest"), ""],
    ["Destination digest", sfget("destination_digest"), ""],
    ["Digest match", sfget("digest_match"), ""],
]

content_types = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/worksheets/sheet3.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/worksheets/sheet4.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>'''
root_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>'''
workbook = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>
<sheet name="Case List" sheetId="1" r:id="rId1"/><sheet name="Scenario Parameters" sheetId="2" r:id="rId2"/><sheet name="Status Summary" sheetId="3" r:id="rId3"/><sheet name="1h Store-Forward" sheetId="4" r:id="rId4"/>
</sheets></workbook>'''
workbook_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet3.xml"/>
<Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet4.xml"/>
<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>'''
styles = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1F4E78"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>'''

OUTPUT.parent.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(OUTPUT, "w", zipfile.ZIP_DEFLATED) as archive:
    archive.writestr("[Content_Types].xml", content_types)
    archive.writestr("_rels/.rels", root_rels)
    archive.writestr("xl/workbook.xml", workbook)
    archive.writestr("xl/_rels/workbook.xml.rels", workbook_rels)
    archive.writestr("xl/styles.xml", styles)
    archive.writestr("xl/worksheets/sheet1.xml", worksheet(case_rows, [34,12,16,16,20,12,16,18,12,14,42,55,20,12,38,16,14,26,26,16,24,26,18,24,20,22,26,26,28,70,70,14]))
    archive.writestr("xl/worksheets/sheet2.xml", worksheet(parameter_rows, [14,18,18,18,12,52,65]))
    archive.writestr("xl/worksheets/sheet3.xml", worksheet(summary_rows, [28,70,18,18,18], autofilter=False))
    archive.writestr("xl/worksheets/sheet4.xml", worksheet(store_forward_rows, [34,55,75], autofilter=False))

print(OUTPUT)

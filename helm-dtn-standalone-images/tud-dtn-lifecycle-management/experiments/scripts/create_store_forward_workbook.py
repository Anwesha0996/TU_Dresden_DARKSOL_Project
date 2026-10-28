#!/usr/bin/env python3
"""Create a dependency-free XLSX snapshot from real store-forward CSV results."""

import csv
import math
import statistics
import sys
import zipfile
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path
from xml.sax.saxutils import escape


SCENARIOS = {"leo", "lunar", "mars"}


def number(value):
    try:
        result = float(value)
        return result if math.isfinite(result) else None
    except (TypeError, ValueError):
        return None


def truth(value):
    return str(value).strip().lower() == "true"


def sval(value):
    return "" if value is None else str(value)


def cell(ref, value, style=0):
    if value is None:
        return f'<c r="{ref}" s="{style}"/>'
    if isinstance(value, bool):
        return f'<c r="{ref}" s="{style}" t="b"><v>{int(value)}</v></c>'
    if isinstance(value, (int, float)) and math.isfinite(value):
        return f'<c r="{ref}" s="{style}"><v>{value}</v></c>'
    text = escape(str(value))
    return f'<c r="{ref}" s="{style}" t="inlineStr"><is><t xml:space="preserve">{text}</t></is></c>'


def column_name(index):
    name = ""
    while index:
        index, remainder = divmod(index - 1, 26)
        name = chr(65 + remainder) + name
    return name


def worksheet(rows, widths=None, freeze=True, autofilter=True, conditional_status_col=None):
    rendered = []
    for row_no, row in enumerate(rows, 1):
        cells = []
        for col_no, value in enumerate(row, 1):
            style = 1 if row_no == 1 else 0
            if conditional_status_col == col_no and row_no > 1:
                style = 2 if str(value).lower() in {"true", "pass", "success"} else 3
            cells.append(cell(f"{column_name(col_no)}{row_no}", value, style))
        rendered.append(f'<row r="{row_no}">{"".join(cells)}</row>')
    max_col = max((len(row) for row in rows), default=1)
    max_row = max(len(rows), 1)
    cols = ""
    if widths:
        cols = "<cols>" + "".join(
            f'<col min="{i}" max="{i}" width="{width}" customWidth="1"/>'
            for i, width in enumerate(widths, 1)
        ) + "</cols>"
    pane = '<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>' if freeze else '<sheetViews><sheetView workbookViewId="0"/></sheetViews>'
    filt = f'<autoFilter ref="A1:{column_name(max_col)}{max_row}"/>' if autofilter and rows else ""
    return f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><dimension ref="A1:{column_name(max_col)}{max_row}"/>{pane}<sheetFormatPr defaultRowHeight="15"/>{cols}<sheetData>{''.join(rendered)}</sheetData>{filt}</worksheet>'''


def avg(values):
    values = [v for v in values if v is not None]
    return sum(values) / len(values) if values else None


def maxv(values):
    values = [v for v in values if v is not None]
    return max(values) if values else None


def minv(values):
    values = [v for v in values if v is not None]
    return min(values) if values else None


def create_workbook(csv_path, output_path):
    with csv_path.open(newline="", encoding="utf-8-sig") as handle:
        reader = csv.DictReader(handle)
        records = list(reader)
        fields = reader.fieldnames or []

    scenario_records = [r for r in records if sval(r.get("network_scenario")).lower() in SCENARIOS]
    groups = defaultdict(list)
    for record in scenario_records:
        groups[(sval(record["network_scenario"]).lower(), int(float(record["interruption_duration_seconds"])))].append(record)

    summary_header = [
        "Scenario", "Interruption (s)", "Runs recorded", "Success", "Failure", "Success rate (%)",
        "Avg successful duration (s)", "Min successful duration (s)", "Max successful duration (s)",
        "Duration std dev (s)", "Avg payload (MB)", "Peak sender CPU (%)", "Peak receiver CPU (%)",
        "Peak sender memory (MB)", "Peak receiver memory (MB)", "Avg throughput (Mbit/s)",
        "Digest matches", "Digest mismatches", "Completed after reconnection", "Effective interruptions"
    ]
    summary_rows = [summary_header]
    for key in sorted(groups, key=lambda k: ({"leo": 0, "lunar": 1, "mars": 2}[k[0]], k[1])):
        group = groups[key]
        successes = [r for r in group if truth(r.get("success"))]
        failures = len(group) - len(successes)
        durations = [number(r.get("duration_seconds")) for r in successes]
        valid_durations = [v for v in durations if v is not None]
        summary_rows.append([
            key[0].upper(), key[1], len(group), len(successes), failures,
            round(100 * len(successes) / len(group), 2) if group else None,
            round(avg(durations), 3) if valid_durations else None,
            round(minv(durations), 3) if valid_durations else None,
            round(maxv(durations), 3) if valid_durations else None,
            round(statistics.stdev(valid_durations), 3) if len(valid_durations) > 1 else 0,
            round(avg([number(r.get("payload_size_mb")) for r in successes]), 3) if successes else None,
            maxv([number(r.get("sender_cpu_percent")) for r in group]),
            maxv([number(r.get("receiver_peak_cpu_percent")) for r in group]),
            maxv([number(r.get("sender_peak_memory_mb")) for r in group]),
            maxv([number(r.get("receiver_peak_memory_mb")) for r in group]),
            round(avg([number(r.get("throughput_mbit_s")) for r in successes]), 4) if successes else None,
            sum(truth(r.get("digest_match")) for r in group),
            sum(r.get("digest_match", "").strip().lower() == "false" for r in group),
            sum(truth(r.get("completed_after_reconnection")) for r in group),
            sum(truth(r.get("interruption_effective")) for r in group),
        ])

    generated = datetime.now(timezone.utc).isoformat(timespec="seconds")
    finished_conditions = sum(1 for g in groups.values() if len(g) >= 10)
    executive_rows = [
        ["Store-and-forward experiment snapshot", "Value"],
        ["Generated (UTC)", generated],
        ["Source CSV", str(csv_path)],
        ["All CSV rows preserved", len(records)],
        ["Scenario-labelled rows used in summaries", len(scenario_records)],
        ["Legacy/unlabelled rows excluded from summaries", len(records) - len(scenario_records)],
        ["Completed conditions (at least 10 recorded runs)", f"{finished_conditions} of 9"],
        ["Status", "LIVE SNAPSHOT — the full matrix may still be running"],
        ["Interpretation", "Success means receiver completion plus the recorded validation outcome; failures remain visible."],
        ["Integrity", "No measurements were invented. Missing values remain blank."],
    ]

    parameter_rows = [["Scenario", "Reference/recorded RTT", "Recorded one-way delay", "Bandwidth", "Packet loss", "Rows", "Interpretation"]]
    profiles = Counter((sval(r.get("network_scenario")).upper(), sval(r.get("configured_rtt")), sval(r.get("configured_one_way_delay")), sval(r.get("configured_bandwidth")), sval(r.get("configured_packet_loss"))) for r in scenario_records)
    for profile, count in sorted(profiles.items()):
        scenario, rtt, delay, bandwidth, loss = profile
        interpretation = ""
        if scenario == "MARS" and (rtt.startswith("1800s") or rtt == "0ms"):
            interpretation = "Contact-only: Mars reference RTT was not imposed; applied propagation delay was 0 ms."
        parameter_rows.append([scenario, rtt, delay, bandwidth, loss, count, interpretation])

    quality_rows = [
        ["Data-quality check", "Count", "Meaning"],
        ["Total CSV rows", len(records), "All rows are retained in Run Details."],
        ["Rows with LEO/Lunar/Mars label", len(scenario_records), "Only these contribute to Scenario Summary."],
        ["Rows without recognized scenario", len(records) - len(scenario_records), "Historical/legacy rows excluded from comparison."],
        ["Scenario rows with missing measured latency", sum(not sval(r.get("network_latency_ms")).strip() for r in scenario_records), "Blank means latency was not measured; configured/reference RTT must not replace it."],
        ["Scenario failures", sum(not truth(r.get("success")) for r in scenario_records), "Failures are not converted into successes."],
        ["Scenario digest mismatches", sum(sval(r.get("digest_match")).strip().lower() == "false" for r in scenario_records), "Includes failed/incomplete rows recorded as false."],
    ]

    detail_rows = [fields]
    numeric_fields = {"repetition", "payload_size_bytes", "payload_size_mb", "duration_seconds", "sender_peak_memory_mb", "receiver_peak_memory_mb", "sender_cpu_percent", "receiver_peak_cpu_percent", "network_latency_ms", "interruption_duration_seconds", "throughput_mb_s", "throughput_mbit_s"}
    boolean_fields = {"digest_match", "success", "completed_after_reconnection", "interruption_effective"}
    for record in records:
        row = []
        for field in fields:
            raw = sval(record.get(field))
            if field in numeric_fields:
                row.append(number(raw))
            elif field in boolean_fields and raw.strip():
                row.append(truth(raw))
            else:
                row.append(raw)
        detail_rows.append(row)

    sheets = [
        ("Overview", executive_rows, [42, 100], False, False, None),
        ("Scenario Summary", summary_rows, [16, 17, 14, 10, 10, 17] + [23] * 14, True, True, None),
        ("Network Parameters", parameter_rows, [14, 36, 24, 15, 14, 10, 75], True, True, None),
        ("Data Quality", quality_rows, [42, 14, 95], True, True, None),
        ("Run Details", detail_rows, [24] * len(fields), True, True, fields.index("success") + 1 if "success" in fields else None),
    ]

    content_types = ['<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>']
    for index in range(1, len(sheets) + 1):
        content_types.append(f'<Override PartName="/xl/worksheets/sheet{index}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>')
    content_types.append('</Types>')
    workbook_sheets = ''.join(f'<sheet name="{escape(name)}" sheetId="{i}" r:id="rId{i}"/>' for i, (name, *_rest) in enumerate(sheets, 1))
    workbook = f'<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>{workbook_sheets}</sheets></workbook>'
    workbook_rels = ['<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">']
    for index in range(1, len(sheets) + 1):
        workbook_rels.append(f'<Relationship Id="rId{index}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet{index}.xml"/>')
    workbook_rels.append(f'<Relationship Id="rId{len(sheets)+1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>')
    styles = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1F4E78"/><bgColor indexed="64"/></patternFill></fill><fill><patternFill patternType="solid"><fgColor rgb="FFC6EFCE"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="4"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFill="1" applyFont="1"/><xf numFmtId="0" fontId="0" fillId="3" borderId="0" xfId="0" applyFill="1"/><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"><font><color rgb="FF9C0006"/></font></xf></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>'''
    root_rels = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>'

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(output_path, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("[Content_Types].xml", "".join(content_types))
        archive.writestr("_rels/.rels", root_rels)
        archive.writestr("xl/workbook.xml", workbook)
        archive.writestr("xl/_rels/workbook.xml.rels", "".join(workbook_rels))
        archive.writestr("xl/styles.xml", styles)
        for index, (_name, rows, widths, freeze, autofilter, status_col) in enumerate(sheets, 1):
            archive.writestr(f"xl/worksheets/sheet{index}.xml", worksheet(rows, widths, freeze, autofilter, status_col))

    print(f"Created {output_path} from {len(records)} real CSV rows ({len(scenario_records)} scenario-labelled rows).")


if __name__ == "__main__":
    base = Path(__file__).resolve().parents[1] / "store-forward" / "results"
    source = Path(sys.argv[1]) if len(sys.argv) > 1 else base / "persistent_store_forward_results.csv"
    output = Path(sys.argv[2]) if len(sys.argv) > 2 else base / "store_forward_professor_results.xlsx"
    create_workbook(source, output)

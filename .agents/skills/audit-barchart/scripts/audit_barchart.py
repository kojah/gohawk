#!/usr/bin/env python3
"""Chart reviewed original audit findings without counting follow-up replays."""

import argparse
from collections import Counter, defaultdict
import csv
from dataclasses import dataclass, field
from html import escape
import json
from pathlib import Path
import re


ROOT = Path(__file__).resolve().parents[4]
AUDITS = ROOT / "benchmarks/precision/audits"
CATALOG = ROOT / "site/src/generated/analyzers.json"
COLORS = {"true-positive": "#31a86b", "false-positive": "#e26868", "inconclusive": "#a4a8b3"}
VERDICTS = tuple(COLORS)
BATCH_NAME = re.compile(r"batch-(\d+)-findings\.tsv\Z")
DOGFOOD_NAME = re.compile(r"[a-z0-9-]+-dogfood-\d{4}-\d{2}-\d{2}\.tsv\Z")


@dataclass
class Totals:
    counts: Counter = field(default_factory=Counter)

    def add(self, verdict):
        self.counts[verdict] += 1

    def get(self, verdict):
        return self.counts[verdict]

    def reviewed(self):
        return self.get("true-positive") + self.get("false-positive")

    def all(self):
        return sum(self.get(verdict) for verdict in VERDICTS)

    def as_dict(self):
        return {verdict: self.get(verdict) for verdict in VERDICTS}


def selected_sources(audit_dir, last_batches, batches, include_dogfood):
    available = {}
    for path in audit_dir.glob("batch-*-findings.tsv"):
        match = BATCH_NAME.fullmatch(path.name)
        if match:
            available[int(match.group(1))] = path
    numbers = sorted(set(batches)) if batches else sorted(available)[-last_batches:]
    if not numbers:
        raise ValueError(f"no numbered batch finding ledgers in {audit_dir}")
    missing = [number for number in numbers if number not in available]
    if missing:
        raise ValueError(f"missing reviewed finding ledger for batch(es): {missing}")
    sources = [available[number] for number in numbers]
    if include_dogfood:
        sources.extend(path for path in sorted(audit_dir.glob("*-dogfood-*.tsv")) if DOGFOOD_NAME.fullmatch(path.name))
    return sources


def read_findings(path):
    with path.open(newline="", encoding="utf-8") as stream:
        heading = stream.readline()
        if not heading.startswith("# "):
            raise ValueError(f"{path}: missing commented TSV header")
        columns = heading[2:].rstrip("\r\n").split("\t")
        required = {"repository", "revision", "analyzer", "position", "checks", "verdict"}
        if not required.issubset(columns) or len(columns) != len(set(columns)):
            raise ValueError(f"{path}: unsupported finding columns")
        for line, row in enumerate(csv.DictReader(stream, fieldnames=columns, delimiter="\t"), 2):
            if None in row or row["verdict"] not in VERDICTS or not row["analyzer"] or not row["checks"]:
                raise ValueError(f"{path}:{line}: invalid reviewed finding")
            checks = [check.strip() for check in re.split(r"[,;]", row["checks"]) if check.strip()]
            if not checks or len(checks) != len(set(checks)) or any(not check.startswith(row["analyzer"] + "/") for check in checks):
                raise ValueError(f"{path}:{line}: ambiguous check attribution")
            yield row, checks


def count_findings(sources):
    by_analyzer = defaultdict(Totals)
    by_check = defaultdict(Totals)
    seen = set()
    multiple_checks = 0
    for path in sources:
        for row, checks in read_findings(path):
            identity = (row["repository"].casefold(), row["revision"], row["analyzer"], row["position"], tuple(checks))
            if identity in seen:
                raise ValueError(f"{path}: duplicate finding across selected audits: {identity}")
            seen.add(identity)
            by_analyzer[row["analyzer"]].add(row["verdict"])
            for check in checks:
                by_check[check].add(row["verdict"])
            multiple_checks += len(checks) > 1
    return by_analyzer, by_check, multiple_checks


def add_catalog(catalog_path, by_analyzer, by_check):
    catalog = json.loads(catalog_path.read_text(encoding="utf-8"))
    current_analyzers, current_checks = set(), set()
    for group in catalog["groups"]:
        for analyzer in group["analyzers"]:
            name = analyzer["name"]
            if name in current_analyzers:
                raise ValueError(f"{catalog_path}: duplicate analyzer {name}")
            current_analyzers.add(name)
            by_analyzer.setdefault(name, Totals())
            for check in analyzer["checks"]:
                identifier = check["id"]
                if identifier in current_checks or not identifier.startswith(name + "/"):
                    raise ValueError(f"{catalog_path}: invalid check {identifier}")
                current_checks.add(identifier)
                by_check.setdefault(identifier, Totals())
    return sorted(set(by_analyzer) - current_analyzers), sorted(set(by_check) - current_checks)


def chart(title, counts, subtitle):
    rows = sorted(counts.items(), key=lambda item: (-item[1].all(), -item[1].get("true-positive"), item[0]))
    if not rows:
        raise ValueError("selected audits contain no reviewed findings")
    width, left, right, top, row_height = 1180, 330, 230, 118, 34
    bar_width = width - left - right
    height = top + len(rows) * row_height + 40
    maximum = max(total.all() for _, total in rows)
    if maximum == 0:
        raise ValueError("selected audits contain no reviewed findings")
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}" role="img" aria-label="{escape(title, quote=True)}">',
             '<rect width="100%" height="100%" fill="#171b22"/>',
             f'<text x="24" y="33" fill="#f5f5f5" font-size="22" font-family="sans-serif">{escape(title)}</text>',
             f'<text x="24" y="58" fill="#bec5ce" font-size="13" font-family="sans-serif">{escape(subtitle)}</text>']
    legend_x = 25
    for verdict, label in (("true-positive", "TP"), ("false-positive", "FP"), ("inconclusive", "?")):
        parts.append(f'<rect x="{legend_x}" y="73" width="12" height="12" fill="{COLORS[verdict]}"/>')
        parts.append(f'<text x="{legend_x + 18}" y="84" fill="#dce1e7" font-size="12" font-family="sans-serif">{label}</text>')
        legend_x += 68
    parts.append('<text x="950" y="84" fill="#bec5ce" font-size="12" font-family="sans-serif">TP / FP / ?</text>')
    for index, (name, total) in enumerate(rows):
        y = top + index * row_height
        if index % 2 == 0:
            parts.append(f'<rect x="12" y="{y - 21}" width="{width - 24}" height="30" fill="#20262f"/>')
        parts.append(f'<text x="24" y="{y}" fill="#e7eaf0" font-size="13" font-family="monospace">{escape(name)}</text>')
        x = left
        for verdict in VERDICTS:
            amount = total.get(verdict)
            segment = amount / maximum * bar_width
            if amount:
                parts.append(f'<rect x="{x:.2f}" y="{y - 14}" width="{segment:.2f}" height="17" fill="{COLORS[verdict]}"><title>{escape(name)}: {amount} {verdict}</title></rect>')
            x += segment
        parts.append(f'<text x="950" y="{y}" fill="#e7eaf0" font-size="13" font-family="monospace">{total.get("true-positive")} / {total.get("false-positive")} / {total.get("inconclusive")}</text>')
    parts.append('</svg>')
    return "\n".join(parts) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--audit-dir", type=Path, default=AUDITS, help="directory of reviewed finding ledgers")
    parser.add_argument("--catalog", type=Path, default=CATALOG, help="current generated analyzer catalog for zero rows")
    parser.add_argument("--output-dir", type=Path, default=ROOT / ".build/audit-barchart")
    parser.add_argument("--last-batches", type=int, default=5)
    parser.add_argument("--batch", type=int, action="append", default=[])
    parser.add_argument("--include-dogfood", action="store_true")
    args = parser.parse_args()
    if args.last_batches < 1 or any(number < 1 for number in args.batch):
        parser.error("batch numbers and --last-batches must be positive")
    try:
        sources = selected_sources(args.audit_dir, args.last_batches, args.batch, args.include_dogfood)
        analyzers, checks, multiple = count_findings(sources)
        if not analyzers:
            raise ValueError("selected audits contain no reviewed findings")
        unlisted_analyzers, unlisted_checks = add_catalog(args.catalog, analyzers, checks)
    except ValueError as error:
        parser.error(str(error))
    batch_numbers = [BATCH_NAME.fullmatch(path.name).group(1) for path in sources if BATCH_NAME.fullmatch(path.name)]
    dogfood_count = len(sources) - len(batch_numbers)
    subtitle = "Original reviewed findings · batches " + ", ".join(batch_numbers)
    if dogfood_count:
        subtitle += f" · {dogfood_count} dogfood audits"
    args.output_dir.mkdir(parents=True, exist_ok=True)
    for name, rows in (("analyzers", analyzers), ("checks", checks)):
        (args.output_dir / f"{name}.svg").write_text(chart(f"Audit findings by {name}", rows, subtitle), encoding="utf-8")
    totals = Totals()
    for row in analyzers.values():
        for verdict in VERDICTS:
            totals.counts[verdict] += row.get(verdict)
    summary = {
        "sources": [str(path) for path in sources],
        "findings": totals.as_dict(),
        "multiple_check_findings": multiple,
        "catalog": str(args.catalog),
        "historical_analyzers_not_in_catalog": unlisted_analyzers,
        "historical_checks_not_in_catalog": unlisted_checks,
        "analyzers": {name: total.as_dict() for name, total in sorted(analyzers.items())},
        "checks": {name: total.as_dict() for name, total in sorted(checks.items())},
        "meaning": "Historical reviewed findings from original scans; not current-run precision or recall.",
    }
    (args.output_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(f"Sources: {', '.join(path.name for path in sources)}")
    print(f"Findings: {totals.get('true-positive')} TP, {totals.get('false-positive')} FP, {totals.get('inconclusive')} inconclusive")
    print(f"Multiple-check findings: {multiple}")
    print(f"Charts: {args.output_dir / 'analyzers.svg'}, {args.output_dir / 'checks.svg'}")


if __name__ == "__main__":
    main()

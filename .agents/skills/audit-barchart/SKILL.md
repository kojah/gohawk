---
name: audit-barchart
description: Chart reviewed gohawk audit findings by analyzer and check, showing true positives, false positives, and inconclusive findings from recent batches.
metadata:
    source: project
---

# Audit bar charts

Run the bundled chart script from the gohawk repository root:

```sh
python3 -B .agents/skills/audit-barchart/scripts/audit_barchart.py
```

It reads the reviewed `batch-N-findings.tsv` ledgers, selects the five highest
batch numbers by default, and writes `analyzers.svg`, `checks.svg`, and
`summary.json` under `.build/audit-barchart/`. Use `--last-batches N` or
repeated `--batch N` to change the window. Add `--include-dogfood` to include
dated, reviewed dogfood ledgers, or `--output-dir PATH` to place the artifacts
elsewhere. The command prints its sources, totals, and output paths.

Present both charts and identify the source batches. Say that each bar counts
reviewed findings from the original scans: TP means a reviewed true positive,
FP a reviewed false positive, and `?` an inconclusive review. A zero means
none was recorded in this window for that catalog check; it does not measure
recall or establish that candidates were seen. Historical FPs
may already be fixed, and findings from different scan baselines should not be
read as a current-run precision measurement. Reviewed TPs can include bounded
hazards or policy-only findings; they are not all reproduced runtime failures.
Follow-up correction ledgers and
regression labels are excluded so the same finding is not counted twice.

For a quick comparison, mention the checks with the most TPs and FPs, and
the review volume behind any apparent precision ratio. Do not treat
inconclusive findings as either TP or FP. If a finding lists multiple checks,
the analyzer chart counts it once and each named check receives one count;
the script reports that duplication in its summary.

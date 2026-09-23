---
name: audit-barchart
description: Render reviewed gohawk audit findings inline by analyzer and check, showing true positives, false positives, and inconclusive findings from recent batches.
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

Show both analyzer and check charts, with consistent TP, FP, and inconclusive
colors, readable labels, and the source batch numbers. In Codex Desktop, use
the `visualize` skill to render `summary.json` as an inline chart widget; follow
its file, validation, and response contract. Do not print its control syntax
as ordinary text. In Claude Code, including its desktop app, use the generated
SVGs as inline images when supported. Otherwise link both SVGs so they can be
opened in the desktop Browser pane. Do not emit Codex-specific control syntax
there.

Explain in the visualization or a brief accompanying note that each bar counts
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

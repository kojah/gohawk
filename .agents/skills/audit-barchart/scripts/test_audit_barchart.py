"""Behavior checks for audit selection, counting, and safe SVG output."""

import importlib.util
import json
from pathlib import Path
import sys
from tempfile import TemporaryDirectory
import unittest
from xml.etree import ElementTree


SCRIPT = Path(__file__).with_name("audit_barchart.py")
SPEC = importlib.util.spec_from_file_location("audit_barchart", SCRIPT)
CHART = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = CHART
SPEC.loader.exec_module(CHART)


HEADER = "# repository\trevision\tanalyzer\tposition\tchecks\tverdict\treason\n"


class AuditBarChartTests(unittest.TestCase):
    def test_latest_batches_and_distinct_verdicts(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "batch-9-findings.tsv").write_text(HEADER + "old/x\ta\tone\ta.go:1\tone/old\ttrue-positive\treview\n")
            (root / "batch-10-findings.tsv").write_text(
                HEADER
                + "new/x\tb\tone\ta.go:1\tone/good\ttrue-positive\treview\n"
                + "new/x\tb\tone\ta.go:2\tone/good\tfalse-positive\treview\n"
                + "new/x\tb\ttwo\tb.go:1\ttwo/a,two/b\tinconclusive\treview\n"
            )
            (root / "batch-10-corrections.tsv").write_text("not an original audit\n")
            sources = CHART.selected_sources(root, 1, [], False)
            self.assertEqual([path.name for path in sources], ["batch-10-findings.tsv"])
            analyzers, checks, multiple = CHART.count_findings(sources)
            self.assertEqual(analyzers["one"].as_dict(), {
                "true-positive": 1, "false-positive": 1, "inconclusive": 0,
            })
            self.assertEqual(analyzers["two"].all(), 1)
            self.assertEqual(checks["two/a"].get("inconclusive"), 1)
            self.assertEqual(checks["two/b"].get("inconclusive"), 1)
            self.assertEqual(multiple, 1)
            catalog = root / "analyzers.json"
            catalog.write_text(json.dumps({"groups": [{"analyzers": [
                {"name": "one", "checks": [{"id": "one/good"}, {"id": "one/quiet"}]},
                {"name": "three", "checks": [{"id": "three/quiet"}]},
            ]}]}))
            unlisted_analyzers, unlisted_checks = CHART.add_catalog(catalog, analyzers, checks)
            self.assertEqual(analyzers["three"].all(), 0)
            self.assertEqual(checks["one/quiet"].all(), 0)
            self.assertEqual(unlisted_analyzers, ["two"])
            self.assertEqual(unlisted_checks, ["two/a", "two/b"])

    def test_rejects_unreviewed_and_duplicate_rows(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "batch-1-findings.tsv"
            finding = "repo/x\ta\tone\ta.go:1\tone/check\ttrue-positive\treview\n"
            path.write_text(HEADER + finding + finding)
            with self.assertRaisesRegex(ValueError, "duplicate finding"):
                CHART.count_findings([path])
            path.write_text(HEADER + finding.replace("true-positive", "unreviewed"))
            with self.assertRaisesRegex(ValueError, "invalid reviewed finding"):
                CHART.count_findings([path])

    def test_svg_escapes_reviewed_names(self):
        totals = CHART.Totals()
        totals.add("true-positive")
        svg = CHART.chart("Test", {"check/<unsafe>&": totals}, "window")
        ElementTree.fromstring(svg)
        self.assertIn("check/&lt;unsafe&gt;&amp;", svg)
        self.assertNotIn("<unsafe>", svg)


if __name__ == "__main__":
    unittest.main()

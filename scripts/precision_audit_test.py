import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("precision_audit", Path(__file__).with_name("precision-audit.py"))
AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUDIT)
SHA = "a" * 40


class PrecisionAuditTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.manifest = self.root / "repos.tsv"

    def test_manifest_requires_pinned_unique_safe_repositories(self):
        self.manifest.write_text(f"owner/repo\t{SHA}\n")
        self.assertEqual(AUDIT.read_manifest(self.manifest), [("owner/repo", SHA)])
        for text in ["owner/repo\tmain\n", f"../repo\t{SHA}\n", f"owner/repo\t{SHA}\nOwner/Repo\t{SHA}\n"]:
            self.manifest.write_text(text)
            with self.assertRaises(ValueError):
                AUDIT.read_manifest(self.manifest)

    def test_history_supports_cohorts_and_selection_ledgers(self):
        self.manifest.write_text(f"Owner/Repo\t{SHA}\n48\tOther/Repo\t{SHA}\t.\tscanned\n")
        self.assertEqual(AUDIT.read_history([self.manifest]), {"owner/repo", "other/repo"})

    def test_report_is_incremental_and_resumable(self):
        entry = ("owner/repo", SHA)
        finding = ("owner/repo", "lockorder", "main.go:3:1")
        with patch.object(AUDIT.REPLAY, "checkout_repository", return_value=self.root), \
             patch.object(AUDIT.REPLAY, "module_directories", return_value=[self.root]), \
             patch.object(AUDIT.REPLAY, "scan", return_value=({finding}, {finding: {"lockorder/example"}}, [])) as scan:
            report = AUDIT.analyze(entry, Path("binary"), self.root, self.root)
            self.assertEqual(report["scan_status"], "scanned")
            self.assertEqual(report["review_status"], "unreviewed")
            self.assertEqual(AUDIT.analyze(entry, Path("binary"), self.root, self.root), report)
            scan.assert_called_once()

    def test_checkout_failure_is_not_a_clean_scan(self):
        with patch.object(AUDIT.REPLAY, "checkout_repository", side_effect=SystemExit("fetch failed")):
            report = AUDIT.analyze(("owner/repo", SHA), Path("binary"), self.root, self.root)
        self.assertEqual(report["scan_status"], "failed")
        self.assertEqual(json.loads((self.root / "owner__repo.json").read_text()), report)

    def test_partial_findings_preserved_without_claiming_clean(self):
        with patch.object(AUDIT.REPLAY, "checkout_repository", return_value=self.root), \
             patch.object(AUDIT.REPLAY, "module_directories", return_value=[self.root]), \
             patch.object(AUDIT.REPLAY, "scan", return_value=(set(), {}, ["timed out in ."])):
            report = AUDIT.analyze(("owner/repo", SHA), Path("binary"), self.root, self.root)
        self.assertEqual(report["scan_status"], "incomplete")


if __name__ == "__main__":
    unittest.main()

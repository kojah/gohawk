import csv
import importlib.util
import json
import os
from pathlib import Path
import signal
import sys
import tempfile
import subprocess
import threading
import time
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

    def test_history_supports_reviewed_findings(self):
        self.manifest.write_text(f"Reviewed/Repo\t{SHA}\tlockorder\tmain.go:3:1\tlockorder/example\tfalse-positive\treason\n")
        self.assertEqual(AUDIT.read_history([self.manifest]), {"reviewed/repo"})

    def test_history_rejects_unknown_formats(self):
        self.manifest.write_text("owner/repo\tmain\tunexpected\n")
        with self.assertRaises(ValueError):
            AUDIT.read_history([self.manifest])

    def test_history_supports_followup_reviews(self):
        self.manifest.write_text(
            "repository\trevision\tanalyzer\tposition\tcheck\tevidence_family\tstatus\tsource_review\n"
            f"Reviewed/Repo\t{SHA}\tlockorder\tmain.go:3:1\tlockorder/example\tcleanup\tfixed\treason\n"
        )
        self.assertEqual(AUDIT.read_history([self.manifest]), {"reviewed/repo"})

    def test_default_history_ignores_nonselection_audit_ledgers(self):
        audit_dir = self.root / "benchmarks/precision/audits"
        audit_dir.mkdir(parents=True)
        cohort = self.root / "benchmarks/precision/round-58/repositories.tsv"
        cohort.parent.mkdir(parents=True)
        cohort.write_text(f"cohort/repo\t{SHA}\n")
        (audit_dir / "500-repository.tsv").write_text(f"old/repo\t{SHA}\n")
        (audit_dir / "batch-55.tsv").write_text(f"55\tbatch/repo\t{SHA}\t.\tscanned\n")
        (audit_dir / "followup207-locks.tsv").write_text("unrelated\tledger\twith\ta\tdifferent\tformat\n")
        (audit_dir / "batch-55-findings.tsv").write_text("another\tledger\twith\ta\tdifferent\tformat\n")

        history = AUDIT.read_history(AUDIT.default_history_paths(self.root))
        self.assertEqual(history, {"cohort/repo", "old/repo", "batch/repo"})

    @unittest.skipUnless(os.name == "posix" and Path("/proc").is_dir(), "requires POSIX process inspection")
    def test_timed_out_command_terminates_child_analyzer(self):
        child_pid_file = self.root / "child.pid"
        command = [
            sys.executable,
            "-c",
            "import pathlib, subprocess, sys; "
            "child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)']); "
            "pathlib.Path(sys.argv[1]).write_text(str(child.pid)); child.wait()",
            str(child_pid_file),
        ]
        try:
            with self.assertRaises(subprocess.TimeoutExpired):
                AUDIT.run_scoped(command, capture_output=True, timeout=0.5)
            child_pid = int(child_pid_file.read_text())
            for _ in range(20):
                status = Path(f"/proc/{child_pid}/stat")
                if not status.exists() or status.read_text().split()[2] == "Z":
                    break
                time.sleep(0.05)
            else:
                self.fail("timed-out command left a running child")
        finally:
            if child_pid_file.exists():
                child_pid = int(child_pid_file.read_text())
                status = Path(f"/proc/{child_pid}/stat")
                if status.exists() and status.read_text().split()[2] != "Z":
                    os.kill(child_pid, signal.SIGKILL)

    def test_scoped_command_preserves_captured_output(self):
        result = AUDIT.run_scoped([sys.executable, "-c", "print('ok')"], capture_output=True)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "ok\n")
        self.assertEqual(result.stderr, "")

    @unittest.skipUnless(os.name == "posix", "requires a local go command stub")
    def test_go_command_owns_and_cleans_its_temp_directory(self):
        go = self.root / "go"
        go.write_text(
            "#!/bin/sh\n"
            "printf '%s\\n' \"$GOTMPDIR\" > \"$1\"\n"
            "mkdir \"$GOTMPDIR/work\"\n"
            "if [ \"$2\" = wait ]; then sleep 60; fi\n"
        )
        go.chmod(0o755)
        environment = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ["PATH"])
        success_path = self.root / "success.tmpdir"
        result = AUDIT.run_scoped(["go", str(success_path)], env=environment, capture_output=True)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        owned = Path(success_path.read_text().strip())
        self.assertEqual(owned.parent, Path("/tmp"))
        self.assertFalse(owned.exists())

        timeout_path = self.root / "timeout.tmpdir"
        with self.assertRaises(subprocess.TimeoutExpired):
            AUDIT.run_scoped(["go", str(timeout_path), "wait"], env=environment, timeout=0.5)
        self.assertFalse(Path(timeout_path.read_text().strip()).exists())
        self.assertNotIn("GOTMPDIR", environment)

    def test_runner_upgrade_requires_exact_prior_hash_and_records_saved_reports(self):
        current = {
            "repositories": [["owner/repo", SHA], ["other/repo", SHA]],
            "binary_sha256": "binary", "runner_sha256": "new", "replay_sha256": "replay",
            "go_version": "go version", "profile": "-enable-all -gohawk-include-tests -json",
        }
        previous = dict(current, runner_sha256="old")
        (self.root / "owner__repo.json").write_text(json.dumps({"repository": "owner/repo", "revision": SHA}))
        with self.assertRaisesRegex(ValueError, "runner changed"):
            AUDIT.resume_metadata(previous, current.copy(), None, self.root)
        upgraded = AUDIT.resume_metadata(previous, current.copy(), "old", self.root)
        self.assertEqual(upgraded["runner_history"], [
            {"runner_sha256": "old", "completed_repositories": ["owner/repo"]}
        ])
        self.assertEqual(AUDIT.resume_metadata(upgraded, current.copy(), None, self.root), upgraded)
        with self.assertRaisesRegex(ValueError, "different profile"):
            AUDIT.resume_metadata(previous, dict(current, profile="changed"), "old", self.root)

    def test_bounded_scans_stop_scheduling_after_report_failure(self):
        started = []
        hold_second = threading.Event()

        def scan(entry):
            started.append(entry)
            if entry == 0:
                raise OSError("no space left on device")
            hold_second.wait(0.1)
            return entry

        with self.assertRaises(OSError):
            AUDIT.analyze_bounded(list(range(100)), 2, scan)
        self.assertEqual(sorted(started), [0, 1])
        self.assertEqual(AUDIT.analyze_bounded([0, 1, 2], 2, lambda entry: entry * 2), [0, 2, 4])

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

    def test_analyzer_error_object_is_incomplete(self):
        payload = {"example/pkg": {
            "broken": {"error": "analysis failed"},
            "lockorder": [{"posn": str(self.root / "main.go") + ":3:1",
                           "category": "lockorder/missing-release"}],
        }}
        result = subprocess.CompletedProcess([], 0, json.dumps(payload), "")
        with patch.object(AUDIT.REPLAY, "module_directories", return_value=[self.root]), \
             patch.object(AUDIT.REPLAY, "run", return_value=result):
            findings, _, errors = AUDIT.REPLAY.scan(Path("binary"), "owner/repo", self.root)
        self.assertEqual(len(findings), 1)
        self.assertEqual(errors, ["1 package error(s) in ."])

    def test_partial_package_retry_remains_incomplete(self):
        failed = subprocess.CompletedProcess([], 1, "", "missing dependency")
        recovered = subprocess.CompletedProcess([], 0, "{}", "")
        with patch.object(AUDIT.REPLAY, "module_directories", return_value=[self.root]), \
             patch.object(AUDIT.REPLAY, "run", return_value=failed), \
             patch.object(AUDIT.REPLAY, "loadable_packages", return_value=["example/good"]), \
             patch.object(AUDIT.REPLAY, "retry_scan", return_value=recovered):
            _, _, errors = AUDIT.REPLAY.scan(Path("binary"), "owner/repo", self.root)
        self.assertEqual(errors, ["partial package recovery in .: missing dependency"])

    def test_nonzero_analysis_with_json_remains_incomplete(self):
        result = subprocess.CompletedProcess([], 1, "{}", "failed package")
        with patch.object(AUDIT.REPLAY, "module_directories", return_value=[self.root]), \
             patch.object(AUDIT.REPLAY, "run", return_value=result):
            _, _, errors = AUDIT.REPLAY.scan(Path("binary"), "owner/repo", self.root)
        self.assertEqual(errors, ["analysis command failed in . (exit 1): failed package"])

    def test_regression_stamp_requires_scannable_false_positive(self):
        (self.root / "repositories.tsv").write_text(f"owner/repo\t{SHA}\n")
        labels = self.root / "labels.csv"
        header = "repository,analyzer,check,position,verdict,gohawk_revision,confirmed_at\n"
        row = "owner/repo,lockorder,lockorder/example,main.go:3:1,false_positive,old,2026-01-01\n"
        for errors, expected_revision in [(["package failed"], "old"), ([], "new")]:
            with self.subTest(errors=errors):
                labels.write_text(header + row)
                arguments = ["precision-regression", str(self.root), "--gohawk", "binary", "--stamp"]
                with patch("sys.argv", arguments), \
                     patch.object(AUDIT.REPLAY, "checkout_repository", return_value=self.root), \
                     patch.object(AUDIT.REPLAY, "current_revision", return_value="new"), \
                     patch.object(AUDIT.REPLAY, "scan", return_value=(set(), {}, errors)):
                    AUDIT.REPLAY.main()
                with labels.open(newline="") as source:
                    stamped = next(csv.DictReader(source))
                self.assertEqual(stamped["gohawk_revision"], expected_revision)


if __name__ == "__main__":
    unittest.main()

"""Check Make's instrumentation guard without running instrumented Go builds."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


MAKEFILE = Path(__file__).resolve().parents[1] / "Makefile"


class CIPolicyTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        (self.root / "Makefile").write_text(MAKEFILE.read_text())
        self.marker = self.root / "go-invoked"
        fake_go = self.root / "fake-go"
        fake_go.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> go-invoked\n')
        fake_go.chmod(0o700)
        self.environment = os.environ.copy()
        for name in ("CI", "GITHUB_ACTIONS", "MAKEFLAGS", "MFLAGS", "MAKELEVEL", "MAKEOVERRIDES", "GOFLAGS"):
            self.environment.pop(name, None)

    def run_make(self, *arguments, markers=None):
        return subprocess.run(
            ["make", "--no-print-directory", "GO=./fake-go", *arguments],
            cwd=self.root,
            env={**self.environment, **(markers or {})},
            text=True,
            capture_output=True,
            timeout=10,
        )

    def test_local_instrumentation_rejected_before_any_work(self):
        for target in ("coverage", "test-race", "ci", "verify-timed-coverage", "verify-timed-test-race", "verify-timed-ci"):
            for markers in ({}, {"CI": "true"}, {"GITHUB_ACTIONS": "true"}, {"CI": "false", "GITHUB_ACTIONS": "true"}):
                with self.subTest(target=target, markers=markers):
                    result = self.run_make("-j4", "build", target, markers=markers)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertIn("CI-only", result.stderr)
                    self.assertFalse(self.marker.exists(), result.stdout)
                    self.assertFalse((self.root / ".build").exists())

    def test_local_instrumentation_flags_are_rejected(self):
        for flag in ("-race", "-cover", "-covermode=count", "-coverpkg=./...", "-coverprofile=coverage.out"):
            with self.subTest(flag=flag):
                result = self.run_make("test", "GOFLAGS=" + flag)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("CI-only", result.stderr)
                self.assertFalse(self.marker.exists())

    def test_github_actions_can_plan_instrumented_targets(self):
        # Dry runs verify CI wiring; no race or coverage test is executed here.
        for target, expected in (("coverage", "-coverprofile=coverage.out"), ("test-race", "test -race"), ("ci", "test -race")):
            with self.subTest(target=target):
                result = self.run_make("-n", target, markers={"CI": "true", "GITHUB_ACTIONS": "true"})
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(expected, result.stdout)
                self.assertFalse(self.marker.exists())

    def test_ordinary_local_tests_remain_available(self):
        result = self.run_make("test")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.read_text(), "test ./...\n")


if __name__ == "__main__":
    unittest.main()

import importlib.util
import sys
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("analyze_swe_rebench_go.py")
SPEC = importlib.util.spec_from_file_location("analyze_swe_rebench_go", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class AnalyzeSWERebenchGoTest(unittest.TestCase):
    def task(self, *, added: str = "", removed: str = "") -> MODULE.Task:
        return MODULE.Task(
            instance_id="owner__repo-1",
            repository="owner/repo",
            title="example",
            categories=frozenset({"minor_bug"}),
            quality="A",
            added=added,
            removed=removed,
        )

    def test_introduced_requires_a_new_match(self) -> None:
        matcher = MODULE.introduced(r"errors\.Is\(")
        self.assertTrue(matcher(self.task(added="errors.Is(err, target)")))
        self.assertFalse(
            matcher(
                self.task(
                    added="errors.Is(err, target)", removed="errors.Is(old, target)"
                )
            )
        )

    def test_context_guarded_send_requires_context_case(self) -> None:
        signal = next(
            item
            for item in MODULE.repair_signals()
            if item.name == "context-guarded channel send"
        )
        self.assertTrue(
            signal.matcher(
                self.task(
                    removed="commands <- command",
                    added="select {\ncase <-ctx.Done():\ncase commands <- command:\n}",
                )
            )
        )
        self.assertFalse(
            signal.matcher(
                self.task(
                    removed="commands <- command",
                    added="select {\ncase commands <- command:\n}",
                )
            )
        )

    def test_report_filters_features_and_ambiguous_tasks(self) -> None:
        tasks = [
            self.task(added="if err != nil { return err }"),
            MODULE.Task(
                "feature",
                "owner/feature",
                "feature",
                frozenset({"core_feat"}),
                "A",
                "if err != nil {}",
                "",
            ),
            MODULE.Task(
                "ambiguous",
                "owner/bug",
                "bug",
                frozenset({"major_bug"}),
                "B4",
                "if err != nil {}",
                "",
            ),
        ]
        report = MODULE.markdown_report(tasks, 0)
        self.assertIn("Bug-labelled tasks: **2**", report)
        self.assertIn(
            "Quality-A bug tasks used for repair-signal counts: **1**", report
        )
        self.assertIn("| error guard introduced | 1 | 1 |", report)


if __name__ == "__main__":
    unittest.main()

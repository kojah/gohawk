#!/usr/bin/env python3
"""Summarize analyzable repair patterns in SWE-rebench V2 Go tasks."""

from __future__ import annotations

import argparse
import re
from collections import Counter
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

BUG_CATEGORIES = frozenset(
    {
        "core_bug",
        "critical_bug",
        "edge_case_bug",
        "major_bug",
        "minor_bug",
        "performance_bug",
        "regression_bug",
        "security_bug",
    }
)


@dataclass(frozen=True)
class Task:
    instance_id: str
    repository: str
    title: str
    categories: frozenset[str]
    quality: str
    added: str
    removed: str


Matcher = Callable[[Task], bool]


@dataclass(frozen=True)
class RepairSignal:
    name: str
    description: str
    matcher: Matcher
    existing_coverage: str
    disposition: str


def introduced(pattern: str) -> Matcher:
    expression = re.compile(pattern, re.MULTILINE)
    return lambda task: (
        bool(expression.search(task.added)) and not expression.search(task.removed)
    )


def transformed(removed: str, added: str) -> Matcher:
    removed_expression = re.compile(removed, re.MULTILINE)
    added_expression = re.compile(added, re.MULTILINE)
    return lambda task: bool(
        removed_expression.search(task.removed) and added_expression.search(task.added)
    )


def all_matchers(*matchers: Matcher) -> Matcher:
    return lambda task: all(matcher(task) for matcher in matchers)


def repair_signals() -> tuple[RepairSignal, ...]:
    return (
        RepairSignal(
            "nil guard introduced",
            "A fix adds a nil comparison that was absent from removed lines.",
            introduced(r"\bif\s+[^\n{;]+(?:==|!=)\s*nil\b"),
            "Go nilness, Staticcheck SA5011, nilerr, and nilnesserr cover provable variants.",
            "Do not add a broad check; most remaining cases require an API or domain precondition.",
        ),
        RepairSignal(
            "error guard introduced",
            "A fix adds an `err != nil` branch that was absent from removed lines.",
            introduced(r"\bif\s+err\s*!=\s*nil\b"),
            "errcheck, nilerr, nilnesserr, go vet, and Staticcheck cover common static mistakes.",
            "Do not duplicate; inspect only narrower recurring contracts.",
        ),
        RepairSignal(
            "error chain operation introduced",
            "A fix starts using errors.Is or errors.As.",
            introduced(r"\berrors\.(?:Is|As)\s*\("),
            "errorlint checks direct comparisons and assertions involving wrapped errors.",
            "Covered externally.",
        ),
        RepairSignal(
            "error wrapping introduced",
            "A fix introduces a `%w` error wrap.",
            introduced(r"%w"),
            "errorlint and wrapcheck cover the relevant error-chain policies.",
            "Covered externally.",
        ),
        RepairSignal(
            "resource close introduced",
            "A fix adds a Close call for an acquired value.",
            introduced(r"\b(?:defer\s+)?[^\n]+\.Close\s*\("),
            "gohawk resourcelifetime, bodyclose, and sqlclosecheck cover well-known contracts.",
            "Extend an exact resource contract only when a repeated API family is found.",
        ),
        RepairSignal(
            "cancellation release introduced",
            "A fix adds a call to a cancel function.",
            introduced(r"\b(?:defer\s+)?cancel\w*\s*\("),
            "go vet lostcancel and gohawk cancellationownership cover derived contexts.",
            "Covered.",
        ),
        RepairSignal(
            "lock operation introduced",
            "A fix adds a mutex lock or unlock operation.",
            introduced(r"\.(?:R?Lock|R?Unlock)\s*\("),
            "The race detector, go vet copylocks, and gohawk lockorder cover important subsets.",
            "The signal is too broad; retain only independently provable lock protocols.",
        ),
        RepairSignal(
            "defensive copy introduced",
            "A fix introduces an explicit copy or Clone call.",
            introduced(
                r"\b(?:copy|slices\.Clone|maps\.Clone|bytes\.Clone|io\.Copy)\s*\("
            ),
            "No mainstream analyzer proves general borrowed-versus-owned slice or buffer contracts.",
            "Research target; deeper alias and ownership modeling is required before diagnostics.",
        ),
        RepairSignal(
            "deterministic sort introduced",
            "A fix introduces sorting that was absent from removed lines.",
            introduced(r"\b(?:sort\.|slices\.Sort)"),
            "gohawk determinism covers map iteration that reaches ordered output.",
            "Covered for the high-confidence output-order contract.",
        ),
        RepairSignal(
            "timer or ticker stop introduced",
            "A fix adds a Stop call.",
            introduced(r"\b(?:defer\s+)?[^\n]+\.Stop\s*\(\s*\)"),
            "gohawk resourcelifetime models time.NewTimer and time.NewTicker; Staticcheck covers time.Tick leaks.",
            "Covered.",
        ),
        RepairSignal(
            "terminal iterator error check introduced",
            "A fix adds an Err call after iteration.",
            introduced(r"\.Err\s*\(\s*\)"),
            "rowserrcheck covers database rows; errcheck and API-specific tools cover other iterators.",
            "Prefer an exact API contract over a name-based check.",
        ),
        RepairSignal(
            "map allocation introduced",
            "A fix explicitly allocates a map.",
            introduced(r"\bmake\s*\(\s*map\["),
            "Staticcheck SA5000 catches assignments to maps proven nil.",
            "Do not infer that every zero map needs allocation.",
        ),
        RepairSignal(
            "panic recovery introduced",
            "A fix adds recover at a callback or process boundary.",
            introduced(r"\brecover\s*\("),
            "No general recovery requirement exists; the boundary contract is application-specific.",
            "Not suitable without a configured callback contract.",
        ),
        RepairSignal(
            "context-guarded channel send",
            "A bare channel send is replaced by a select that can observe context cancellation.",
            all_matchers(
                transformed(r"(?m)^\s*[A-Za-z_]\w*(?:\.\w+)?\s*<-", r"\bselect\s*\{"),
                lambda task: bool(re.search(r"\.Done\s*\(\)", task.added)),
            ),
            "No mainstream golangci-lint analyzer caught the replayed Bubble Tea defect; gohawk also missed it.",
            "Prototype a conservative producer-lifecycle extension that proves the receiver can exit on the same cancellation signal.",
        ),
        RepairSignal(
            "context-aware retry delay",
            "A time.Sleep retry delay is replaced by a timer selected with context cancellation.",
            all_matchers(
                transformed(r"\btime\.Sleep\s*\(", r"\bselect\s*\{"),
                lambda task: bool(re.search(r"\.Done\s*\(\)", task.added)),
            ),
            "github/gh-aw provides timesleepnocontext; mainstream golangci-lint and gohawk missed the replayed OpenSearch defect.",
            "Do not duplicate the broad rule; consider only stronger lifecycle evidence shared with channel sends.",
        ),
        RepairSignal(
            "live synchronization primitive reset",
            "A whole receiver assignment containing a fresh mutex is removed in favor of preserving the existing lock.",
            lambda task: bool(
                re.search(r"(?m)^\s*\*\w+\s*=\s*\w+(?:\[[^\]]+\])?\s*\{", task.removed)
                and re.search(r"(?:sync\.)?R?W?Mutex\s*\{", task.removed)
            ),
            "go vet copylocks rejects copied locks, but it and golangci-lint --default all missed the replayed fresh-lock replacement.",
            "Prototype an opt-in hazard for overwriting a receiver that contains an already-live synchronization primitive.",
        ),
        RepairSignal(
            "transaction rollback defer introduced",
            "A fix introduces deferred rollback for a transaction-like resource.",
            all_matchers(
                introduced(r"\bdefer\b"),
                lambda task: bool(re.search(r"\bRollback\w*\s*\(", task.added)),
            ),
            "gohawk resourcelifetime covers database/sql transactions; specialized transaction analyzers cover additional frameworks.",
            "Prefer configurable exact contracts over another built-in general check.",
        ),
    )


def diff_sides(patch: str) -> tuple[str, str]:
    added = []
    removed = []
    for line in patch.splitlines():
        if line.startswith("+") and not line.startswith("+++"):
            added.append(line[1:])
        elif line.startswith("-") and not line.startswith("---"):
            removed.append(line[1:])
    return "\n".join(added), "\n".join(removed)


def read_tasks(path: Path) -> list[Task]:
    try:
        from pyarrow import compute, parquet
    except ImportError as error:
        raise SystemExit(
            "analyze-swe-rebench-go: install pyarrow or run with `uv run --with pyarrow`"
        ) from error

    columns = ["language", "instance_id", "repo", "problem_statement", "patch", "meta"]
    table = parquet.read_table(path, columns=columns)
    table = table.filter(compute.equal(table["language"], "go"))
    tasks = []
    for row in table.to_pylist():
        metadata = row["meta"]["llm_metadata"]
        added, removed = diff_sides(row["patch"] or "")
        tasks.append(
            Task(
                instance_id=row["instance_id"],
                repository=row["repo"],
                title=(row["problem_statement"] or "").splitlines()[0].strip(),
                categories=frozenset(metadata["pr_categories"] or ()),
                quality=metadata["code"] or "unknown",
                added=added,
                removed=removed,
            )
        )
    return tasks


def markdown_report(tasks: list[Task], example_limit: int) -> str:
    bug_tasks = [task for task in tasks if task.categories & BUG_CATEGORIES]
    reviewed_tasks = [task for task in bug_tasks if task.quality == "A"]
    category_counts = Counter(
        category for task in tasks for category in task.categories
    )
    lines = [
        "# SWE-rebench V2 Go analyzer opportunity scan",
        "",
        "This report scans every Go row in the SWE-rebench V2 executable dataset, then narrows prevalence claims to bug-labelled, quality-A tasks. Repair signals are deterministic properties of gold patch additions and removals; they overlap and are not a claim that every matching task has the same root cause.",
        "",
        "Source: [nebius/SWE-rebench-V2](https://huggingface.co/datasets/nebius/SWE-rebench-V2), using the executable dataset Parquet snapshot with SHA-256 `0e0bf9355f892ad74ae98d4e1c404f39fd6654a8e351ee3e6ab162e4a64cd3ad`. See the adjacent README for the exact regeneration command.",
        "",
        "## Population",
        "",
        f"- Go tasks: **{len(tasks):,}** across **{len({task.repository for task in tasks}):,}** repositories.",
        f"- Bug-labelled tasks: **{len(bug_tasks):,}** across **{len({task.repository for task in bug_tasks}):,}** repositories.",
        f"- Quality-A bug tasks used for repair-signal counts: **{len(reviewed_tasks):,}** across **{len({task.repository for task in reviewed_tasks}):,}** repositories.",
        "",
        "Feature, documentation, infrastructure, ambiguous, and test-misaligned rows remain represented in the population totals but do not inflate the repair counts below.",
        "",
        "## Dataset categories",
        "",
        "| Category | Tasks |",
        "|---|---:|",
    ]
    for category, count in category_counts.most_common():
        lines.append(f"| `{category}` | {count:,} |")

    lines.extend(
        [
            "",
            "## Repair signals and analyzer overlap",
            "",
            "| Repair signal | Tasks | Repositories | Existing coverage | Disposition |",
            "|---|---:|---:|---|---|",
        ]
    )
    matches: dict[str, list[Task]] = {}
    for signal in repair_signals():
        selected = [task for task in reviewed_tasks if signal.matcher(task)]
        matches[signal.name] = selected
        lines.append(
            f"| {signal.name} | {len(selected):,} | {len({task.repository for task in selected}):,} | "
            f"{signal.existing_coverage} | {signal.disposition} |"
        )

    lines.extend(
        [
            "",
            "## Shortlist",
            "",
            "1. **Context-guarded channel sends.** Extend producer lifecycle only when analysis can connect a sender and receiver to the same cancellation signal and prove that the receiver may exit first. A mere channel send in a context-taking function is not sufficient evidence.",
            "2. **Live synchronization primitive reset.** Investigate whole-object assignments that overwrite a receiver containing a mutex, RWMutex, Once, WaitGroup, Cond, or noCopy-bearing atomic value. Start opt-in because reset methods may have a documented quiescence precondition.",
            "3. **Alias and buffer ownership research.** Defensive-copy fixes recur across independent repositories, but a diagnostic needs evidence that borrowed storage escapes or is mutated after transfer. Keep this as modeling work rather than a syntax check.",
            "",
            "The context-aware sleep pattern is real but already has a focused external analyzer. Transaction rollback is also covered for database/sql by gohawk and by specialist tools for additional frameworks. Neither should become a duplicate built-in check.",
            "",
            "## Representative overlap replay",
            "",
            "The three highest-confidence concrete gaps were replayed at their dataset base commits with gohawk at `937b55c4edcd` (`-enable-all`) and golangci-lint v2.13.2 (`--no-config --default all --tests=false`). Unrelated and style diagnostics were ignored; neither tool reported the repaired defect site.",
            "",
            "| Task | Base commit | Defect site | Result |",
            "|---|---|---|---|",
            "| `charmbracelet__bubbletea-1372` | `6a1ebaa0ea00` | `tea.go` | No diagnostic for the cancellation deadlock. |",
            "| `opensearch-project__opensearch-go-540` | `2464386c5b71` | `opensearchtransport/opensearchtransport.go` | No diagnostic for the cancellation-blind retry delay. |",
            "| `casbin__casbin-1229` | `2557f8dd4b37` | `persist/cache/cache_sync.go` | No diagnostic for replacing a live RWMutex. |",
            "",
            "## Representative tasks",
        ]
    )
    for signal in repair_signals():
        selected = matches[signal.name]
        if not selected:
            continue
        lines.extend(["", f"### {signal.name}", "", signal.description, ""])
        for task in sorted(selected, key=lambda item: item.instance_id)[:example_limit]:
            lines.append(f"- `{task.instance_id}` (`{task.repository}`): {task.title}")

    lines.extend(
        [
            "",
            "## Validation protocol for a proposed check",
            "",
            "1. Run the proposed analyzer on the dataset base commit and require a diagnostic at the defect site.",
            "2. Apply the gold patch and require that the diagnostic disappears.",
            "3. Run gohawk and golangci-lint with all checks enabled on both revisions to document overlap rather than infer it from names.",
            "4. Minimize one diagnostic and multiple accepted forms into local fixtures.",
            "5. Dogfood unrelated repositories before enabling the check; ambiguous ownership or lifecycle evidence suppresses the diagnostic.",
            "",
            "## Limitations",
            "",
            "- Gold patches can mix the actual repair with refactoring, generated changes, and test maintenance.",
            "- Patch signals overlap; counts must not be summed.",
            "- Quality-A is the dataset's LLM-assisted task-quality label, not a human proof that every changed line fixes the issue.",
            "- Absence of a patch signal does not imply absence of that bug class.",
            "- Replaying historical repositories can fail because of retired toolchains, dependencies, CGO libraries, or network services.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--input", type=Path, required=True, help="SWE-rebench V2 Parquet file"
    )
    parser.add_argument(
        "--output", type=Path, help="write Markdown report instead of stdout"
    )
    parser.add_argument(
        "--examples", type=int, default=5, help="examples per repair signal"
    )
    arguments = parser.parse_args()
    if arguments.examples < 0:
        parser.error("--examples must not be negative")

    report = markdown_report(read_tasks(arguments.input), arguments.examples)
    if arguments.output:
        arguments.output.write_text(report, encoding="utf-8")
    else:
        print(report, end="")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Replay a pinned, human-reviewed precision cohort."""

import argparse
import csv
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

def fail(message: str) -> None:
    raise SystemExit(f"precision-regression: {message}")


def run(command: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
    return subprocess.run(command, text=True, check=False, **kwargs)


def read_manifest(path: Path) -> list[tuple[str, str]]:
    rows = []
    with path.open(newline="") as source:
        for raw in csv.reader(source, delimiter="\t"):
            if not raw or raw[0].startswith("#"):
                continue
            if len(raw) != 2:
                fail(f"{path}: expected repository and revision")
            rows.append((raw[0], raw[1]))
    return rows


def checkout_repository(root: Path, repository: str, revision: str) -> Path:
    checkout = root / repository.replace("/", "__")
    if checkout.exists():
        actual = run(["git", "-C", str(checkout), "rev-parse", "HEAD"], capture_output=True)
        if actual.returncode or actual.stdout.strip() != revision:
            fail(f"{repository}: checkout is not pinned at {revision}")
        return checkout.resolve()
    checkout.mkdir(parents=True)
    commands = [
        ["git", "-C", str(checkout), "init", "--quiet"],
        ["git", "-C", str(checkout), "remote", "add", "origin", f"https://github.com/{repository}.git"],
        ["git", "-C", str(checkout), "fetch", "--quiet", "--depth=1", "origin", revision],
        ["git", "-C", str(checkout), "checkout", "--quiet", "--detach", "FETCH_HEAD"],
    ]
    for command in commands:
        result = run(command)
        if result.returncode:
            fail(f"{repository}: checkout command failed: {' '.join(command)}")
    return checkout


def module_directories(checkout: Path) -> list[Path]:
    modules = {path.parent for path in checkout.rglob("go.mod") if ".git" not in path.parts}
    return sorted(modules, key=lambda path: (len(path.relative_to(checkout).parts), str(path)))[:3]


def scan(gohawk: Path, repository: str, checkout: Path) -> set[tuple[str, str, str]]:
    findings: set[tuple[str, str, str]] = set()
    environment = os.environ | {
        "CGO_ENABLED": "0",
        "GOFLAGS": "-mod=readonly",
        "GONOSUMDB": "",
        "GOPRIVATE": "",
        "GOTOOLCHAIN": "local",
        "GOWORK": "off",
        "GOPROXY": "https://proxy.golang.org",
    }
    for module in module_directories(checkout):
        try:
            result = run(
                # Reviewed labels include findings in _test.go files, which the
                # default policy skips; replay them so the labels stay meaningful.
                [str(gohawk), "-enable-all", "-gohawk-include-tests", "-json", "./..."],
                cwd=module,
                env=environment,
                capture_output=True,
                timeout=180,
            )
        except subprocess.TimeoutExpired:
            print(f"warning: {repository}: timed out in {module.relative_to(checkout)}", file=sys.stderr)
            continue
        # gohawk still emits findings for the packages that loaded when others
        # fail, and several cohorts depend on those partial scans. An empty
        # payload, however, means the scan did not run at all (for example an
        # out-of-memory kill under a parallel replay), and treating that as
        # "no findings" would report reviewed true positives as lost.
        if not result.stdout.strip():
            print(
                f"warning: {repository}: no output from {module.relative_to(checkout)} "
                f"(exit {result.returncode}): {result.stderr.strip()[:200]}",
                file=sys.stderr,
            )
            continue
        try:
            payload = json.loads(result.stdout or "{}")
        except json.JSONDecodeError:
            print(f"warning: {repository}: invalid JSON in {module.relative_to(checkout)}", file=sys.stderr)
            continue
        errors = 0
        for analyzers in payload.values():
            if not isinstance(analyzers, dict):
                continue
            for analyzer, diagnostics in analyzers.items():
                for diagnostic in diagnostics:
                    if not isinstance(diagnostic, dict):
                        continue
                    if diagnostic.get("error"):
                        errors += 1
                        continue
                    position = str(diagnostic.get("posn", "")).replace(str(checkout) + os.sep, "")
                    findings.add((repository, analyzer, position))
        if errors:
            # A package that failed to load contributes no findings, so a lost
            # label from a module with load errors may be an environment problem
            # (a transient module fetch, or memory pressure under a parallel
            # replay) rather than an analyzer regression. Say so.
            print(
                f"warning: {repository}: {errors} package error(s) in {module.relative_to(checkout)}",
                file=sys.stderr,
            )
    return findings


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cohort", type=Path, help="directory containing repositories.tsv and labels.csv")
    parser.add_argument("--checkout-root", type=Path, help="reuse checkouts named owner__repository")
    parser.add_argument("--gohawk", type=Path, help="existing gohawk binary")
    parser.add_argument("--only", action="append", default=[], help="run one repository (repeatable)")
    args = parser.parse_args()

    repository_root = Path(__file__).resolve().parents[1]
    cohort = args.cohort.resolve()
    manifest = read_manifest(cohort / "repositories.tsv")
    selected = set(args.only)
    manifest = [row for row in manifest if not selected or row[0] in selected]
    if not manifest:
        fail("no repositories selected")

    temporary = Path(tempfile.mkdtemp(prefix="gohawk-precision."))
    try:
        gohawk = args.gohawk.resolve() if args.gohawk else temporary / "gohawk"
        if not args.gohawk:
            result = run(["go", "build", "-trimpath", "-o", str(gohawk), "."], cwd=repository_root)
            if result.returncode:
                fail("gohawk build failed")
        checkout_root = args.checkout_root.resolve() if args.checkout_root else temporary / "repositories"
        checkout_root.mkdir(parents=True, exist_ok=True)
        findings: set[tuple[str, str, str]] = set()
        for repository, revision in manifest:
            print(f"scanning {repository}@{revision[:12]}", flush=True)
            findings |= scan(gohawk, repository, checkout_repository(checkout_root, repository, revision))

        with (cohort / "labels.csv").open(newline="") as source:
            labels = list(csv.DictReader(source))
        labels = [row for row in labels if not selected or row["repository"] in selected]
        false_positives = {
            (row["repository"], row["analyzer"], row["position"])
            for row in labels
            if row["verdict"] == "false_positive"
        }
        true_positives = {
            (row["repository"], row["analyzer"], row["position"])
            for row in labels
            if row["verdict"] == "true_positive"
        }
        returned_noise = sorted(false_positives & findings)
        lost_signal = sorted(true_positives - findings)
        for finding in returned_noise:
            print("returned false positive:", *finding, file=sys.stderr)
        for finding in lost_signal:
            print("lost true positive:", *finding, file=sys.stderr)
        print(
            f"checked {len(labels)} labels: {len(false_positives) - len(returned_noise)} of "
            f"{len(false_positives)} false positives remain absent; "
            f"{len(true_positives) - len(lost_signal)} of {len(true_positives)} true positives remain present"
        )
        if returned_noise or lost_signal:
            raise SystemExit(1)
    finally:
        shutil.rmtree(temporary)


if __name__ == "__main__":
    main()

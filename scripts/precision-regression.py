#!/usr/bin/env python3
"""Replay a pinned, human-reviewed precision cohort."""

import argparse
import csv
import datetime
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


def merge_json_stream(text: str) -> dict:
    """Merge the JSON objects go vet prints, one per package, into one document."""
    decoder = json.JSONDecoder()
    merged: dict = {}
    index = 0
    while True:
        while index < len(text) and text[index].isspace():
            index += 1
        if index >= len(text):
            return merged
        payload, end = decoder.raw_decode(text, index)
        if isinstance(payload, dict):
            merged.update(payload)
        index = end



def loadable_packages(module: Path, environment: dict[str, str]) -> list[str]:
    """Return the packages in the module whose own and dependency imports resolve."""
    try:
        listed = run(
            ["go", "list", "-e", "-f",
             "{{if and (not .Error) (not .DepsErrors)}}{{.ImportPath}}{{end}}", "./..."],
            cwd=module,
            env=environment,
            capture_output=True,
            timeout=180,
        )
    except subprocess.TimeoutExpired:
        return []
    return [line for line in listed.stdout.split("\n") if line.strip()]


def retry_scan(
    gohawk: Path, module: Path, environment: dict[str, str], packages: list[str]
) -> subprocess.CompletedProcess[str]:
    """Re-run the scan over an explicit package list."""
    try:
        return run(
            ["go", "vet", f"-vettool={gohawk}", "-enable-all", "-gohawk-include-tests", "-json", *packages],
            cwd=module,
            env=environment,
            capture_output=True,
            timeout=180,
        )
    except subprocess.TimeoutExpired:
        return subprocess.CompletedProcess(args=[], returncode=1, stdout="", stderr="timed out")


def scan(gohawk: Path, repository: str, checkout: Path) -> tuple[set[tuple[str, str, str]], list[str]]:
    """Return the findings, and the reasons any module could not be analysed.

    A module that fails to build contributes no findings, so every reviewed
    label in it reads as a lost true positive. That failure can only invent
    regressions, never hide one, so the caller must be able to tell it apart
    from a real change rather than see it as missing signal.
    """
    findings: set[tuple[str, str, str]] = set()
    incomplete: list[str] = []
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
                # Running through go vet analyzes one package at a time from
                # export data, so a module with a large dependency graph does not
                # need every dependency type-checked from source at once.
                ["go", "vet", f"-vettool={gohawk}", "-enable-all", "-gohawk-include-tests", "-json", "./..."],
                cwd=module,
                env=environment,
                capture_output=True,
                timeout=180,
            )
        except subprocess.TimeoutExpired:
            incomplete.append(f"timed out in {module.relative_to(checkout)}")
            continue
        # gohawk still emits findings for the packages that loaded when others
        # fail, and several cohorts depend on those partial scans. An empty
        # payload, however, means the scan did not run at all (for example an
        # out-of-memory kill under a parallel replay), and treating that as
        # "no findings" would report reviewed true positives as lost.
        if not result.stdout.strip() and result.returncode:
            # One package whose imports do not resolve aborts loading for the
            # whole ./... pattern, so a repository loses every label over a
            # single unbuildable corner. nats.go carries an encoders/protobuf
            # package whose import is absent from its own go.mod, and the other
            # thirty packages build. Retry with the packages that do load, so
            # only the broken corner is lost.
            loadable = loadable_packages(module, environment)
            result = retry_scan(gohawk, module, environment, loadable) if loadable else result
        if not result.stdout.strip() and result.returncode:
            incomplete.append(
                f"no output from {module.relative_to(checkout)} "
                f"(exit {result.returncode}): {result.stderr.strip()[:160]}"
            )
            continue
        try:
            payload = merge_json_stream(result.stdout)
        except json.JSONDecodeError:
            incomplete.append(f"invalid JSON in {module.relative_to(checkout)}")
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
            incomplete.append(f"{errors} package error(s) in {module.relative_to(checkout)}")
    return findings, incomplete


# A label records a human verdict about one finding. The repository revision is
# pinned in the manifest, so the input is reproducible, but without the gohawk
# revision that produced the verdict a failure cannot distinguish "this change
# broke it" from "this drifted some releases ago". LABEL_PROVENANCE carries
# that, and --stamp refreshes it for every label that still holds, so the
# recorded revision means "last confirmed at", not "first written at".
LABEL_FIELDS = ["repository", "analyzer", "position", "verdict"]
LABEL_PROVENANCE = ["gohawk_revision", "confirmed_at"]


def describe_provenance(row: dict) -> str:
    revision = (row.get("gohawk_revision") or "").strip()
    confirmed = (row.get("confirmed_at") or "").strip()
    if not revision:
        return "provenance unknown"
    return f"last confirmed at {revision}" + (f" on {confirmed}" if confirmed else "")


def current_revision(repository_root: Path) -> str:
    revision = run(
        ["git", "-C", str(repository_root), "rev-parse", "--short", "HEAD"], capture_output=True
    ).stdout.strip()
    if not revision:
        return "unknown"
    dirty = run(
        ["git", "-C", str(repository_root), "status", "--porcelain"], capture_output=True
    ).stdout.strip()
    # A stamp from a modified tree names a revision that does not contain the
    # behaviour it certifies, so say so rather than record a revision that
    # cannot be checked out and reproduced.
    return revision + ("-dirty" if dirty else "")


def stamp_labels(cohort: Path, labels: list[dict], held: set[tuple[str, str, str]], revision: str) -> int:
    """Record the running revision on every label that still holds."""
    today = datetime.date.today().isoformat()
    stamped = 0
    for row in labels:
        if (row["repository"], row["analyzer"], row["position"]) not in held:
            continue
        row["gohawk_revision"] = revision
        row["confirmed_at"] = today
        stamped += 1
    path = cohort / "labels.csv"
    with path.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=LABEL_FIELDS + LABEL_PROVENANCE)
        writer.writeheader()
        for row in labels:
            writer.writerow({field: row.get(field) or "" for field in LABEL_FIELDS + LABEL_PROVENANCE})
    return stamped


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cohort", type=Path, help="directory containing repositories.tsv and labels.csv")
    parser.add_argument("--checkout-root", type=Path, help="reuse checkouts named owner__repository")
    parser.add_argument("--gohawk", type=Path, help="existing gohawk binary")
    parser.add_argument("--only", action="append", default=[], help="run one repository (repeatable)")
    parser.add_argument(
        "--require-scannable",
        action="store_true",
        help="fail when a repository could not be analysed, rather than only reporting it",
    )
    parser.add_argument(
        "--stamp",
        action="store_true",
        help="record the running gohawk revision on every label that still holds",
    )
    parser.add_argument(
        "--analyzer",
        action="append",
        default=[],
        help="replay only labels for this analyzer, skipping repositories that have none (repeatable)",
    )
    args = parser.parse_args()

    repository_root = Path(__file__).resolve().parents[1]
    cohort = args.cohort.resolve()
    manifest = read_manifest(cohort / "repositories.tsv")
    selected = set(args.only)
    manifest = [row for row in manifest if not selected or row[0] in selected]

    with (cohort / "labels.csv").open(newline="") as source:
        labels = list(csv.DictReader(source))
    labels = [row for row in labels if not selected or row["repository"] in selected]
    analyzers = set(args.analyzer)
    if analyzers:
        # Scoping to an analyzer only has to scan the repositories that carry a
        # label for it, which is what makes a single-analyzer replay quick
        # enough to run beside the unit tests.
        labels = [row for row in labels if row["analyzer"] in analyzers]
        labelled = {row["repository"] for row in labels}
        manifest = [row for row in manifest if row[0] in labelled]
        if not manifest:
            print(f"{cohort.name}: no labels for {', '.join(sorted(analyzers))}")
            return
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
        unscannable: dict[str, list[str]] = {}
        for repository, revision in manifest:
            print(f"scanning {repository}@{revision[:12]}", flush=True)
            found, incomplete = scan(gohawk, repository, checkout_repository(checkout_root, repository, revision))
            findings |= found
            if incomplete:
                unscannable[repository] = incomplete

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
        # A label in a repository that did not analyse cleanly proves nothing.
        # Counting it as lost turns a checkout that needs a build step into a
        # phantom regression, which is the failure this partition prevents.
        blocked = sorted(finding for finding in lost_signal if finding[0] in unscannable)
        lost_signal = [finding for finding in lost_signal if finding[0] not in unscannable]
        # A false positive in an unscannable repository is equally unchecked. It
        # would otherwise read as "remains absent", which is a pass the run did
        # not earn, so drop it from the denominator too.
        blocked_noise = sorted(finding for finding in false_positives if finding[0] in unscannable)
        checked_noise = len(false_positives) - len(blocked_noise)
        checked_signal = len(true_positives) - len(blocked)
        for repository, reasons in sorted(unscannable.items()):
            print(f"unscannable: {repository}: {reasons[0]}", file=sys.stderr)
        provenance = {
            (row["repository"], row["analyzer"], row["position"]): describe_provenance(row) for row in labels
        }
        for finding in returned_noise:
            print("returned false positive:", *finding, f"({provenance[finding]})", file=sys.stderr)
        for finding in lost_signal:
            print("lost true positive:", *finding, f"({provenance[finding]})", file=sys.stderr)
        print(
            f"checked {len(labels) - len(blocked) - len(blocked_noise)} labels: "
            f"{checked_noise - len(returned_noise)} of {checked_noise} false positives remain absent; "
            f"{checked_signal - len(lost_signal)} of {checked_signal} true positives remain present"
        )
        if blocked or blocked_noise:
            print(
                f"not checked: {len(blocked) + len(blocked_noise)} label(s) in {len(unscannable)} "
                "repository(ies) that could not be analysed; fix or drop those repositories to "
                "restore the coverage"
            )
        # Unscannable repositories are pre-existing corpus debt, so they do not
        # fail the gate by default; --require-scannable enforces it once a
        # cohort is clean, which keeps the debt from growing silently.
        if args.require_scannable and unscannable:
            raise SystemExit(1)
        if args.stamp:
            held = ((false_positives - findings) | (true_positives & findings)) - set(blocked)
            revision = current_revision(repository_root)
            print(f"stamped {stamp_labels(cohort, labels, held, revision)} holding labels at {revision}")
        if returned_noise or lost_signal:
            raise SystemExit(1)
    finally:
        shutil.rmtree(temporary)


if __name__ == "__main__":
    main()

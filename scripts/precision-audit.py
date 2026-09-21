#!/usr/bin/env python3
"""Scan fresh, pinned repositories without assigning review verdicts.

The precision replay owns checkout and analysis mechanics. This runner adds
bounded parallelism, history deduplication, and restartable per-repository
reports. It never executes repository tests or generation commands.
"""

import argparse
import concurrent.futures
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent.parent
SPEC = importlib.util.spec_from_file_location("precision_replay", ROOT / "scripts/precision-regression.py")
REPLAY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REPLAY)


def read_manifest(path):
    entries = []
    seen = set()
    for line in path.read_text().splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        fields = line.split("\t")
        if len(fields) != 2:
            raise ValueError(f"{path}: expected repository<TAB>full commit SHA")
        repo, sha = fields
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo) or any(
            part in (".", "..") for part in repo.split("/")
        ):
            raise ValueError(f"invalid repository: {repo}")
        if not re.fullmatch(r"[0-9a-f]{40}", sha):
            raise ValueError(f"{repo}: a full lowercase commit SHA is required")
        if repo.lower() in seen:
            raise ValueError(f"duplicate repository: {repo}")
        seen.add(repo.lower())
        entries.append((repo, sha))
    return entries


def read_history(paths):
    seen = set()
    for path in paths:
        for line in path.read_text().splitlines():
            if not line.strip() or line.startswith("#"):
                continue
            fields = line.split("\t")
            # Audit selection ledgers: batch, repository, revision, modules, status.
            # Regression cohorts and pinned candidate manifests: repository, revision.
            # Reviewed findings: repository, revision, analyzer, position,
            # checks, verdict, reason. These also live beside selection ledgers.
            if len(fields) not in (2, 5, 7):
                raise ValueError(f"{path}: unsupported history row")
            seen.add(fields[1 if len(fields) == 5 else 0].lower())
    return seen


def save_report(path, report):
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    temporary.replace(path)


def analyze(entry, binary, checkouts, output):
    repo, sha = entry
    destination = output / (repo.replace("/", "__") + ".json")
    if destination.exists():
        report = json.loads(destination.read_text())
        if (report["repository"], report["revision"]) != entry:
            raise ValueError(f"{repo}: saved report does not match pinned input")
        return report
    print(f"Scanning {repo}@{sha[:12]}", flush=True)
    report = {"repository": repo, "revision": sha, "review_status": "unreviewed"}
    try:
        checkout = REPLAY.checkout_repository(checkouts, repo, sha)
        report["modules"] = [str(p.relative_to(checkout)) for p in REPLAY.module_directories(checkout)]
        findings, checks, errors = REPLAY.scan(binary, repo, checkout)
        report["findings"] = [
            {"analyzer": item[1], "position": item[2], "checks": sorted(checks.get(item, []))}
            for item in sorted(findings)
        ]
        report["errors"] = errors
        report["scan_status"] = "incomplete" if errors else "scanned" if report["modules"] else "no_modules"
    except (Exception, SystemExit) as error:
        report.update(scan_status="failed", findings=[], errors=[str(error)])
    save_report(destination, report)
    print(f"{repo}: {report['scan_status']}, {len(report['findings'])} findings", flush=True)
    return report


def positive(value):
    number = int(value)
    if number < 1:
        raise argparse.ArgumentTypeError("must be positive")
    return number


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--gohawk", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--exclude-ledger", type=Path, action="append", default=[])
    parser.add_argument("--limit", type=positive, default=25)
    parser.add_argument("--jobs", type=positive, default=2)
    args = parser.parse_args()
    binary = args.gohawk.resolve(strict=True)
    output = args.output.resolve()
    manifest = read_manifest(args.manifest)
    history_paths = sorted((ROOT / "benchmarks/precision").glob("round-*/repositories.tsv"))
    history_paths += sorted((ROOT / "benchmarks/precision/audits").glob("*.tsv"))
    history = read_history(history_paths + args.exclude_ledger)
    selected = [entry for entry in manifest if entry[0].lower() not in history][:args.limit]
    if not selected:
        parser.error("no fresh repositories remain after history deduplication")
    output.mkdir(parents=True, exist_ok=True)
    metadata = {
        "repositories": [list(entry) for entry in selected],
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "replay_sha256": hashlib.sha256((ROOT / "scripts/precision-regression.py").read_bytes()).hexdigest(),
        "go_version": REPLAY.run(["go", "version"], capture_output=True).stdout.strip(),
        "profile": "-enable-all -gohawk-include-tests -json",
    }
    run_file = output / "run.json"
    if run_file.exists() and json.loads(run_file.read_text()) != metadata:
        parser.error("output belongs to different inputs/tooling; use a new directory")
    save_report(run_file, metadata)
    # Keep compilation concurrency bounded too; the shared replay sets CGO=0,
    # GOWORK=off, GOTOOLCHAIN=local and -mod=readonly for candidate modules.
    os.environ["GOMAXPROCS"] = "2"
    checkouts = output / "checkouts"
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as pool:
        reports = list(pool.map(lambda entry: analyze(entry, binary, checkouts, output), selected))
    save_report(output / "summary.json", {"reports": reports})
    return int(any(report["scan_status"] != "scanned" for report in reports))


if __name__ == "__main__":
    raise SystemExit(main())

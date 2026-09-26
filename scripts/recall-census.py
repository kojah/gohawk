#!/usr/bin/env python3
"""Stage 1 of a recall audit: count every candidate a check saw by the reason it
reached its final decision.

The scan reuses the precision replay's pinned checkouts, module selection, and
build environment, and adds the evidence tracer for the chosen analyzers. Only
the candidate, considered, and decision events are kept: the census needs the
reason each candidate stopped at, and the evidence steps would multiply the
trace a hundredfold. Recall results are exploratory and never feed a gate; see
.agents/skills/gohawk-recall-audit/SKILL.md.
"""

import argparse
import collections
import importlib.util
import json
import random
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
_spec = importlib.util.spec_from_file_location("precision_regression", HERE / "precision-regression.py")
replay = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(replay)

KEPT_PHASES = {"candidate", "considered", "decision", "label"}
# The diagnostic events check.Report emits name the reported range, not the
# proof's candidate, so they are counted from the proof's own decision.
REPORT_EVENTS = {"diagnostic-candidate", "diagnostic-reported"}


def select(manifests: list[Path], sample: int, seed: int) -> list[tuple[str, str]]:
    pins: dict[str, str] = {}
    for manifest in manifests:
        for repository, revision in replay.read_manifest(manifest):
            pins.setdefault(repository, revision)
    ordered = sorted(pins.items())
    random.Random(seed).shuffle(ordered)
    return ordered[:sample] if sample else ordered


def scan_module(gohawk: Path, module: Path, analyzers: str, trace: Path, environment: dict[str, str]) -> str:
    """Scan one module, retrying with the packages that load when ./... cannot,
    as the precision replay does, so one unbuildable corner does not cost the
    whole module."""
    trace.unlink(missing_ok=True)
    flags = [
        f"-vettool={gohawk}", f"-enable={analyzers}", "-gohawk-include-tests", "-json",
        f"-gohawk-trace={analyzers}", f"-gohawk-trace-file={trace}",
    ]
    for patterns in (["./..."], None):
        if patterns is None:
            patterns = replay.loadable_packages(module, environment)
            if not patterns:
                return "" if not replay.module_has_packages(module, environment) else "no loadable packages"
            trace.unlink(missing_ok=True)
        try:
            result = replay.run(["go", "vet", *flags, *patterns], cwd=module, env=environment, capture_output=True, timeout=600)
        except subprocess.TimeoutExpired:
            return "timed out"
        if result.stdout.strip() or not result.returncode:
            return ""
    return "no output"


def keep_proof_events(trace: Path, checkout: Path, sink) -> int:
    """Copy the census events whose candidate lies in the repository's own source."""
    kept = 0
    if not trace.exists():
        return 0
    root = str(checkout) + "/"
    with trace.open() as lines:
        for line in lines:
            event = json.loads(line)
            candidate = event.get("candidate", "")
            if event.get("phase") not in KEPT_PHASES or event.get("reason") in REPORT_EVENTS:
                continue
            if event["phase"] == "label" and event.get("outcome") != "unknown":
                continue
            if not candidate.startswith(root) or "/vendor/" in candidate:
                continue
            event["candidate"] = candidate[len(root):]
            event.pop("details", None)
            sink.write(json.dumps(event) + "\n")
            kept += 1
    trace.unlink()
    return kept


def scan_all(arguments: argparse.Namespace) -> None:
    output: Path = arguments.output
    output.mkdir(parents=True, exist_ok=True)
    environment = replay.os.environ | {
        "CGO_ENABLED": "0", "GOFLAGS": "-mod=readonly", "GONOSUMDB": "", "GOPRIVATE": "",
        "GOTOOLCHAIN": "local", "GOWORK": "off", "GOPROXY": "https://proxy.golang.org",
    }
    selected = select(arguments.manifest, arguments.sample, arguments.seed)
    (output / "selection.tsv").write_text("".join(f"{repository}\t{revision}\n" for repository, revision in selected))
    status = {}
    for number, (repository, revision) in enumerate(selected, 1):
        events = output / (repository.replace("/", "__") + ".jsonl")
        if events.exists():
            continue
        print(f"[{number}/{len(selected)}] {repository}", flush=True)
        try:
            checkout = replay.checkout_repository(arguments.checkout_root, repository, revision)
        except SystemExit:
            status[repository] = "checkout failed"
            continue
        problems, kept = [], 0
        partial = events.with_suffix(".partial")
        with partial.open("w") as sink:
            for module in replay.module_directories(checkout):
                problem = scan_module(arguments.gohawk, module, arguments.analyzers, output / "trace.tmp", environment)
                if problem:
                    problems.append(f"{module.relative_to(checkout)}: {problem}")
                kept += keep_proof_events(output / "trace.tmp", checkout.resolve(), sink)
        partial.rename(events)
        status[repository] = "; ".join(problems) or "ok"
        print(f"    {kept} events; {status[repository]}", flush=True)
    with (output / "status.json").open("a") as record:
        record.write(json.dumps(status) + "\n")


def summarize(arguments: argparse.Namespace) -> None:
    """Print one row per check: candidates, reported, and decline reasons."""
    final: dict[tuple[str, str, str], dict] = {}
    unknown_labels: dict[tuple[str, str, str], set[str]] = collections.defaultdict(set)
    repositories_with: dict[str, set[str]] = collections.defaultdict(set)
    for path in sorted(arguments.output.glob("*.jsonl")):
        repository = path.stem.replace("__", "/")
        for line in path.open():
            event = json.loads(line)
            check = event.get("check") or event["analyzer"]
            key = (repository, check, event["candidate"])
            if event["phase"] == "label":
                unknown_labels[key].add(event["reason"])
            if event["phase"] != "decision":
                continue
            final[key] = event
            repositories_with[check].add(repository)
    by_check: dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    reported: collections.Counter = collections.Counter()
    for (_, check, _), event in final.items():
        key = f"{event['reason']} ({event['outcome']})"
        by_check[check][key] += 1
        if event["outcome"] == "rejected":
            reported[check] += 1
    # A decision that a path met an opaque use names only that; the unknown
    # labels on the candidate say which boundary it was.
    boundaries: dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    for key, event in final.items():
        undecided = event["outcome"] == "unknown" or event["reason"] == "opaque-consumption"
        if event["outcome"] != "rejected" and (undecided or unknown_labels.get(key)):
            for label in unknown_labels.get(key, ()) or ("(no unknown label)",):
                boundaries[key[1]][label] += 1
    for check in sorted(by_check):
        total = sum(by_check[check].values())
        print(f"## {check}: {total} candidates in {len(repositories_with[check])} repositories, {reported[check]} decided as defects")
        for reason, count in by_check[check].most_common():
            print(f"  {count:6d}  {100 * count / total:5.1f}%  {reason}")
        if boundaries[check]:
            print("  unknown labels on silent candidates (a candidate can carry several):")
            for label, count in boundaries[check].most_common(15):
                print(f"    {count:6d}  {label}")


def silent_candidates(output: Path) -> list[dict]:
    """Every candidate that ended silent, with its decision and unknown labels."""
    pins = dict(line.split("\t") for line in (output / "selection.tsv").read_text().split("\n") if line)
    rows: dict[tuple[str, str, str], dict] = {}
    for path in sorted(output.glob("*.jsonl")):
        repository = path.stem.replace("__", "/")
        for line in path.open():
            event = json.loads(line)
            check = event.get("check") or event["analyzer"]
            key = (repository, check, event["candidate"])
            row = rows.setdefault(key, {"repository": repository, "revision": pins[repository].strip(), "check": check,
                                        "candidate": event["candidate"], "labels": []})
            if event["phase"] == "label":
                position = event.get("position", "").split(repository.replace("/", "__") + "/")[-1]
                row["labels"].append((event["reason"], position))
            if event["phase"] == "decision":
                row["decision"], row["outcome"] = event["reason"], event["outcome"]
    return [row for row in rows.values() if row.get("outcome") not in (None, "rejected")]


def draw(arguments: argparse.Namespace) -> None:
    """Write a fixed-seed sample of silent candidates for one check and reason."""
    rows = [row for row in silent_candidates(arguments.output) if row["check"] == arguments.check]
    reason = arguments.reason
    matching = [row for row in rows if row["decision"] == reason or any(label == reason for label, _ in row["labels"])
                or reason == "(no unknown label)" and not row["labels"] and row["decision"] == "opaque-consumption"]
    matching.sort(key=lambda row: (row["repository"], row["candidate"]))
    random.Random(arguments.seed).shuffle(matching)
    for row in matching[: arguments.count]:
        path, line = row["candidate"].rsplit(":", 2)[0], row["candidate"].rsplit(":", 2)[1]
        link = f"https://github.com/{row['repository']}/blob/{row['revision']}/{path}#L{line}"
        labels = "; ".join(f"{label}@{position}" for label, position in row["labels"])
        print("\t".join([row["check"], reason, row["repository"], row["revision"], row["candidate"], row["decision"], labels, link]))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, action="append", default=[], help="pinned owner/repository TSV (repeatable)")
    parser.add_argument("--sample", type=int, default=0, help="repositories to draw with the seed (default all)")
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--checkout-root", type=Path, default=Path(".build/recall-checkouts"))
    parser.add_argument("--gohawk", type=Path, help="gohawk binary")
    parser.add_argument("--analyzers", default="resourcelifetime")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--summarize", action="store_true", help="only print the census of an earlier scan")
    parser.add_argument("--draw", action="store_true", help="print a sample of silent candidates for --check and --reason")
    parser.add_argument("--check")
    parser.add_argument("--reason")
    parser.add_argument("--count", type=int, default=12)
    arguments = parser.parse_args()
    if arguments.draw:
        draw(arguments)
        return
    if arguments.summarize:
        summarize(arguments)
        return
    if arguments.gohawk is None or not arguments.manifest:
        sys.exit("--gohawk and --manifest are required to scan")
    arguments.gohawk = arguments.gohawk.resolve()
    arguments.checkout_root = arguments.checkout_root.resolve()
    scan_all(arguments)
    summarize(arguments)


if __name__ == "__main__":
    main()

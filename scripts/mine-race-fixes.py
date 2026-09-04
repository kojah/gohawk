#!/usr/bin/env python3
"""Gather real concurrency-bug fixes and replay a check against the revision
that still had the bug.

A precision audit labels findings the analyzer reported, so it can say how
often a report is wrong and never how often the analyzer stayed silent when it
should not have. Recall needs ground truth from outside the analyzer's own
output, and a fix commit is exactly that: the revision before it is a labelled
defect, and the message says what the defect was.

The seed queries describe the SYMPTOM -- a data race was fixed -- and never the
mechanism a particular check keys on. Searching for the mechanism, such as an
RLock that became a Lock, pre-filters the corpus to bugs shaped like the
detector and turns any resulting number into a restatement of the detector's
own assumptions. Bugs fixed by adding a mutex, restructuring, switching to
atomics, or copying a value out have to stay in the denominator.

For the same reason this script does not decide whether a candidate is in a
check's class. It gathers candidates, replays the check against the parent
revision, and writes a worksheet with an empty label column. Classification is
a reviewed judgement, as it is for a precision cohort, and review produces two
numbers rather than one:

  prevalence -- of real race fixes, how many are the shape this check targets,
                which decides whether the check is worth having at all;
  recall     -- of those, how many the check reported.

A revision that does not build is recorded as such rather than counted as a
miss, because an unanalysable package and a missed defect are different facts.
"""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time

# Phrasings that describe a fixed race without naming a synchronisation
# primitive. Adding a query that names one reintroduces the selection bias this
# script exists to avoid.
SEED_QUERIES = [
    "fix data race",
    "fix race condition",
    "fixes data race",
    "resolve data race",
    "data race detected",
    "race detector reported",
]

COMMIT_URL = re.compile(r"github\.com/([^/]+/[^/]+)/commit/([0-9a-f]{7,40})")


def fail(message: str) -> None:
    raise SystemExit(f"mine-race-fixes: {message}")


def run(command: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
    return subprocess.run(command, text=True, check=False, capture_output=True, **kwargs)


def search(query: str, limit: int, attempts: int = 5) -> list[tuple[str, str, str]]:
    """Return (repository, sha, subject) for one query, backing off when the
    commit search endpoint applies its secondary rate limit."""
    delay = 20.0
    for attempt in range(attempts):
        result = run(["gh", "search", "commits", query, "--limit", str(limit), "--json", "sha,commit,url"])
        if result.returncode == 0:
            return parse_hits(result.stdout)
        if "rate limit" not in result.stderr.lower():
            print(f"  query failed: {result.stderr.strip()[:120]}", file=sys.stderr)
            return []
        if attempt + 1 < attempts:
            print(f"  rate limited, waiting {delay:.0f}s", file=sys.stderr)
            time.sleep(delay)
            delay *= 2
    return []


def parse_hits(payload: str) -> list[tuple[str, str, str]]:
    hits = []
    for entry in json.loads(payload or "[]"):
        # Commit search leaves the repository field unpopulated, so the
        # repository is recovered from the commit URL instead.
        match = COMMIT_URL.search(entry.get("url", ""))
        if not match:
            continue
        subject = (entry.get("commit", {}).get("message") or "").splitlines()[:1]
        hits.append((match.group(1), entry.get("sha", match.group(2)), subject[0] if subject else ""))
    return hits


def clone(repository: str, work: Path) -> Path | None:
    """Blobless clone keeps history searchable without fetching every revision's
    file contents."""
    target = work / repository.replace("/", "__")
    if target.exists():
        return target
    result = run(["git", "clone", "--quiet", "--filter=blob:none",
                  f"https://github.com/{repository}.git", str(target)])
    if result.returncode != 0:
        print(f"  clone failed: {repository}", file=sys.stderr)
        return None
    return target


def changed_go_packages(checkout: Path, sha: str) -> list[str]:
    result = run(["git", "-C", str(checkout), "show", "--name-only", "--format=", sha])
    if result.returncode != 0:
        return []
    packages = set()
    for line in result.stdout.splitlines():
        if line.endswith(".go") and "/testdata/" not in line:
            packages.add("./" + os.path.dirname(line) if os.path.dirname(line) else "./")
    return sorted(packages)


def replay(checkout: Path, sha: str, check: str, gohawk: Path, timeout: int) -> tuple[str, int]:
    """Check out the parent of a fix and run one check over the packages it
    touched. Returns an outcome and the number of findings."""
    packages = changed_go_packages(checkout, sha)
    if not packages:
        return "no-go-files", 0
    if run(["git", "-C", str(checkout), "checkout", "--quiet", f"{sha}^"]).returncode != 0:
        return "no-parent", 0
    build = run(["go", "build", *packages], cwd=checkout, timeout=timeout)
    if build.returncode != 0:
        # An unanalysable revision is not a missed defect, and counting it as
        # one would understate the check.
        return "unbuildable", 0
    analysis = run([str(gohawk), "-enable-checks", check, *packages], cwd=checkout, timeout=timeout)
    findings = sum(1 for line in analysis.stdout.splitlines() if line.startswith("warning["))
    return ("reported" if findings else "silent"), findings


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", default="lockorder/read-lock-write", help="check to replay")
    parser.add_argument("--gohawk", type=Path, required=True, help="gohawk binary to replay with")
    parser.add_argument("--work", type=Path, required=True, help="directory for cached clones")
    parser.add_argument("--out", type=Path, required=True, help="worksheet to write")
    parser.add_argument("--per-query", type=int, default=10, help="commits to take from each query")
    parser.add_argument("--pause", type=float, default=8.0, help="seconds between searches")
    parser.add_argument("--timeout", type=int, default=900, help="per-revision build and analysis timeout")
    arguments = parser.parse_args()

    if not arguments.gohawk.exists():
        fail(f"{arguments.gohawk} does not exist; build it first")
    arguments.work.mkdir(parents=True, exist_ok=True)

    candidates: dict[tuple[str, str], str] = {}
    for query in SEED_QUERIES:
        print(f"searching: {query}", file=sys.stderr)
        for repository, sha, subject in search(query, arguments.per_query):
            candidates.setdefault((repository, sha), subject)
        time.sleep(arguments.pause)

    with arguments.out.open("w", newline="") as sheet:
        sheet.write("repository\tsha\toutcome\tfindings\tlabel\tsubject\n")
        for (repository, sha), subject in sorted(candidates.items()):
            checkout = clone(repository, arguments.work)
            if checkout is None:
                continue
            try:
                outcome, findings = replay(checkout, sha, arguments.check, arguments.gohawk, arguments.timeout)
            except subprocess.TimeoutExpired:
                outcome, findings = "timeout", 0
            print(f"  {repository}@{sha[:9]}: {outcome}", file=sys.stderr)
            sheet.write(f"{repository}\t{sha}\t{outcome}\t{findings}\t\t{subject}\n")

    print(f"\nwrote {arguments.out}. Label each row before counting:", file=sys.stderr)
    print("  in-class    the parent really does have the defect this check targets", file=sys.stderr)
    print("  other-race  a real race, but not this check's shape (counts for prevalence)", file=sys.stderr)
    print("  not-a-race  the commit was not fixing a race after all", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

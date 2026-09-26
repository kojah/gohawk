#!/usr/bin/env python3
"""Gather real bug fixes -- races, deadlocks, and resource, goroutine, and
context leaks -- and replay a check against the revision that still had the
bug.

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
# Commit search has no language qualifier, so several queries mention a Go
# concept instead. That narrows by LANGUAGE, which is legitimate, and not by
# synchronisation mechanism, which would not be.
SEED_QUERIES = [
    "fix data race",
    "fix race condition",
    "fixes data race",
    "resolve data race",
    "data race detected",
    "race detector reported",
    "data race goroutine",
    "race condition goroutine",
    "go test -race",
]

# Deadlock fixes follow the same rule: the phrasing names the symptom, a
# program that stopped making progress, and never a lock, channel, or group.
# The Go runtime's own message is the most specific symptom there is.
DEADLOCK_QUERIES = [
    "fix deadlock",
    "fixes deadlock",
    "fix a deadlock",
    "fix potential deadlock",
    "resolve deadlock",
    "deadlock goroutine",
    "all goroutines are asleep",
    "goroutine hang",
]

# Leak fixes name what ran out or piled up, never the call that was missing:
# "fd leak" and "too many open files", not "add Close" or "defer cancel".
RESOURCE_LEAK_QUERIES = [
    "fix fd leak",
    "fix file descriptor leak",
    "too many open files",
    "fix resource leak",
    "fix connection leak",
    "fix socket leak",
]

GOROUTINE_LEAK_QUERIES = [
    "fix goroutine leak",
    "fixes goroutine leak",
    "goroutine leak",
    "leaking goroutine",
    "leaked goroutines",
]

# "context leak" is the name of the symptom go vet reports; it does not say
# how the leak was fixed.
CONTEXT_LEAK_QUERIES = [
    "fix context leak",
    "context leak",
    "leaked context",
]

SYMPTOMS = {
    "race": (SEED_QUERIES, "other-race", "not-a-race", "a race"),
    "deadlock": (DEADLOCK_QUERIES, "other-deadlock", "not-a-deadlock", "a deadlock"),
    "resource-leak": (RESOURCE_LEAK_QUERIES, "other-leak", "not-a-leak", "a resource leak"),
    "goroutine-leak": (GOROUTINE_LEAK_QUERIES, "other-leak", "not-a-leak", "a goroutine leak"),
    "context-leak": (CONTEXT_LEAK_QUERIES, "other-leak", "not-a-leak", "a context leak"),
}

COMMIT_URL = re.compile(r"github\.com/([^/]+/[^/]+)/commit/([0-9a-f]{7,40})")


def fail(message: str) -> None:
    raise SystemExit(f"mine-fix-commits: {message}")


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


def commit_touches_go(repository: str, sha: str) -> bool:
    """Ask the API which files a commit changed, so a repository whose fix has
    no Go in it is never cloned. Commit search cannot filter by language, and
    most of what it returns is not Go."""
    result = run(["gh", "api", f"repos/{repository}/commits/{sha}",
                  "--jq", "[.files[]?.filename] | .[]"])
    if result.returncode != 0:
        # An unanswerable question is not a no: fall through and let the clone
        # decide, rather than dropping a candidate because the API was busy.
        return True
    return any(name.strip().endswith(".go") for name in result.stdout.splitlines())


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


def replay(checkout: Path, sha: str, check: str, gohawk: Path, timeout: int, tests: bool) -> tuple[str, int, str]:
    """Check out the parent of a fix and run one check over the packages it
    touched. Returns an outcome, the number of findings, and where the first
    few are: a finding in a touched package is not necessarily the fixed
    defect, and the reviewer has to see which it was."""
    packages = changed_go_packages(checkout, sha)
    if not packages:
        return "no-go-files", 0, ""
    if run(["git", "-C", str(checkout), "checkout", "--quiet", f"{sha}^"]).returncode != 0:
        return "no-parent", 0, ""
    build = run(["go", "build", *packages], cwd=checkout, timeout=timeout)
    if build.returncode != 0:
        # An unanalysable revision is not a missed defect, and counting it as
        # one would understate the check.
        return "unbuildable", 0, ""
    flags = ["-enable-checks", check] + (["-gohawk-include-tests"] if tests else [])
    analysis = run([str(gohawk), *flags, "-json", *packages], cwd=checkout, timeout=timeout)
    positions = findings_in(analysis.stdout, checkout)
    return ("reported" if positions else "silent"), len(positions), "; ".join(positions[:5])


def findings_in(output: str, checkout: Path) -> list[str]:
    """Read the positions of the findings from gohawk's JSON, one object per
    package, relative to the checkout."""
    decoder, index, positions = json.JSONDecoder(), 0, []
    root = str(checkout.resolve()) + "/"
    while index < len(output):
        while index < len(output) and output[index].isspace():
            index += 1
        if index >= len(output):
            break
        try:
            payload, index = decoder.raw_decode(output, index)
        except json.JSONDecodeError:
            break
        for analyzers in payload.values():
            for diagnostics in analyzers.values():
                if isinstance(diagnostics, list):
                    positions += [entry.get("posn", "").replace(root, "") for entry in diagnostics]
    return sorted(positions)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", default="lockorder/read-lock-write", help="check, or comma-separated checks, to replay")
    parser.add_argument("--symptom", choices=sorted(SYMPTOMS), default="race", help="kind of fixed defect to seed on")
    parser.add_argument("--include-tests", action="store_true", help="also report findings in test files")
    parser.add_argument("--gohawk", type=Path, required=True, help="gohawk binary to replay with")
    parser.add_argument("--work", type=Path, required=True, help="directory for cached clones")
    parser.add_argument("--out", type=Path, required=True, help="worksheet to write")
    parser.add_argument("--per-query", type=int, default=10, help="commits to take from each query")
    parser.add_argument("--pause", type=float, default=8.0, help="seconds between searches")
    parser.add_argument("--timeout", type=int, default=900, help="per-revision build and analysis timeout")
    arguments = parser.parse_args()

    if not arguments.gohawk.exists():
        fail(f"{arguments.gohawk} does not exist; build it first")
    # Replays run inside each checkout, so a relative path would not resolve.
    arguments.gohawk = arguments.gohawk.resolve()
    arguments.work.mkdir(parents=True, exist_ok=True)

    queries, other_label, unrelated_label, defect = SYMPTOMS[arguments.symptom]
    candidates: dict[tuple[str, str], str] = {}
    for query in queries:
        print(f"searching: {query}", file=sys.stderr)
        for repository, sha, subject in search(query, arguments.per_query):
            candidates.setdefault((repository, sha), subject)
        time.sleep(arguments.pause)

    with arguments.out.open("w", newline="") as sheet:
        sheet.write("repository\tsha\toutcome\tfindings\tpositions\tlabel\tsubject\n")
        for (repository, sha), subject in sorted(candidates.items()):
            if not commit_touches_go(repository, sha):
                outcome, findings, positions, checkout = "not-go", 0, "", None
            else:
                checkout = clone(repository, arguments.work)
                outcome, findings, positions = ("clone-failed", 0, "") if checkout is None else ("", 0, "")
            if checkout is not None:
                try:
                    outcome, findings, positions = replay(
                        checkout, sha, arguments.check, arguments.gohawk, arguments.timeout, arguments.include_tests,
                    )
                except subprocess.TimeoutExpired:
                    outcome, findings, positions = "timeout", 0, ""
            print(f"  {repository}@{sha[:9]}: {outcome}", file=sys.stderr)
            sheet.write(f"{repository}\t{sha}\t{outcome}\t{findings}\t{positions}\t\t{subject}\n")
            # Flush per row so a long run is readable, and survives being stopped.
            sheet.flush()

    print(f"\nwrote {arguments.out}. Label each row before counting:", file=sys.stderr)
    print("  in-class    the parent really does have the defect this check targets", file=sys.stderr)
    print(f"  {other_label}  {defect}, but not this check's shape (counts for prevalence)", file=sys.stderr)
    print(f"  {unrelated_label}  the commit was not fixing {defect} after all", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

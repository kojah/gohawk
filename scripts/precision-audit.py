#!/usr/bin/env python3
"""Scan fresh, pinned repositories without assigning review verdicts.

The precision replay owns checkout and analysis mechanics. This runner adds
bounded parallelism, history deduplication, and restartable per-repository
reports. It never executes repository tests or generation commands.
"""

import argparse
import concurrent.futures
import csv
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent.parent
SPEC = importlib.util.spec_from_file_location("precision_replay", ROOT / "scripts/precision-regression.py")
REPLAY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REPLAY)


def run_scoped(command, **kwargs):
    """Run replay commands in a group so vettool children die on timeout.

    `go vet` may time out while its analyzer still owns the output pipe. Killing
    only the `go vet` process leaves that child analyzing indefinitely, so a
    timed-out module must terminate the entire command group before the replay
    records it as incomplete.
    """
    timeout = kwargs.pop("timeout", None)
    capture_output = kwargs.pop("capture_output", False)
    if capture_output:
        kwargs["stdout"] = subprocess.PIPE
        kwargs["stderr"] = subprocess.PIPE
    grouped = os.name == "posix"
    go_temp = None
    if command[0] == "go":
        # Go normally writes go-build* under the shared GOTMPDIR. A killed
        # go vet cannot remove its work tree, so this command owns one exact
        # directory which the runner can release even after a timeout.
        go_temp = tempfile.TemporaryDirectory(prefix="gohawk-audit-go-", dir="/tmp")
        environment = dict(kwargs.get("env", os.environ))
        environment["GOTMPDIR"] = go_temp.name
        kwargs["env"] = environment
    try:
        with subprocess.Popen(command, text=True, start_new_session=grouped, **kwargs) as process:
            try:
                stdout, stderr = process.communicate(timeout=timeout)
            except subprocess.TimeoutExpired:
                if grouped:
                    try:
                        os.killpg(process.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                else:
                    process.terminate()
                try:
                    process.communicate(timeout=2)
                except subprocess.TimeoutExpired:
                    pass
                finally:
                    # A child may ignore TERM or close its inherited pipes early.
                    # Kill any surviving member before leaving the process group.
                    if grouped:
                        try:
                            os.killpg(process.pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                    else:
                        process.kill()
                    process.communicate()
                raise
        return subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
    finally:
        if go_temp is not None:
            go_temp.cleanup()


REPLAY.run = run_scoped


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
            # checks, verdict, reason. Follow-up reviews add evidence family and
            # status in an eight-column ledger; both keep repository first.
            if fields == ["repository", "revision", "analyzer", "position", "check", "evidence_family", "status", "source_review"]:
                continue
            if len(fields) not in (2, 5, 7, 8):
                raise ValueError(f"{path}: unsupported history row")
            seen.add(fields[1 if len(fields) == 5 else 0].lower())
    return seen


def default_history_paths(root=ROOT):
    """Read selection history, not arbitrary follow-up ledgers with other schemas."""
    cohorts = sorted((root / "benchmarks/precision").glob("round-*/repositories.tsv"))
    audit_dir = root / "benchmarks/precision/audits"
    selections = sorted(
        path for path in audit_dir.glob("*.tsv")
        if path.name == "500-repository.tsv" or re.fullmatch(r"batch-\d+\.tsv", path.name)
    )
    return cohorts + selections


def save_report(path, report):
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    temporary.replace(path)


def cache_policy_sha256(policy):
    canonical = json.dumps(policy, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(canonical.encode()).hexdigest()


def resume_metadata(previous, current, accepted_runner, output, accepted_cache_policy=None):
    """Keep inputs immutable while recording explicit runner/cache transitions."""
    for field in ("repositories", "binary_sha256", "replay_sha256", "go_version", "profile"):
        if previous.get(field) != current[field]:
            raise ValueError(f"saved run has different {field}")
    old_policy = previous.get("cache_policy", {"kind": "shared-default"})
    runner_changed = previous["runner_sha256"] != current["runner_sha256"]
    policy_changed = old_policy != current["cache_policy"]
    if not runner_changed and not policy_changed:
        return previous
    if runner_changed and accepted_runner != previous["runner_sha256"]:
        raise ValueError("runner changed; pass the saved SHA via --accept-prior-runner-sha256")
    if policy_changed and accepted_cache_policy != cache_policy_sha256(old_policy):
        raise ValueError("cache policy changed; pass its saved SHA via --accept-prior-cache-policy-sha256")
    completed = []
    for repo, revision in current["repositories"]:
        path = output / (repo.replace("/", "__") + ".json")
        if not path.exists():
            continue
        report = json.loads(path.read_text())
        if (report["repository"], report["revision"]) != (repo, revision):
            raise ValueError(f"{repo}: saved report does not match pinned input")
        completed.append(repo)
    current["runner_history"] = list(previous.get("runner_history", []))
    if runner_changed:
        current["runner_history"].append({
            "runner_sha256": previous["runner_sha256"],
            "cache_policy": old_policy,
            "completed_repositories": completed,
        })
    current["cache_policy_history"] = list(previous.get("cache_policy_history", []))
    if policy_changed:
        current["cache_policy_history"].append({
            "cache_policy": old_policy,
            "cache_policy_sha256": cache_policy_sha256(old_policy),
            "runner_sha256": previous["runner_sha256"],
            "completed_repositories": completed,
        })
    return current


def analyze(entry, binary, checkouts, output, include_tests=True):
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
        findings, checks, errors = REPLAY.scan(binary, repo, checkout, include_tests)
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


def analyze_bounded(selected, jobs, analyze_one, before_submit=None):
    """Submit at most jobs scans, stopping new work on the first failure."""
    reports = [None] * len(selected)
    with concurrent.futures.ThreadPoolExecutor(max_workers=jobs) as pool:
        pending = {}
        next_index = 0
        while next_index < len(selected) or pending:
            while next_index < len(selected) and len(pending) < jobs:
                if before_submit is not None:
                    before_submit()
                future = pool.submit(analyze_one, selected[next_index])
                pending[future] = next_index
                next_index += 1
            done, _ = concurrent.futures.wait(pending, return_when=concurrent.futures.FIRST_COMPLETED)
            for future in done:
                reports[pending.pop(future)] = future.result()
    return reports


def analyze_with_isolated_cache(selected, jobs, window_size, minimum_free, output, analyze_one):
    """Rebuild in small, drained windows so only this audit's cache is removed.

    Every window owns a fresh Go build cache under the audit output. No cache is
    deleted while a scan uses it; the executor finishes both jobs before the
    TemporaryDirectory exits. A low-space check stops new submissions, and
    the window cleanup then recovers its exact cache even on scan failure.
    """
    previous_cache = os.environ.get("GOCACHE")
    reports = []
    try:
        for start in range(0, len(selected), window_size):
            with tempfile.TemporaryDirectory(prefix="gohawk-audit-cache-", dir=output) as cache:
                os.environ["GOCACHE"] = cache

                def require_space():
                    available = shutil.disk_usage(output).free
                    if available < minimum_free:
                        raise OSError(f"audit root free space {available} below required {minimum_free} bytes")

                reports.extend(
                    analyze_bounded(selected[start:start + window_size], jobs, analyze_one, require_space)
                )
    finally:
        if previous_cache is None:
            os.environ.pop("GOCACHE", None)
        else:
            os.environ["GOCACHE"] = previous_cache
    return reports


def committed_ledger(path):
    """A seal must be the committed audit record, not an unreviewed draft."""
    path = path.resolve(strict=True)
    relative = path.relative_to(ROOT)
    recorded = subprocess.run(
        ["git", "show", f"HEAD:{relative.as_posix()}"], cwd=ROOT, capture_output=True, check=True,
    ).stdout
    if recorded != path.read_bytes():
        raise ValueError(f"{path}: ledger differs from the committed audit record")


def checkout_in_use(checkout):
    """Do not remove a tree referenced by a live local process."""
    if os.name != "posix" or not Path("/proc").is_dir():
        return True
    checkout = checkout.resolve()
    for process in Path("/proc").iterdir():
        if not process.name.isdigit():
            continue
        references = [process / "cwd"]
        try:
            references.extend((process / "fd").iterdir())
        except (OSError, PermissionError):
            continue
        for reference in references:
            try:
                target = Path(os.readlink(reference))
                if target == checkout or checkout in target.parents:
                    return True
            except (OSError, PermissionError):
                continue
    return False


def cleanup_reviewed_checkouts(output, selection, findings, require_committed=True, dry_run=False):
    """Remove exact pinned trees only after every report and finding is sealed.

    The original reports and committed ledgers remain available for a later
    replay. A failed validation removes nothing; a dirty or unpinned checkout
    is skipped rather than treating its contents as disposable audit data.
    """
    if require_committed:
        committed_ledger(selection)
        committed_ledger(findings)
    run = json.loads((output / "run.json").read_text())
    summary = json.loads((output / "summary.json").read_text())["reports"]
    repositories = [tuple(item) for item in run["repositories"]]
    if len(repositories) != len(set(repositories)) or len(summary) != len(repositories):
        raise ValueError("run and summary repository counts differ")
    if any(not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo) or
           not re.fullmatch(r"[0-9a-f]{40}", revision) for repo, revision in repositories):
        raise ValueError("saved run contains an unsafe repository or revision")
    with selection.open(newline="") as source:
        selected = list(csv.DictReader((line for line in source if not line.startswith("#")), delimiter="\t",
                                       fieldnames=["batch", "repository", "revision", "modules", "status"]))
    if [(row["repository"], row["revision"]) for row in selected] != repositories:
        raise ValueError("sealed selection differs from saved run")
    if any(not row["status"].endswith(";reviewed") for row in selected):
        raise ValueError("sealed selection contains an unreviewed repository")
    with findings.open(newline="") as source:
        reviewed = list(csv.DictReader((line for line in source if not line.startswith("#")), delimiter="\t",
                                       fieldnames=["repository", "revision", "analyzer", "position", "checks",
                                                   "verdict", "reason"]))
    reviewed_keys = [(row["repository"], row["revision"], row["analyzer"], row["position"], row["checks"])
                     for row in reviewed]
    if len(reviewed_keys) != len(set(reviewed_keys)) or any(
        row["verdict"] not in ("true-positive", "false-positive", "inconclusive") or not row["reason"].strip()
        for row in reviewed
    ):
        raise ValueError("sealed finding ledger has duplicate or unreviewed findings")
    report_keys = []
    for (repo, revision), recorded_report in zip(repositories, summary):
        report = json.loads((output / (repo.replace("/", "__") + ".json")).read_text())
        if report != recorded_report or (report["repository"], report["revision"]) != (repo, revision):
            raise ValueError(f"{repo}: report differs from saved summary or pin")
        for finding in report.get("findings", []):
            report_keys.append((repo, revision, finding["analyzer"], finding["position"],
                                ",".join(finding["checks"])))
    if set(report_keys) != set(reviewed_keys) or len(report_keys) != len(reviewed_keys):
        raise ValueError("sealed finding ledger does not cover every report finding exactly")

    checkouts = output / "checkouts"
    if checkouts.is_symlink() or not checkouts.is_dir():
        raise ValueError("checkout root is missing or a symlink")
    removed, skipped = [], []
    for repo, revision in repositories:
        checkout = checkouts / repo.replace("/", "__")
        if not checkout.exists():
            continue
        if checkout.is_symlink() or not checkout.is_dir() or checkout.parent.resolve() != checkouts.resolve():
            skipped.append((repo, "unsafe checkout path"))
            continue
        head = subprocess.run(["git", "-C", str(checkout), "rev-parse", "HEAD"], capture_output=True, text=True)
        git_root = subprocess.run(["git", "-C", str(checkout), "rev-parse", "--show-toplevel"],
                                  capture_output=True, text=True)
        dirty = subprocess.run(["git", "-C", str(checkout), "status", "--porcelain", "--ignored",
                                "--untracked-files=all"],
                               capture_output=True, text=True)
        if (head.returncode or head.stdout.strip() != revision or git_root.returncode or
                Path(git_root.stdout.strip()).resolve() != checkout.resolve() or
                dirty.returncode or dirty.stdout.strip()):
            skipped.append((repo, "checkout pin or cleanliness differs"))
            continue
        if checkout_in_use(checkout):
            skipped.append((repo, "checkout is in use"))
            continue
        if not dry_run:
            shutil.rmtree(checkout)
        removed.append(repo)
    return removed, skipped


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path)
    parser.add_argument("--gohawk", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--cleanup-reviewed-checkouts", action="store_true")
    parser.add_argument("--cleanup-dry-run", action="store_true")
    parser.add_argument("--sealed-selection", type=Path)
    parser.add_argument("--sealed-findings", type=Path)
    parser.add_argument("--exclude-ledger", type=Path, action="append", default=[])
    parser.add_argument("--limit", type=positive, default=25)
    parser.add_argument("--jobs", type=positive, default=2)
    parser.add_argument("--isolated-go-cache-window", type=positive)
    parser.add_argument("--min-root-free-gib", type=positive, default=8)
    parser.add_argument("--accept-prior-runner-sha256")
    parser.add_argument("--accept-prior-cache-policy-sha256")
    # Earlier frozen batches reviewed test-file findings; gohawk now skips test
    # files by default, and a fresh batch audits that default.
    parser.add_argument("--include-tests", action="store_true")
    args = parser.parse_args()
    if args.cleanup_reviewed_checkouts:
        if not args.sealed_selection or not args.sealed_findings:
            parser.error("checkout cleanup requires --sealed-selection and --sealed-findings")
        try:
            removed, skipped = cleanup_reviewed_checkouts(
                args.output.resolve(strict=True), args.sealed_selection, args.sealed_findings,
                dry_run=args.cleanup_dry_run,
            )
        except (OSError, ValueError, subprocess.CalledProcessError) as error:
            parser.error(str(error))
        verb = "Eligible" if args.cleanup_dry_run else "Removed"
        print(f"{verb} {len(removed)} reviewed pinned checkouts; skipped {len(skipped)}")
        for repo, reason in skipped:
            print(f"Skipped {repo}: {reason}")
        return int(bool(skipped))
    if not args.manifest or not args.gohawk:
        parser.error("scanning requires --manifest and --gohawk")
    binary = args.gohawk.resolve(strict=True)
    output = args.output.resolve()
    manifest = read_manifest(args.manifest)
    history_paths = default_history_paths()
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
        "profile": " ".join(REPLAY.profile_flags(args.include_tests)),
        "cache_policy": (
            {"kind": "isolated-window", "window_size": args.isolated_go_cache_window,
             "minimum_root_free_gib": args.min_root_free_gib}
            if args.isolated_go_cache_window else {"kind": "shared-default"}
        ),
    }
    run_file = output / "run.json"
    if run_file.exists():
        try:
            metadata = resume_metadata(
                json.loads(run_file.read_text()), metadata, args.accept_prior_runner_sha256,
                output, args.accept_prior_cache_policy_sha256,
            )
        except ValueError as error:
            parser.error(str(error))
    save_report(run_file, metadata)
    # Keep compilation concurrency bounded too; the shared replay sets CGO=0,
    # GOWORK=off, GOTOOLCHAIN=local and -mod=readonly for candidate modules.
    os.environ["GOMAXPROCS"] = "2"
    checkouts = output / "checkouts"
    def analyze_one(entry):
        return analyze(entry, binary, checkouts, output, args.include_tests)

    if args.isolated_go_cache_window:
        reports = analyze_with_isolated_cache(
            selected, args.jobs, args.isolated_go_cache_window,
            args.min_root_free_gib * (1024 ** 3), output, analyze_one,
        )
    else:
        reports = analyze_bounded(selected, args.jobs, analyze_one)
    save_report(output / "summary.json", {"reports": reports})
    return int(any(report["scan_status"] != "scanned" for report in reports))


if __name__ == "__main__":
    raise SystemExit(main())

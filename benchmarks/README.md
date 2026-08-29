# Dogfooding benchmarks

The dogfooding benchmark measures gohawk against pinned revisions of
representative Go repositories. It is intended to produce reproducible
evidence about whole-repository analysis cost, not to enforce a noisy CI
performance threshold.

Run the complete benchmark from the repository root:

```sh
scripts/benchmark-dogfood.sh
```

The script builds gohawk once, checks out each revision from
[`repositories.tsv`](repositories.tsv), warms the Go build and analysis caches,
and then records three measured runs using a repository-owned process
measurement helper. Results are written beneath
`benchmark-results/` and include:

- the exact gohawk and target revisions;
- the Go version and host details;
- package counts;
- wall-clock time and peak resident memory for every run; and
- complete warm-up and measured analyzer output.

The generated `summary.md` is suitable for attaching to the dogfooding
tracking issue. Raw measurements are also available in `results.csv`.

Large repositories can take significant time and disk space. To validate the
harness or measure a single project, use `--only`:

```sh
scripts/benchmark-dogfood.sh --only gohawk --runs 1
scripts/benchmark-dogfood.sh --only caddy --runs 5
```

Pass analyzer selection flags without changing the manifest:

```sh
scripts/benchmark-dogfood.sh \
  --only caddy \
  --gohawk-arg=-enable=globalstate \
  --runs 5
```

Use `--manifest` to supply another tab-separated repository list. Each data
row has four fields: a stable name, Git repository URL, revision, and one Go
package pattern. A repository value of `.` refers to the current gohawk
checkout. Revisions should normally be full commit hashes; `HEAD` is resolved
and recorded before measurement.

The first run for each repository is an unmeasured warm-up. Clone time,
dependency downloads, binary and measurement-helper construction, and
`go list` are deliberately excluded from the measurements. Exit status 3 is
accepted because Go analysis drivers use it when diagnostics are reported;
other nonzero statuses stop the benchmark.

Results from different machines should not be compared as if they came from a
controlled environment. When publishing measurements, retain the metadata and
compare revisions on the same otherwise-idle machine.

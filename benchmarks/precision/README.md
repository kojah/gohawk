# Precision regression cohorts

These cohorts preserve human review from repeatable dogfood runs.
Each repository is pinned to an exact revision. The gate fails if a reviewed
false positive returns or a reviewed true positive disappears.

Run both cohorts with `make precision-regression`, or replay one cohort while
reusing retained checkouts:

```sh
scripts/precision-regression.py benchmarks/precision/round-3 \
  --checkout-root /path/to/checkouts
```

Checkout directories use the `owner__repository` naming convention. The
harness analyzes the three shallowest Go modules, matching the original pilot.
Every replay uses `-enable-all`; labels may therefore cover default or opt-in
checks.
New diagnostics are intentionally reported by normal dogfooding review rather
than guessed to be regressions: only a human can label a new finding.

Round 4 contains the 20 false positives fixed from a 15-repository traced
audit plus all 23 reviewed true positives. The full audit, including the
remaining labeled noise families, lives in the companion lead-generation
study rather than being encoded as accepted benchmark behavior.

Round 5 preserves the first reviewed batch from the 500-repository `-enable-all`
audit. It covers immediately invoked nested cleanup defers, background process
waiters, and direct `os.Process.Wait`, together with nearby resource and process
leaks that must remain reportable. Only repositories contributing a regression
label are retained in the executable cohort; the broader selection ledger is
kept separately from the CI-sized precision gate.

Round 6 preserves the second reviewed batch. It covers channel identity across
direction conversions, map-sized worker joins, `testing/synctest.Test`
ownership, and test-only API declarations together with nearby lifecycle and
resource findings.

Round 7 preserves the third reviewed batch. It covers proven acquisition-error
branches, custom context implementations, named testing callbacks, and
lifecycle ownership transferred through a returned aggregate. Nearby context,
resource, process, goroutine, and loop-cleanup findings remain reportable.

Round 8 preserves the fourth reviewed batch. It covers cancellation stored in
an enclosing lifecycle, deferred cleanup callbacks, ambiguous `Row` names,
returned testing callbacks, and stop channels received through static helpers.
Nearby resource, process, testing, context, and goroutine findings remain
reportable.

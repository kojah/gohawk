# Precision regression cohorts

These cohorts preserve the human review from two 20-repository dogfood runs.
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
New diagnostics are intentionally reported by normal dogfooding review rather
than guessed to be regressions: only a human can label a new finding.

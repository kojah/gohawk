# Batch 56 corrected-replay receipt

The [original finding review](batch-56-findings.tsv) records what the frozen
`7305cc9` analyzer emitted and the source verdict for every site. This receipt
does not change those verdicts. It measures one later analyzer build against
the 69 original `nilargument/dereferenced-nil` false positives (after the
documented Go-Kratos label correction).

The [machine-readable replay](batch-56-nilargument-replay.json) records all
16 affected pinned packages, their original positions, remaining positions,
emitted diagnostics, exit statuses, and errors. A unique gohawk binary built
from committed `5b2758d8a48376f162fb790dbdf66aeb69c42502` had SHA-256
`bbc695d10e57e4e084f216a2ad250cec716f10ed562e779464086e1389f599e1`.
That commit already included fixes through `c795215`; it is not the frozen
baseline binary. Each package was statically reanalyzed with
`-enable-checks=nilargument/dereferenced-nil -gohawk-include-tests -json`.
All 16 scoped runs exited successfully without timeout or parse error.

| Original FP sites | Removed | Remaining | New sites |
| ---: | ---: | ---: | ---: |
| 69 | 58 | 11 | 0 |

The remaining 11 are six monkey-patched Bifrost test sites, one go-sse
intentional-panic test, and four gopatch intentional-panic test sites. Their
exact keys and output messages are in the replay. This check still had **zero
true positives** in the batch-56 original source review. The replay does not
establish precision across all packages, a broader candidate scan, or any
new coverage claim. Experimental status is unchanged pending later batches.

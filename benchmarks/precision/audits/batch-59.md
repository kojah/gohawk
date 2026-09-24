# Batch 59: fourth frozen overnight audit tranche

This completes the fourth 250-repository tranche of the September 24
1,000-repository audit. The [selection receipt](batch-59-selection.json),
[scan report](batch-59-scans.json), [repository ledger](batch-59.tsv), and
[per-finding source review](batch-59-findings.tsv) preserve exact pins,
coverage status, diagnostic keys, and source-backed verdicts. The
[four-batch overview](overnight-2026-09-24-1000.md) aggregates the whole
cohort. Selection used the documented 200–5,000-star expansion after
deduplication against earlier audit cohorts.

The analyzer remained the unchanged `7305cc9` binary (SHA-256
`576caba104c1f436c3791268e2c6cea40e3585315a34fa84363f458a947ba5ec`)
with `-enable-all -gohawk-include-tests -json`. The runner hash was
`ba3ec8504f2b1421f2bb5a37ea4039581f6b3e9b4106f04f58fb2b318bfa82f3`;
it used two scan jobs, isolated four-repository Go build-cache windows,
task-owned temporary Go directories, and a 12 GiB root-free submission
guard. Candidate tests, generators, and applications were not executed.

All 250 pinned repositories were attempted across 297 module entries:
193 complete and 57 incomplete scans, with no failed repository report.
Of the complete scans, 124 emitted no findings. Incomplete scans retain
their exact errors and emitted partial findings; they are **not** counted
as clean. Every one of the 721 emitted findings was reviewed against pinned
source: 575 true positives and 146 false positives. These are judgments
of reported policies, not runtime reproductions or a recall measurement.

| Analyzer | TP | FP |
| --- | ---: | ---: |
| `resourcelifetime` | 499 | 60 |
| `nilargument` | 0 | 59 |
| `goroutineownership` | 46 | 3 |
| `lockorder` | 7 | 12 |
| `processownership` | 12 | 2 |
| `cancellationownership` | 4 | 6 |
| `deferinloop` | 3 | 3 |
| `concurrentcapture` | 2 | 0 |
| `producerlifecycle` | 2 | 1 |

One repository, `twitchdev/twitch-cli`, contributes 257 resource findings.
Source review distinguishes 240 responses whose bodies were abandoned
from 16 exact HTTP 204 No Content responses (for which Go exposes
`http.NoBody`) and one deliberately malformed-URL request that acquires
no response. This concentration is visible in the finding ledger; it must
not be mistaken for 257 independent bug families. Other source-backed
false-positive families include Fiber's in-memory `App.Test` responses,
bodyless test-server responses, a transaction released through an
unmodeled helper, and intentional process-terminal signal registrations.
The experimental `nilargument` check again made only false definite-nil
claims in this tranche; this batch does not justify promoting it.

The scan receipt records the fixed binary/profile/pins separately from
later analyzer changes. A finding's original verdict is not rewritten by
a future corrected replay. Follow-up priorities, including automatic
cleanup of sealed checkouts, are listed in the four-batch overview rather
than counted as completed analyzer corrections here.

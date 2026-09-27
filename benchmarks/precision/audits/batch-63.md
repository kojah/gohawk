# Batch 63: 250 fresh repositories, stars 250–299

Batches 62 and 63 were audited together as one 500-repository round with the
`f8c461f` binary (SHA-256
`4b5e83ee699d431e060884d410c434579055621d0b5839991ca49ea77e6e38ea`). That
binary includes the batch-61 fixes (nil selections in the points-to model and
unread done channels in `goroutineownership`) and the retirement of
`use-after-release`, `recursive-acquire`, `channelsafety`, and the four
experimental `producerlifecycle` checks. The [selection receipt](batch-63-selection.json),
[scan report](batch-63-scans.json), [repository ledger](batch-63.tsv), and
[per-finding source review](batch-63-findings.tsv) preserve the exact pins,
coverage status, diagnostic keys, and source-backed verdicts. Batch 62 is
the other half of the round.

Selection used the batch-61 criteria in the 250–299 star band: 281
candidates were reviewed to pin 250, after deduplication against every
earlier cohort. The profile was `-enable-all -gohawk-include-tests -json`
with two scan jobs. The first 118 repositories were scanned with the
shared Go build cache; after it filled the disk, the scan resumed with
isolated four-repository cache windows, which the scan report's cache policy
records. Findings from both parts use the same binary and pins.
Candidate tests, generators, and applications were not executed.

All 250 repositories were attempted across 320 module entries:
191 scans completed and 59 were incomplete. Of the complete
scans, 113 emitted no findings. Incomplete scans keep their exact errors
and any partial findings, and they are **not** counted as clean. Every one of
the 412 emitted findings was reviewed against pinned source: 350 true
positives, 62 false positives. These are judgments of the reported
policies, not runtime reproductions or a recall measurement.

**Default profile (production files): 143 TP, 35 FP (80% precision).** Test
files, reviewed because the audit profile adds `-gohawk-include-tests`: 207
TP, 27 FP (88%). A default run does not analyze test files, so the
production figures are what a user of the default profile sees.

| Analyzer | Production TP | Production FP | Test TP | Test FP |
| --- | ---: | ---: | ---: | ---: |
| `resourcelifetime` | 96 | 18 | 182 | 26 |
| `goroutineownership` | 23 | 7 | 23 | 1 |
| `lockorder` | 11 | 7 | 0 | 0 |
| `processownership` | 5 | 3 | 1 | 0 |
| `concurrentcapture` | 2 | 0 | 0 | 0 |
| `deferinloop` | 3 | 0 | 1 | 0 |
| `producerlifecycle` | 1 | 0 | 0 | 0 |
| `cancellationownership` | 2 | 0 | 0 | 0 |

The round's false-positive families are listed with [batch 62](batch-62.md#false-positive-families-batches-62-and-63).

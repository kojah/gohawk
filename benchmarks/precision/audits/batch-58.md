# Batch 58: third frozen overnight audit tranche

This is the third 250-repository tranche of the September 24 target of 1,000
fresh pinned Go repositories. The [selection receipt](batch-58-selection.json),
[scan report](batch-58-scans.json), [repository ledger](batch-58.tsv), and
[per-finding source review](batch-58-findings.tsv) preserve the full commit
pins, scan status, original diagnostic keys, and evidence for every verdict.
The fresh discovery pool expanded to 200–499 stars after the 500–5,000-star
Go 1.20+ pool had only 53 unused pins. The selection receipt records filters,
GitHub search pages, exclusions, and manifest hashes.

The analyzer remained the unchanged `7305cc9` binary (SHA-256
`576caba104c1f436c3791268e2c6cea40e3585315a34fa84363f458a947ba5ec`)
with `-enable-all -gohawk-include-tests -json`. Analysis was static-only:
candidate tests, generators, and applications were not executed. Scanner
tooling changed only for process cleanup, task-owned Go temp directories,
bounded submissions, and isolated build-cache windows. The scan receipt's
runner and cache-policy histories identify the 46 reports saved under runner
`e2c7e5d9`, 85 more under `45340e98`, 15 under `dea523ea` with two-repository
cache windows, and the final 104 under `ba3ec850` with four-repository
windows and a 12 GiB root-free submission guard. Two jobs were used except
the initial one-job isolated-cache probe. No saved report was reclassified by
a scanner transition; later analyzer fixes do not change the frozen baseline.

All 250 pinned repositories were attempted across 318 module entries. There
were 174 complete and 76 incomplete repository scans. Of the complete scans,
96 emitted no findings; incomplete scans are not counted as clean. Every one
of the 467 emitted findings was reviewed against pinned source: 317 true
positives and 150 false positives. These are source judgments of reported
policies, not runtime reproductions or a recall measurement.

| Analyzer | TP | FP |
| --- | ---: | ---: |
| `resourcelifetime` | 203 | 22 |
| `nilargument` | 0 | 88 |
| `goroutineownership` | 57 | 15 |
| `lockorder` | 8 | 13 |
| `processownership` | 20 | 4 |
| `concurrentcapture` | 6 | 0 |
| `deferinloop` | 13 | 4 |
| `cancellationownership` | 3 | 4 |
| `producerlifecycle` | 7 | 0 |

The clearest precision concern is again experimental `nilargument`: all 88
reports in this tranche are false definite-nil claims. Families include
initialized test receivers, GORM schema access after successful parse, and
embedded fields initialized before a promoted method call. Keep the original
verdicts in this record; no promotion or broader possible-nil claim is
justified by this batch.

Other recurring false-positive boundaries were bounded test-loop defers,
intentionally failed resource acquisitions, explicit transfer to returned
objects or framework hooks, a process reaped by a dedicated waiter, and
descriptors created as a program's final action before exit. The source
review records each boundary and retains nearby true-positive controls.
The ten TiKV lockorder false positives were removed by the later
lock-identity correction `60e8e61`, with Dify's two real missing-release
controls retained in a separate corrected replay; that does not rewrite the
frozen labels here.

Incomplete scans retain their exact dependency, toolchain, or timeout errors
and any findings emitted by packages that did finish. Large-module Go vet
timeouts were allowed to finish naturally, and task-owned cache windows were
drained and deleted between groups to avoid filling the shared root disk.
No incomplete repository is counted as finding-free merely because some
packages produced no diagnostics.

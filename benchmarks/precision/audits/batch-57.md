# Batch 57: second frozen overnight audit tranche

This is the second 250-repository tranche of the September 24 target of 1,000
fresh pinned repositories. The [selection receipt](batch-57-selection.json),
[scan report](batch-57-scans.json), [repository ledger](batch-57.tsv), and
[per-finding source review](batch-57-findings.tsv) preserve provenance, full
commit pins, scan state, and every original finding key.

The scan used the unchanged gohawk `7305cc9` binary (SHA-256
`576caba104c1f436c3791268e2c6cea40e3585315a34fa84363f458a947ba5ec`),
runner SHA-256
`b1b854aa65ff2a353560340e3c5bea5ab27854981dc4abbd50a934e897579b2d`,
`-enable-all -gohawk-include-tests -json`, and two concurrent scan jobs.
Analysis was static-only; candidate tests, generators, and applications were
not executed. Later analyzer fixes do not change this frozen baseline.

All 250 pinned repositories were attempted across 304 module entries. There
were 205 complete scans and 45 incomplete scans. Of the complete scans, 123
emitted no findings; incomplete scans are not counted as clean. Every one of
the 314 emitted findings was reviewed against pinned source: 199 true
positives and 115 false positives. These are source judgments of reported
policies, not runtime reproductions or a recall measurement.

| Analyzer | TP | FP |
| --- | ---: | ---: |
| `resourcelifetime` | 144 | 19 |
| `nilargument` | 2 | 81 |
| `goroutineownership` | 26 | 3 |
| `lockorder` | 9 | 2 |
| `concurrentcapture` | 8 | 1 |
| `processownership` | 6 | 1 |
| `cancellationownership` | 3 | 1 |
| `deferinloop` | 1 | 7 |

The main precision concern remains `nilargument`: 81 false positives include
Clivern/Beaver's 24 Redis clients initialized by `Init`, a test-only group of
eight pointers initialized by `Get` in peterbourgon/ff, and six initialized
Jenkins receivers. The two true-positive controls are a definitely nil RTP
packet receiver in peer-calls and a zero-valued logger used on Monibuca's
error branch. This evidence supports keeping the check experimental pending
correction and later batches, not broadening its possible-nil claims.

Other recurring false-positive contracts include fixed-size defer loops,
intentionally failed resource acquisitions, ownership transferred to a
returned object, and reversed inner lock order serialized by a shared outer
mutex. The [finding review](batch-57-findings.tsv) gives exact source evidence
and nearby controls for each case. It preserves original labels separately
from any later corrected replay.

One scan-stability issue repeated: `apache/incubator-seata-go` timed out in a
dependency package while its frozen analyzer child exceeded 22 GiB RSS and
survived the timed-out `go vet` parent. The exact orphan was terminated after
its repository report was persisted as incomplete, with zero findings. The
other incomplete scans retain their errors and any findings emitted by
packages that did finish; none imply an absence of diagnostics.

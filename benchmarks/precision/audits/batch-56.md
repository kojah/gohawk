# Batch 56: frozen overnight concurrency and lifecycle audit

This is the first 250-repository tranche of the September 24 overnight audit.
The original scanner used the unchanged `7305cc9` gohawk binary (SHA-256
`576caba104c1f436c3791268e2c6cea40e3585315a34fa84363f458a947ba5ec`)
with `-enable-all -gohawk-include-tests -json`, two concurrent scan jobs, and
static-only, read-only module analysis. Candidate tests, generators, and
applications were not executed. The [selection receipt](batch-56-selection.json),
[scan report](batch-56-scans.json), [repository ledger](batch-56.tsv), and
[per-finding review](batch-56-findings.tsv) preserve full pins and provenance.

All 250 pinned repositories were attempted across 292 module entries. There
were 197 complete scans, 52 incomplete scans, and one failed checkout. Only
116 complete scans had no findings; incomplete and failed scans are not counted
as clean. Every one of the 381 emitted findings was reviewed against its pinned
source: 278 true positives, 100 false positives, and three inconclusive.
These are source judgments about the reported policy and feasible paths, not
runtime reproductions or a recall measurement.

Post-record correction (September 24): the original record labeled
`go-kratos/gateway@a553bef5c1a8cc03ae3d114e5e578c51b8ac06a0`
`proxy/condition/condition.go:101:14` (`nilargument/dereferenced-nil`) a true
positive. Reviewing the exact diagnostic changed that verdict to false positive:
it asserts a definitely nil `Condition_ByHeader.ByHeader`, but the type-switch
value comes from caller input and can contain a nonnil field. A malformed value
can still panic; it does not establish the reported definite-nil claim. The
finding key is unchanged, and the totals above include this one-label correction.

| Analyzer | TP | FP | Inconclusive |
| --- | ---: | ---: | ---: |
| `nilargument` | 0 | 69 | 0 |
| `resourcelifetime` | 172 | 21 | 1 |
| `lockorder` | 30 | 0 | 0 |
| `goroutineownership` | 22 | 2 | 2 |
| `concurrentcapture` | 19 | 4 | 0 |
| `cancellationownership` | 15 | 0 | 0 |
| `deferinloop` | 10 | 3 | 0 |
| `processownership` | 8 | 1 | 0 |
| `producerlifecycle` | 2 | 0 | 0 |

The largest false-positive family is field nilness across helpers and returned
aggregates: LinDB contributes 36 `nilargument` alerts on addresses of embedded
fields; Crewjam's certificate helper, Cortile's manager constructor, and
Quasilyte's environment constructor show related gaps. These original verdicts
remain in the ledger even where later, separately verified analyzer changes
remove a report. Other recurring false positives include intentional panic
tests, deliberately failed resource acquisitions, transferred HTTP/file owners,
and a serial worker pool misread as concurrent. Several reports require no
new general model: a three-entry test table retaining two or three file handles
is not an actionable unbounded defer-in-loop hazard.

Nearby true positives constrain those decisions. LinDB has genuine shared
capture findings; Cortile leaves successful HTTP response bodies open;
Gocopper's first log file leaks if opening the second fails, even though the
success path transfers it into a returned logger; Bifrost has lost cancel
functions, mutex returns without unlock, and an unclosed HTTP body alongside
its monkey-patched-test nil alerts. The complete evidence and uncertainty for
each site are recorded per location in the finding ledger.

The three inconclusive sites are two timeout-based worker-completion reports
and one long-lived external logger/file handoff. Finite worker termination is
not assumed to satisfy a caller's join obligation, and a logger-shaped wrapper
is not assumed to close an external file without a proven contract.

Scan stability also needs follow-up. `scratchdata/scratchdata` timed out in its
root module after an analyzer child grew above 40 GiB and survived its parent;
the exact orphan was terminated after the parent timed out. `esimov/pigo` had
a `github.com/golang/freetype/truetype` goroutine-stack overflow in static
analysis. `Sliverkiss/workbuddy2api` failed to fetch its pinned revision.
These repositories retain incomplete/failed status, not inferred negative
results. The scan report preserves their errors and any findings from packages
that did finish.

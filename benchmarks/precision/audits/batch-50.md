# Batch 50: 250-repository static precision audit

## Result and scope

All 250 exact-SHA repository selections received a bounded scan attempt. Across
319 selected module entries, 195 repository scans completed and 55 were
incomplete. Every one of the 444 reported findings was reviewed against source:
363 true positives, 55 false positives, and 26 inconclusive. Findings occurred
in 111 repositories, including partially analyzed repositories. Another 102
repositories completed the selected scan scope without diagnostics.

These are review labels for the frozen original analyzer, not claims that every
finding is high-impact, accepted upstream, or corrected. Bounded-loop lifetime
extensions, missing test cleanup, and production leaks remain distinguished in
the individual reasons. Inconclusive findings are neither silently accepted as
bugs nor counted as confirmed false positives.

No candidate tests, generators, applications, benchmark programs, or build hooks
were executed. Candidate Go packages and test source were loaded by `go vet`
with the pinned gohawk analyzer, with CGO disabled, module files read-only, and
toolchain auto-download disabled. The audit's local analyzer regression tests
and corrected-binary source replays are separate from candidate execution.

The selection is new relative to prior audit ledgers, cohorts, in-flight
manifests, and recorded outreach. Root Go directives are compatible with local
Go 1.27.0 and target Go 1.26/1.27. The selected star range is 500–1,197, within
the established 500–5,000 boundary; repository size is below 30,000. Discovery
checked 1,156 fresh candidates, found 258 eligible, and pinned the first 250.

## Evidence

- [Selection ledger](batch-50.tsv): all exact pins and final scan/review status.
- [Selection provenance](batch-50-selection.json): search partitions, metadata,
  Go directives, and deduplication inputs.
- [Original scan metadata](batch-50-scans.json): binary/runner hashes, profile,
  module scope, errors, and every original diagnostic key.
- [Individual reviews](batch-50-findings.tsv): exact SHA, analyzer, position,
  check identifier, verdict, and source-based reasoning for every finding.

The baseline binary was built from `6ed616c240f9bd89eb0f32069bbef9c96ef17b8b`,
SHA-256 `7c5d74bc1a4e50d930e18533bf3fbf441f43839a413d5a4746754522090fc267`.
Runner revision was `4b91ed90dca707cec75bb36f8441be05a75e730a`. It was not
hot-swapped as fixes arrived. Four repository scans ran concurrently, with up
to three modules per repository and a 180-second bound per analysis command.

| Analyzer | True positive | False positive | Inconclusive |
| --- | ---: | ---: | ---: |
| cancellationownership | 8 | 2 | 2 |
| concurrentcapture | 12 | 7 | 0 |
| deferinloop | 11 | 1 | 0 |
| evalorder | 7 | 0 | 0 |
| goroutineownership | 7 | 6 | 2 |
| lockorder | 21 | 12 | 3 |
| processownership | 30 | 5 | 5 |
| producerlifecycle | 22 | 1 | 2 |
| resourcelifetime | 245 | 21 | 12 |

This is a selected, partly incomplete audit cohort, not an unbiased population
precision estimate or proof that zero-diagnostic repositories contain no bugs.

## Generalizable gaps

The main recurring issues are evidence propagation, not missing library-name
exceptions:

- **Owned wrappers and mutable owner fields.** A wrapper can borrow an already
  managed connection rather than acquire a new obligation. Cleanup registered
  before a field assignment may later read that updated field. Returned owners,
  branch-merged field stores, connection-handle maps, and process collections
  require distinct, bounded proofs. ARTEX, kluctl, artifact-fs, and fence expose
  these differences; successful transfer alone must not erase early-error leaks.
- **Completion versus readiness.** A startup signal is not worker completion.
  Conversely, `t.Cleanup(wg.Wait)` registered before launch is still a join.
  Nil arguments can make an otherwise visible channel operation infeasible.
  A `Done` before a deferred private file close, as in ledger, remains
  inconclusive rather than being treated as a plainly missing wait.
- **Iteration-sensitive identity.** A channel receive can produce a different
  pooled mutex on every iteration. Conflating those receives creates recursive
  lock warnings. This must not suppress a real loop that reacquires one stable
  mutex. Nylon also retains genuine opposite-order locks and an inconclusive
  cross-peer order, so whole-repository suppression is inappropriate.
- **Path and worker correlation.** Sake uses a capacity-one semaphore exactly
  when step mode accesses its shared flag. Metatube launches exactly two workers
  whose distinct indices select separate output variables. Goshs rejects
  read-only creation before acquisition, making one later denial infeasible,
  while the same denial is a real leak for another open path.
- **Protocol and parent cleanup.** Transaction rollback can close query rows;
  raw descriptor wrappers can release an original file; HTTP body transfer can
  settle an obligation. Finite proposal/ack protocols need more than generic
  channel send counting. These relationships should not be guessed from names.
- **Intent and actionability.** Daemon launches and process-lifetime logger
  sinks are not automatically bugs. Empty HTTP bodies can be non-actionable,
  but Client.Timeout wrappers can retain cancellation resources even for an
  empty response, as the Pinniped review demonstrates.

## Corrections remain separate from the baseline

Streaming assessment produced bounded fixes for borrowed-wrapper acquisition,
nil completion channels, readiness mistaken for completion, and fresh
channel-received mutex identity. Focused corrected replays removed the reviewed
SMTP wrapper finding, the iron-proxy nil-channel finding, two autoscaler
readiness findings, and six of seven Nylon recursive-lock findings. The four
Nylon contradictory-order diagnostics were retained. Original rows and counts
above intentionally remain unchanged.

A process-ownership traversal stack overflow was also reproduced on a dependency
of kiwifs and addressed with cycle protection. This is a scan robustness fix,
not a false-positive verdict. The corrected dependency replay completes with
one diagnostic and no stderr; the affected kiwifs packages retain both original
true positives. The pre-launch test-cleanup join fix removes changie's false
positive while preserving its eight true positives.

In total, **11 original false positives have verified removals; 44 remain
unresolved**. This is targeted replay evidence, not a complete corrected census.
Broader owner/collection/protocol proofs remain deferred.

Implementation commits are `5c0bebc` (wrapper and completion evidence),
`e3d1ac8` (loop-received lock identity), and `011e287` (cyclic owner traversal
and pre-registered test cleanup). Final `make verify` passed in an isolated
snapshot of these changes: module verification, formatting, generated docs,
vet, dead-code check, lint, self-analysis, all gohawk unit/architecture tests,
and the tracing component's race tests. Candidate tests were not executed.

| Corrected binary | SHA-256 | Static replay evidence |
| --- | --- | --- |
| v1 | `a80cf5c036d30a7b351f5328c0e5d8b74489aab9dfabfc34503d97c80d8352fb` | SMTP FP removed; iron-proxy FP removed; go-judge's three TP controls preserved |
| v2 | `4ab309d3a1d8d9fb23cbfe05c1447ee66345bd8e7b8512d5a25971fc4a1e4e9e` | Both autoscaler readiness FPs removed on completed warmed replay; first cold attempt timed out |
| v3 | `94dd157246cb72ef7339afc8d98378dded3edb886c5a1082599a2555f262b30e` | Nylon recursive findings 7 to 1; all four contradictory-order findings preserved |
| v4 | `e8e2ecb2ba8f1ca65bb2cfa464a2b432328ea2d5304d9ea5a8bab56c0330c120` | swag dependency no longer crashes; kiwifs's two process TP controls preserved |
| v5 | `a2c8a5f96a20e3da6dbe4c0237a18504e58bbe0758eeb1a0492f6579b363a0fd` | Changie cleanup FP removed; all eight TP controls preserved |

These binaries were built incrementally while fixes were under review; their
hashes identify the actual replay executables. They are not release artifacts
or clean-revision precision stamps. Exact candidate revisions remain in the
selection ledger. The dependency crash reproduction used swaggo/swag v1.16.6,
as pinned by kiwifs's module requirements.

## Operational limitations

Incomplete scans include dependency/package failures, partial package recovery,
toolchain/module incompatibility, timeouts, and the original traversal crash.
Valid diagnostics emitted alongside failures were still individually reviewed;
such repositories were not relabeled as fully scanned or clean. The runner's
nonzero final exit reflects these incomplete attempts.

Timeouts exposed a process-tree issue: terminating the `go vet` parent could
leave analyzer children running. Verified orphan processes belonging to this
batch's exact binary were terminated; unrelated processes were untouched. A
future runner should terminate the whole owned process group. The frozen runner
was not changed midway through the cohort.

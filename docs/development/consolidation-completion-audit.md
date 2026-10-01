# Consolidation completion audit

Beads `gohawk-dho.23` tracks this audit against the original objective:
clean architecture, no duplicated proof mechanics, and all remaining easy
false positives addressed. Completion is still unproven. The source base for
this review is `3524977`, followed by the scalar/type/loop/evidence
consolidation tracked in `gohawk-dho.24`.

## Requirements and evidence

| Requirement | Current evidence | Status and remaining verification |
| --- | --- | --- |
| Shared responsibilities and downward dependencies | The architecture guide names the SSA, heap, lifecycle, resource and summary layers. Repository-wide architecture tests enforce imports, analyzer layout, fact ownership, summary access, traversal and reporting boundaries. | Structural conformance has passing canonical receipts. It does not prove every proof policy is cohesive; semantic review remains open. |
| No duplicated proof mechanics | The value-walk and storage/summary reviews document concrete merges and distinct policies. The broader normalized-body scan below found five additional mechanical duplicate groups, now consolidated. | The identified groups are addressed. Partial blocks and equivalent logic written differently remain outside that scan; no repository-wide absence claim is made. |
| One authoritative decision for each check | Reviewed lifecycle analyzers classify once and ask a shared flow query. The remaining normalized matches in classifier caches and state keys contain different domain state, rather than second acceptance rules. | The ten-check inventory below locates each reporting pipeline. Concurrent capture and process ownership now share their final decisions with tracing. Lock reporting still contains inline multi-rule policy; deeper classification review remains open. |
| Remaining easy FPs fixed | Frozen batches 62/63 supplied 55 production FP locations. The queue refresh, successful 22-site replay and four subsequent corrections leave 18 unresolved production sites. The ten-family assessment states the missing evidence for each. Rune callback wait is separately corrected; fresh-lock publication remains open. | No target-resolution-only correction has been demonstrated. The remaining families need policy, identity, state or protocol evidence. Further current-source review must distinguish a newly available bounded fix from a genuinely larger model. |
| Precision preserved by consolidation | Parent/current boundary comparisons, accepted and diagnostic fixtures, ordinary tests and local dogfood scans accompany focused changes. Existing facts retain must/may polarity, exact binding, observation time and bounded unknown outcomes. | Passing receipts prove their stated scopes. They do not certify every historical finding against the latest source. Use affected pinned cases when behavior changes; do not rewrite frozen labels or credit unscannable cases. |
| Tight development cycle | Focused tests precede stable `make verify` gates. The local gate runs ordinary tests, formatting, vet, lint, dead-code and local dogfood; it does not invoke precision-regression. | Maintained. No full precision replay or local race run is part of these iterations. |
| Work tracked and own changes published | Beads children record corrections, reviews and unresolved questions. Implementation commits contain exact task paths; unrelated staged deletion and working-tree artifacts remain separate. | Verify commit, upstream synchronization and child closure after each implementation. The epic and this completion audit remain active. |

## Broader duplicate-candidate scan

A local Go AST/token scanner examined 266 production Go files and 1,992
function bodies under `internal/` at the source base. It excludes tests,
generated source, fixture/testdata/vendor trees and dot/underscore trees,
matching the repository's source-inventory exclusions. It compares complete
function bodies of at least 35 tokens after normalizing identifier names;
string/numeric literals, operators and keywords remain distinct. Predeclared
identifiers such as true, false and nil are normalized too. It is not
type-aware and supplies candidates, not proof of equivalent semantics.

All ten resulting groups were inspected in source. The local artifacts are
`.build/goal-duplicate-functions.go` and
`.build/goal-duplicate-functions.json`; these are review aids, not a new gate.

| Candidate group | Reviewed disposition |
| --- | --- |
| `storageInteger` / `StorageInteger` | Heap slice bounds now use `ssaflow.StorageInteger`; omitted-bound defaults and exact integer conversion remain unchanged. |
| `addressedStruct` / `structBehind` | `syntax.PointerStruct` owns the one-pointer underlying-struct shape. The other copies of this shape check use it too, retaining domain identity and ownership checks. Named pointers and aliases remain supported; value structs and nested pointers remain outside this shape. |
| `panics` / `endsInPanic` | `ssaflow.BlockEndsInPanic` owns the last-instruction check. Concurrency inference retains its separate recovery and return-witness rules; an explicit panic does not prove whole-function termination. |
| `dominatesLatches` / `dominatesBackEdges` | `NaturalLoop.DominatesBackEdges` owns the predecessor dominance query. Its guarantee excludes break/exit paths; it is not a claim that the block runs on every possible loop path. |
| `lockEvidence` / `Evidence` | Lock acquisition and helper-route spans now use `check.Evidence`; source ranges and labels retain the existing behavior. |
| Resource flow key / obligation flow key | Each key includes its own obligation state and path guards. Both use the shared work-list driver; replacing domain keys with one generic obligation would lose resource state. |
| Spawn/resource classifier cache adapters | Both memoize their authoritative classifier and trace its result. Action and reason types, evidence families and reporting rules differ; there is no second classifier decision in these adapters. |
| Field/global/enclosing-scope stores | Destination-specific leaf queries. Field/global stores ask possible alias; enclosing-scope stores ask derivation. Their provenance mechanics are already shared, and the distinct queries must not silently acquire one another's policy. |
| Spawn/resource budget adapters | Each lazily owns a candidate pool, chooses its domain limit and observer, and delegates charging to `SearchBudget.Within`. The pool mechanics are shared; domain limits and candidate ownership remain local. |
| Lock handoff / returned owner | Lock handoff compares arguments with alias or derivation; returned ownership compares results with alias or aggregate containment under an ownership search. Identifier normalization hides that material difference. |

The post-change scan sees 1,987 bodies in the same 266 files and retains the
five justified domain-adapter groups above. This proves only the disposition
of this candidate set. It does not find partial duplication, differently
structured equivalent predicates, small bodies, or code outside `internal/`.

## Final-decision mismatch found during catalog review

At `015bc08`, `processownership.reportStartedCommand` emitted its final trace
before its existing `commandUnusedAfterStart` suppression. The browser-launch
fixture was correctly silent but traced `unowned-return` as rejected. A focused
trace regression reproduced this mismatch on the parent.

Beads `gohawk-dho.25` moves that final suppression into `decideProcessReturn`.
Its structured state and reason are consumed by reporting and tracing: an
uncovered unused command is unknown, a reportable unowned return is proven,
and a fully covered flow retains its accepted/ambiguous outcome. The command
use query still runs only after an uncovered return is found, preserving the
existing query order and diagnostic boundary. Earlier pre-Start suppression
paths remain separate; this does not claim complete tracing coverage.

The focused analyzer tests cover exactly one unknown browser-launch decision
and retain the accepted/rejected/opaque-handoff assertions. Parent/current
fixture JSON and the traced invocation are recorded under
`.build/goal-process-decision-*`; they compare diagnostic output independently
of the corrected trace. Both `-enable-all -json` fixture invocations exit 3
with empty stderr and identical nonempty diagnostic JSON (31,707 bytes).
The final traced receipt is `.build/goal-process-decision-all.trace.jsonl`;
the earlier invocations without all checks produced empty output and are not
used as equivalence evidence. Canonical `make verify` passes, including the
ordinary suite (54 seconds), formatting, vet, lint, dead-code and local
dogfood (`.build/goal-process-decision-verify.log`). No full precision corpus
or local race run was performed. No audited FP removal is credited.

This finding confirms why a complete decision-owner review is still required
even after duplicate-body consolidation.

## Catalog decision-owner inventory

The current catalog contains eight analyzer packages and ten check IDs in
`analyzers/catalog_specs.go`. Source inspection at `7e8421f`, followed by the
concurrent-capture consolidation in `gohawk-dho.26`, locates these reporting
pipelines. Production report-call searches excluded tests and testdata; each
listed reporter and decision entry was read directly. Graph tools are
unavailable, so this is a source-based inventory, not a graph completeness
claim or a review of every transitive predicate.

| Check | Decision owner and reporting boundary | Remaining architecture question |
| --- | --- | --- |
| Concurrent capture | The AST collector selects repeated writes to outer locals, then `captureEvidence.proveMutation` returns the guard decision consumed by reporting and tracing. | The former inline four-rule switch is consolidated. Syntax candidate gates remain bounded suppressions; their semantic contracts still need review. |
| Goroutine join | `spawnAnalysis.prove` combines obligation discovery, one obligation walk and conservative post-walk boundaries into `GoroutineProof`. The entry reports only `GoroutineLifecycleViolated`. | Classification and discovery helpers need the remaining partial-duplication review; the final reporter does not independently select suppressions. |
| Cancellation release | `proveCancellation` converts `EvaluateObligationWitness` into `CancellationProof`; reporting and final tracing consume the same outcome. | Parent and deferred-cell classifiers remain distinct evidence families; their shared mechanics need review rather than a generic cancellation exemption. |
| Resource release | `evaluateResourceFlow` owns acquisition boundaries, resource-state walking and final policy result. `checkAcquisition` traces that result and reports only its report flag. | The pre-flow memory-writer gate and post-flow policy rules belong in the remaining cohesion review. Resource state is richer than a generic join lattice. |
| Process wait | Pre-Start ownership gates select local obligations; the post-Start walk supplies a witness to `decideProcessReturn`. Reporting and tracing consume that final decision. | Pre-Start paths are not all traced. Their ownership rules and the flow's command/merged-command classification still require partial-duplication review. |
| Deferred cleanup in loop | `resourceLiveAtNextIteration` owns retention-before-defer, instruction classification and live-backedge search. Its Boolean result alone controls the reporter. | The proof currently emits final reasons internally instead of returning a structured outcome. Settled and unknown completion paths share an accepted trace reason. |
| Producer send lifetime | `abandonedProducerSend` returns `producerProof` after send attribution and `channelReceives`; the reporter and final trace consume Proven/Known. | Protocol counting and receive effects require the remaining classification review. Position deduplication is reporting mechanics, not a second proof. |
| Lock missing release | A completed buffered `walkLockOrderBounded` supplies held-return witnesses to `lockFlowContext.reportMissingReleases`, which applies private-lock, witnessed-release and caller-release boundaries. | Final multi-rule policy remains inline in the reporter rather than a single structured decision. |
| Read-lock write | `reportReadLockWrites` uses current held/read-held state, exact write/owner relation, exclusive-lock uncertainty and possible imported writers before reporting. | Final guard policy remains inline. The audited guard-to-field association gap is unresolved; common owner identity does not establish field protection. |
| Contradictory lock order | Completed function walks stage order edges; `lockOrders.record` applies declaration/instance boundaries and bounded cycle search, then `reportOrderCycle` formats the selected cycle. | Class refinement, serialization and novelty policies remain in one graph pipeline; this inventory does not establish their full semantic cohesion. |

Concurrent capture supplied a second concrete mismatch: the diagnostic path
emitted `capture-unguarded-write` with an accepted outcome. Its guard policies
now return one `mutationProof`; the collector maps that outcome to tracing and
reporting. The parent fails the rejected-outcome regression. The current test
also covers one decision per candidate and all four guard families, including
a branching-worker accepted fixture for the existing syntax fallback.
Parent/current `-enable-all -json` fixture scans both exit 3 with empty
stderr and identical nonempty diagnostic JSON (18,217 bytes). The traced CLI
receipt contains 22 candidate proof decisions, each unknown or rejected as
appropriate. The reporting infrastructure separately emits 15
`diagnostic-reported` events; those notifications are not additional guard
proofs. Receipts use `.build/goal-capture-decision-*`. No FP removal is credited
from this architecture correction. Focused analyzer tests and the final
canonical local gate pass (`.build/goal-capture-decision-final-verify.log`).
The initial canonical run found a commentary-coverage gap after moving the
old rationale; the collector now explains why only reported objects are
deduplicated, and the focused commentary check passes. No full precision
corpus or local race run was performed.

## Next verification

The catalog inventory identifies the remaining inline lock decisions
(`gohawk-dho.27`) and defer-loop outcome boundary (`gohawk-dho.28`). The
architecture audit must resolve those items and inspect partial duplication
in resource-state and obligation classification, beyond complete-body matches. It must also compare the
unresolved precision families with current helpers, rather than assuming
the earlier assessment permanently excludes an easy correction.

The [value-walk review](value-walk-review.md),
[storage/summary review](storage-summary-review.md), and
[remaining FP assessment](../../benchmarks/precision/audits/remaining-fp-assessment-2026-10-01.md)
remain scoped supporting evidence. Passing their checks is not a substitute
for the requirement-by-requirement completion audit.

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
| One authoritative decision for each check | Reviewed lifecycle analyzers classify once and ask a shared flow query. The remaining normalized matches in classifier caches and state keys contain different domain state, rather than second acceptance rules. | The ten-check inventory below locates each reporting pipeline. Concurrent capture and process ownership now share their final decisions with tracing. Lock release and mutation reporting also consume structured decisions after dho.27. Defer-loop outcomes use a structured proof after dho.28; resource lifetime now also owns its state and pre-flow policy exclusion after dho.30. Deeper classification and precision-family review remain open. |
| Remaining easy FPs fixed | Frozen batches 62/63 supplied 55 production FP locations. The queue refresh, successful 22-site replay and four subsequent corrections left 18 unresolved production sites. The later opaque registry-owner correction removes two FDio sites, leaving 16 unresolved. The ten-family assessment states the missing evidence for each. Rune callback wait is separately corrected; fresh-lock publication remains open. | No target-resolution-only correction has been demonstrated. The remaining families need policy, identity, state or protocol evidence. The current-helper reassessment found the FDio field-origin gap fixed in dho.31. Further source review must distinguish a newly available bounded fix from a genuinely larger model. |
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
| Resource release | `evaluateResourceFlow` owns acquisition boundaries, the memory-writer policy exclusion, resource-state walking and final structured result. `checkAcquisition` traces that state and reports only proven diagnostic evidence. | dho.30 puts the memory-writer gate inside the proof and removes trace-only reason interpretation. Resource state is richer than a generic join lattice; transitive classification review remains required. |
| Process wait | Pre-Start ownership gates select local obligations; the post-Start walk supplies a witness to `decideProcessReturn`. Reporting and tracing consume that final decision. | Pre-Start paths are not all traced. Their ownership rules and the flow's command/merged-command classification still require partial-duplication review. |
| Deferred cleanup in loop | `proveDeferLifetime` owns retention-before-defer, instruction classification and live-backedge search. Its structured outcome controls reporting and final tracing. | dho.28 separates unknown backedges from no-live-backedge acceptance without changing the traversal or diagnostic rule. Classifier policy still needs the remaining partial-duplication review. |
| Producer send lifetime | `abandonedProducerSend` returns `producerProof` after send attribution and `channelReceives`; the reporter and final trace consume Proven/Known. | Protocol counting and receive effects require the remaining classification review. Position deduplication is reporting mechanics, not a second proof. |
| Lock missing release | A completed buffered `walkLockOrderBounded` supplies held-return witnesses to `lockFlowContext.reportMissingReleases`, which requests `proveMissingRelease` for private-lock, witnessed-release and caller-release boundaries. | dho.27 moves final multi-rule policy into one structured proof. Flow evidence and caller contracts retain their distinct owners. |
| Read-lock write | `proveReadLockWrite` uses current held/read-held state, exact write/owner relation, exclusive-lock uncertainty and possible imported writers; the reporter consumes its result. | dho.27 consolidates final guard policy. The audited guard-to-field association gap is unresolved; common owner identity does not establish field protection. |
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

## Lock reporting consolidation

Beads `gohawk-dho.27` moves missing-release policy out of the reporting loop
into `proveMissingRelease`, and read-lock-write policy into
`proveReadLockWrite`. Both produce `lockDiagnosticProof`, whose state and
reason drive reporting and the shared trace projection. The policy order,
held-return witnesses, first reportable mutation owner and unknown-candidate
owner queries are preserved. The old caller-release-only trace adapter is
removed; accepted caller-transfer traces retain their reasons and positions.
The order graph and diagnostic buffering are unchanged.

The new trace assertions fail on the parent because those formerly silent
boundaries and reportable witnesses had no final proof decision. Existing
fixtures already cover private versus published mutexes, absent local release
policy, caller transfer, explicit versus possible writers, and reported
returns/writes. The production helper moves those policies rather than adding
another traversal or field-to-guard model. The final missing-release concern
is extracted from the existing 522-line operations file into its own focused
file; caller-set inference and conditional-result binding remain in operations.
This does not claim that all transitive lock helpers are consolidated.

The parent/current `-enable-all -json` scans of `lockorder` and `ordercycles`
fixtures both exit 3 with empty stderr and identical nonempty diagnostic JSON
(129,170 bytes). The current traced invocation verifies rejected release/write
witnesses, accepted private mutexes and unknown local-release/writer boundaries,
alongside the existing caller-transfer and imported-writer assertions. Receipts
use `.build/goal-lock-decisions-*`. Focused analyzer and architecture commentary
checks pass. Canonical `make verify` passes all local gates
(`.build/goal-lock-decisions-verify.log`), including ordinary tests, formatting,
vet, lint, dead-code and local dogfood. No full precision corpus or local race
run was performed. No precision label, FP count or exported fact schema changes.


## Defer-loop lifetime decision

Beads `gohawk-dho.28` replaces the Boolean live-backedge result and its embedded
final traces with `deferLifetimeProof`. The existing traversal selects the
same first live witness. Unknown-backedge state is recorded inside that walk;
it only determines an unknown final outcome when no live witness exists.
Retention-before-defer and unavailable instruction evidence remain unknown.
The entry projects the structured proof once into tracing and reporting;
classifier evidence and iterator-exhaustion evidence stay in the engine.

The former accepted combined settled/unknown reason is replaced by
`no-live-backedge` and `lifetime-unknown-at-backedge`. Fixtures confirm that
unknown/settled paths do not imply cleanup and cannot erase a separate live
path. In particular, the cleanup-on-both-branches fixture's current silence
is unknown, not proven cleanup. A return-only fixture supplies the accepted
control. The new trace assertions share the existing analyzer fixture run,
avoiding a second complete fixture analysis. The proof remains one cohesive
loop-lifetime query despite its existing function-size review trigger; a
second traversal or independently deciding wrapper is not introduced.

Parent/current `-enable-all -json` fixture scans both exit 3 with identical
nonempty diagnostic JSON (23,933 bytes). Receipts use
`.build/goal-defer-proof-*`. No FP removal or exported summary change is
credited; this is an authoritative-outcome and trace correction. The current
CLI receipt has exactly one proof decision for each of 36 fixture candidates:
21 live-backedge rejections, nine unknown backedges, three no-live-backedge
acceptances and three pre-defer retention uncertainties. Both stderr streams
are empty. The final canonical local gate passes all targets
(`.build/goal-defer-proof-final-verify.log`); initial formatter and helper/tag
lint issues were corrected. No full precision corpus or local race run was
performed. The identical diagnostic-state trace switches in process, lock and
defer reporting are consolidated in `gohawk-dho.29` as described below; this
decision-owner review does not establish the absence of other duplication.

## Diagnostic trace outcome projection

Beads `gohawk-dho.29` consolidates the identical final state-to-outcome switches
in process, lock and defer reporting into `trace.DiagnosticOutcome`. The
adapter's contract is explicitly about evidence permitting a diagnostic:
proven maps to rejected, disproven to accepted, and unknown or invalid states
remain unknown. It does not decide reporting or consume reasons, witnesses,
cleanup contracts or path evidence. Ordinary cleanup/transfer proofs retain
their opposite interpretation and are not routed through this adapter.

A contract test covers the complete uint8 state domain so unspecified values
cannot silently become acceptance or a proven diagnostic. Existing trace
regressions in all three consumers retain their phase, reason, outcome and
candidate assertions. The source-layering gate permits trace to depend on SSA
evidence types; SSA, heap and lifecycle engines still cannot import tracing.
The generic observer signature remains unchanged.

Focused trace and all three consumer package tests pass, as do the source
layering, trace and commentary checks. Parent/current `-enable-all -json`
process fixture scans both exit 3 with empty stderr and identical nonempty
diagnostic JSON (31,707 bytes). The traced current fixture retains accepted,
rejected and unknown proof decisions. Receipts use `.build/goal-trace-outcomes-*`.
The final canonical local gate passes (`goal-trace-outcomes-final-verify.log`);
the initial test-style lint finding was corrected. No full precision corpus,
local race, FP removal or exported summary change is credited.


## Resource final proof and bounded flow comparison

Beads `gohawk-dho.30` follows the resource classification review. The former
policy result stored a report Boolean, while final tracing separately selected
unknown for two HTTP reason codes and accepted every other silent result.
That described opaque consumption as acceptance and possible pre-acquisition
deferred cleanup as proven release. The existing memory-writer exclusion also
bypassed final proof presentation in the analyzer entry.

The policy result now carries diagnostic evidence state. The flow owner keeps
policy exclusions and exact return coverage disproven, distinguishes opaque
ownership and possible deferred cleanup as unknown, and returns proven only
for the existing reportable witness. The reporter consumes that state, and
tracing uses `DiagnosticOutcome` without consulting reasons. The memory-writer
gate moves into the same ordered proof before candidate attribution and every
query; it gains an accepted final policy reason, not a cleanup guarantee.
The existing acquisition proof remains above its function-size review trigger:
its cohesive responsibility is one ordered acquisition-to-return decision,
including pre-flow boundaries and witness-conditioned post-flow policy. No
second traversal or reporting policy is introduced.

Direct comparison of `resourceFlowState`/`advanceResourceState`/
`resourceSuccessorStates` with `ssaflow.obligationOutcome` finds material
contracts that prohibit mechanically replacing the resource walk with the
generic max-action lattice. Resource state distinguishes activation from
settlement and sticky uncertainty; acquisition-error and presence edges can
remove activation. The generic walk monotonically strengthens coverage. It
prunes stable guard contradictions, whereas resource flow conservatively
marks both contradiction kinds unknown. Resource returns and edges additionally
consult exact owner, collection and conditional completion evidence. Both use
`WalkStates`, `PathGuards` and `SuccessorPolicy`; domain transitions remain
local. This disposition covers these flow functions, not every classifier
predicate or every transitive callee. The adjacent classifier/cache review
still leaves broader partial-duplication investigation open.

The regression shares the existing resource fixture run. The parent overlay
fails on accepted opaque consumption, accepted possible deferred release and
missing memory-writer decisions; the current focused package tests pass.
The selected controls cover exactly one proof decision at each expected
candidate, including real release, bypassed async handoff and mixed external
writers. The actual imported async fixture's SSA confirms the boxed file
argument and ordinary return after the imported helper; it is recorded in
`.build/goal-resource-decision-ssa.txt` rather than inferred from syntax.

Parent/current `-enable-all -json` scans of the local resource fixture both
exit 3 with empty stderr and identical nonempty diagnostic JSON (335,147 bytes).
The current scan enables tracing; its 713 final proof decisions contain 256
accepted, 141 unknown and 316 rejected outcomes, including three memory-writer
exclusions. Infrastructure diagnostic notifications are excluded from those
counts. These are fixture observations, not precision-corpus labels or FP
removal credit. Receipts use `.build/goal-resource-decision-*`. Focused analyzer
and architecture checks pass. The final canonical local gate passes all
checks (`.build/goal-resource-decision-final-verify.log`), including ordinary
tests (25 seconds), formatting, vet, lint, dead-code and local dogfood. The
initial run passed ordinary tests and the other gates but found structurally
similar assertion tables through the duplication linter; the new final-decision
assertions use a case table instead. No full precision replay or local race run
was performed.

## Current-helper FP reassessment: embedded registry groups

Beads `gohawk-dho.31` revisits the remaining families after consolidation.
Skywalking's two same-owner cursor writes still require field-to-guard or
participant evidence; ordinary field ownership and the existing exclusive-lock
uncertainty do not establish that relation. FDio supplies a bounded correction:
its completion group is a field address below a stable captured load of a
comma-ok assertion of `GetPrivateData`'s imported result. The existing
`opaqueGroupOrigin` boundary recognized an unreadable returned group but stopped
at this field address. It now folds the field's base with the same
`ReachingWalk`, retaining the storage resolution step first.

This widens the existing unknown ownership boundary rather than proving
registration identity, callback ordering or completion. Fresh local owners,
visible fresh constructors, unrelated factory calls and a fresh pointer stored
into a returned owner's group field retain missing-join diagnostics. The new
fixture covers nested fields, mixed local/registry origins and captured forms;
its parent overlay fails three accepted cases and their unknown trace
expectations. Current focused tests pass. The scoped
[registry-group receipt](../../benchmarks/precision/audits/registry-group-followup-2026-10-02.md)
credits two FP removals and leaves 16 unresolved sites. Canonical local
validation passes all checks (`.build/goal-registry-group-final-verify.log`).
An imported-fresh-owner probe confirmed the accepted recall loss; its
false-negative fixture was removed and the gap recorded in the retained header,
following the project policy. The broader classifier review and the other
FP-family reassessments remain open; this correction does not establish goal
completion.

## Partial-block mechanics review

Beads `gohawk-dho.32` extends the earlier complete-body scan to every nested
Go block beneath production function declarations, at baseline `0fc74e8`.
It uses the same exclusions and identifier normalization. The 270 files and
1,994 functions yield no repeated blocks at 80 tokens and 20 candidate groups
at 35 tokens. This is a syntactic candidate search: it does not compare
arbitrary statement subsequences or semantically equivalent expressions.
Artifacts are `.build/goal-duplicate-blocks.go` and
`.build/goal-duplicate-blocks{,-current}.json`.

Two groups identify actual duplicated mechanics. Cancellation and resource
result guards repeated the unconditional guarantee-to-outcome switch; it now
belongs to `resultfacts.Guarantee.Outcome`. The conversion cannot infer a
summary, strengthen an unknown guarantee, bind a call or establish cleanup.
Both consumers keep literal evidence before their broker query, and retain
their existing query budgets and provider assumptions.

`BlockInCycle` and `BlockReachable` repeated a raw CFG breadth-first traversal.
Inspection also found the same loop in `InstructionMayFollow`; all three now
delegate to one private driver in the flow file. Each wrapper retains its
start policy: identity counts for block reachability, a cycle requires at
least one edge, and instructions in the same block retain source order even
inside loops. Nil/cross-function guards remain in their existing wrappers.
The driver clones its starting slice so queue growth cannot alter SSA
successors. No path-feasibility or lifecycle policy is shared through this
mechanical traversal.

The other 18 candidate groups remain review inputs, not automatic duplication
findings. They include already-disposed domain state keys, classifier caches,
store predicates and budget adapters, plus projection roots, result conditions,
CLI ordering, completion coverage and local switch arms. Exact body similarity
does not establish that their argument contracts or conservative boundaries
agree. Their source review remains part of the open completion audit.

Targeted source inspection also confirms two further mechanical candidates:
the CLI's position/string ordering comparator (`gohawk-dho.33`) and heap
slice-view lookup with array fallback (`gohawk-dho.34`). Other matches expose
material distinctions: callback roots allow slices but cap the search at eight
steps, while ownership roots follow fields without that cap; process captures
use captured-binding identity before arguments use may-alias identity; HTTP
error claims use derivation while nil claims use may-alias. Condition negation
in concurrency publication additionally stops at an existing compared value.
Those distinctions must survive any future extraction. Completion's normal
return/action witness scan and CLI selection parsing still need a shared-helper
review; they are not disposed as distinct merely because their enclosing
functions differ.

Focused SSA, result-domain, broker and affected analyzer tests pass. The
guarantee control covers every underlying uint8 value, leaving unspecified
values unknown; actual SSA controls distinguish reachability from cycles and
retain instruction order inside cycles. Parent/current `-enable-all -json`
fixture scans of cancellation and resource lifetime both exit 3 with empty
stderr and byte-identical nonempty diagnostics (35,065 and 335,147 bytes).
Receipts use `.build/goal-partial-mechanics-*`. Their baseline binary hash is
`68ac5a420286b77de2814d0eba3cbc72e8a627ee481a37c79c88767fe0210423`;
the corrected hash is
`0f8c99d52a52e509e7619393dd42d78aec8d7f0a86985e7a45c78e139a25b6a5`.
Both are retained pre-commit executables; the hashes identify the scanned
binaries rather than clean VCS stamps. No FP removal or fact-schema change is
credited. Final `make verify` passes all checks
(`.build/goal-partial-mechanics-completion-verify.log`), including ordinary
tests (five seconds). Earlier gates found fixture duplicate-word and complexity
lint issues; distinct marker arguments and splitting the tests by contract
corrected them. No full precision corpus or local race run was performed.

## CLI ordering and heap slice views

Beads `gohawk-dho.33` and `gohawk-dho.34` address two remaining nested-block
candidates at baseline `698467f`. Fact and heap dumps now obtain the same
function comparator from `dump_order.go`, beside their shared source-position
comparison. The existing filename/offset ordering and qualified-name tie break
are unchanged. Each dump retains its own function collection, body filtering
and fact selection. The raw token-position sort used by SSA dumps and the
AST-range ordering used by trace presentation have different inputs and are
unchanged; no claim of consolidating every presentation order is made.

The heap review found a third copy outside the exact block matches: indexing
used the same array fallback as slicing and selected aggregate snapshots.
All three now use `regionGraph.view`. It returns existing offset/size/capacity
metadata first, otherwise the full window of an array or pointer to an array.
An unrecorded slice remains unknown, and an empty array remains a known empty
window. Index bounds, slice-bound validation, selected-snapshot size limits,
wildcard writes, stale markers and content projection stay with their callers.
This changes neither observation times nor query budgets or summary schemas.

The nested-block scan after these extractions covers 271 production files and
1,997 functions and yields 16 candidate groups at 35 tokens. Its artifact is
`.build/goal-duplicate-blocks-after-cli-view.json`. The reduction establishes
that the selected copies were removed, not that the remaining groups are all
equivalent or that arbitrary partial duplication is absent. Completion-witness
scans, selection parsing and the deeper classifier/FP-family reviews remain
open.

Existing heap controls cover constant and nested slice offsets, three-index
bounds, out-of-length indexes, dynamic offsets, copied destination windows,
unknown/oversize windows and replaced/opaque destinations. The focused heap
package passes; focused CLI tests retain fact-kind filtering, private functions,
unconverted methods and optional SSA rendering. An initial focused CLI build
found the SSA import made unused by moving the comparator; it was removed.
The canonical local gate then passes all targets
(`.build/goal-cli-view-verify.log`), including ordinary tests (74 seconds).

Parent/current fact and heap dumps of the two-file local probe containing a
method, private function and returned closure both exit 0 with empty stderr
and byte-identical nonempty output (2,336 and 1,976 bytes). Receipts and probe
sources use `.build/goal-cli-view-*`. The retained baseline binary hash is
`0f8c99d52a52e509e7619393dd42d78aec8d7f0a86985e7a45c78e139a25b6a5`;
the corrected hash is
`20ce2f13e30a204de684fb0ae18dd5defa2b13b42c5116b9fc05cc9185ff3c25`.
These pre-commit hashes identify the exact executables, not clean VCS stamps.
The resource fixture compatibility comparison reuses the prior baseline
receipt from that same executable hash, avoiding an unchanged second scan.
Both resource scans exit 3 with empty stderr and identical nonempty diagnostic
JSON (335,147 bytes). No FP correction, full precision corpus replay or local
race run is credited.

## Next verification

After the catalog reporting-boundary consolidations, the architecture audit
must inspect partial duplication
in resource-state and obligation classification, beyond complete-body matches. It must also compare the
unresolved precision families with current helpers, rather than assuming
the earlier assessment permanently excludes an easy correction.

The [value-walk review](value-walk-review.md),
[storage/summary review](storage-summary-review.md), and
[remaining FP assessment](../../benchmarks/precision/audits/remaining-fp-assessment-2026-10-01.md)
remain scoped supporting evidence. Passing their checks is not a substitute
for the requirement-by-requirement completion audit.

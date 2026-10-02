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
| Remaining easy FPs fixed | Frozen batches 62/63 supplied 55 production locations originally labelled FP. The queue refresh, successful 22-site replay and four subsequent corrections left 18 unresolved production sites. The later opaque registry-owner correction removes two FDio sites, leaving 16 unresolved; the one-time entry-context correction in dho.37 removes one k8ssandra site, leaving 15. The entry-process correction in dho.4.1 removes four coder sites, leaving 11 recorded unresolved sites. The ten-family assessment preserves its earlier 18-site snapshot and the evidence gaps; these follow-ups are scoped receipts, not a fresh corpus census. Rune callback wait is separately corrected; fresh-lock publication remains open. The later pipe, private-entry and closure-choice follow-ups leave seven recorded production sites plus Rune; see the scoped receipts below. | Skywalking field association, Openase transport completion, goiardi caller preconditions, Ferro receiver-state guarantees and Rune publication remain unresolved. No target-resolution-only correction has been demonstrated. The remaining families need policy, identity, state or protocol evidence. The current-helper reassessment found the FDio field-origin gap fixed in dho.31 and the one-time entry policy corrected in dho.37. Further source review must distinguish a newly available bounded fix from a genuinely larger model. |
| Precision preserved by consolidation | Parent/current boundary comparisons, accepted and diagnostic fixtures, ordinary tests and local dogfood scans accompany focused changes. Focused controls pin must/may polarity, exact binding, observation time and bounded unknown outcomes. The captured-outcome timing correction in dho.44.11.5.25.2 now requires the unique store to dominate closure creation; later current values remain caller-supplied evidence. | Passing receipts prove their stated scopes. They do not certify every historical finding against the latest source. Use affected pinned cases when behavior changes; do not rewrite frozen labels or credit unscannable cases. |
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

## Return/action witnesses and selector values

Beads `gohawk-dho.35` and `gohawk-dho.36` review the remaining scan and parser
candidates at baseline `9853502`. The return/action enumeration in heap
requirements and lifecycle completion has the same contract and now belongs
to `ssaflow.HasReturnAndAction`. It supplies independent witnesses only, not
evidence that an action covers a return. The predicate remains short-circuited
after its first match, while normal-return enumeration continues; this retains
the query order of predicates that charge a budget. The helper adds no budget
or feasibility policy. Heap requirements retain their bounded obligation flow;
lifecycle completion retains its selected reachable blocks, entry assumptions
and uncovered-return query. Nonreturning functions establish no guarantee.

The CLI review found the same value extraction in check selectors, outside the
two exact block matches for analyzer and group selectors. `selectionFlagValue`
now owns inline-versus-following extraction for all three. It returns the
number of trailing arguments consumed. Missing-value wording stays identical;
empty-value wording, supported-name validation, duplicate/conflict checks and
selection precedence remain with the callers. A following flag is still
consumed as the value and rejected by name validation; this extraction does
not introduce a general argument parser or change that existing behavior.

Focused SSA, heap and lifecycle package tests and CLI selector tests pass. The
new witness control uses actual SSA to cover absent returns, absent actions,
optional actions, blocks excluded by a fixed condition and predicate calls
stopping after the first action while the later return is still observed.
Existing heap budget-cut and lifecycle condition controls retain the subsequent
coverage proof. The normal-return witness is deliberately separate from the
flow's vacuous acceptance of a function without a normal exit.

The nested-block scan now covers 271 production files and 1,999 functions,
with 14 candidate groups at 35 tokens
(`.build/goal-duplicate-blocks-after-witness-selection.json`). The remaining
groups still require contract dispositions; this reduction is not a broader
absence proof. Generated helper documentation includes the witness API.

The canonical local gate passes all targets
(`.build/goal-witness-selection-verify.log`), including ordinary tests
(91 seconds). Parent/current comparison of 24 CLI invocations retains every
exit status and exact stdout/stderr: six valid enable/disable selections across
analyzers, groups and checks exit 0; 18 missing, empty and following-flag values
exit 2. Receipts use `.build/goal-witness-selection-{parent,current}-cases.json`
and the prior two-file local probe. The resource fixture comparison retains
identical nonempty diagnostic JSON (335,147 bytes), exit 3 and empty stderr,
reusing the previous receipt from the unchanged baseline executable hash.
The retained baseline hash is
`20ce2f13e30a204de684fb0ae18dd5defa2b13b42c5116b9fc05cc9185ff3c25`;
the corrected hash is
`735f414e60f1a599f8b2930e4a8b0239d7d9ea08f97b358353018e5558c343d7`.
These identify pre-commit executables, not clean VCS stamps. No FP removal,
fact-schema change, full precision corpus replay or local race run is credited.

## One-time entry-context reassessment

Beads `gohawk-dho.37` uses existing `ssaflow.RunsOnceInProgramEntry` evidence
at cancellationownership's instruction classifier. The program-entry return is
unknown for a standard context acquired outside cycles, never evidence that its
cancel runs or workers finish. Signal registration, repeatable/helper scopes
and referenced or non-entry main forms retain diagnostics. Exact cleanup still
wins through the same authoritative flow query; exported facts are unchanged.

The [durable receipt](../../benchmarks/precision/audits/program-entry-context-followup-2026-10-02.md)
records actual SSA, parent/current trace outcomes, immutable binary hashes and
successful pinned scans. K8ssandra's original report disappears (exit 3 to 0);
both reviewed Openase cancellation TPs remain (exit 3, identical JSON). The
parent overlay fails the new accepted cases and trace expectations, while the
corrected focused test passes. The resulting queue has 15 unresolved sites.
The accepted coverage loss concerns earlier completion of entry-context work;
no blanket exemption for helpers, command subprocesses or signal stops follows.

The action enum moves to the existing label vocabulary, keeping proof.go within
the lint limit. The stable canonical gate passes all local checks, with ordinary
tests taking 3 seconds (`.build/goal-entry-context-stable-verify.log`). Focused
documentation checks also pass after correcting the test reference. No full
precision replay or local race run was performed. This bounded correction
does not settle the deeper duplicate-classification review or overall objective.

## Boolean-negation traversal and finite candidate dispositions

Beads `gohawk-dho.38` reviews the current nested-block candidates at `9f19d82`.
Graph discovery tools remain unavailable. The source-based scanner covers 271
internal production files and 1,999 functions, yielding 14 groups. It normalizes
all identifier names, including predeclared names, and compares whole nested AST
blocks of at least 35 tokens. Tests, generated files, fixtures and vendor trees
are excluded. It does not find arbitrary subsequences, differently structured
logic, smaller blocks, or code outside its scope; this is a finite review rather
than a proof that the repository has no remaining duplication.

The NOT-chain traversal shared by fixed branch evaluation and concurrency
condition binding is now `ssaflow.BooleanNegationSource`. Two recursive copies
in guard decoding and conditional completion use the same helper. It returns
only the exact leaf and odd/even inversion parity. Loads, comparisons,
conversions and phi merges remain leaves; stability, binding and cleanup
semantics stay with each caller. Concurrency binding still declines prior call
contexts, captured parameters and negation peeling for a compared condition.
The goroutine analyzer's separate single-negation acceptance rules are unchanged.

Actual probe SSA is `.build/goal-negation-probe.ssa.txt`, with empty stderr and
exit 0. It shows explicit stored one-, two- and three-NOT chains, a pointer load,
a named Boolean conversion, a comparison and a phi. Focused tests in the flow,
lifecycle and concurrency-facts packages pass (0.423, 0.529 and 3.294 seconds).
The added controls check parity and opaque leaves, exact fixed bindings,
concurrency binding gates, and stored-negation conditional cleanup. Generated
helper references include the new contract.

The current scanner then covers 271 files and 2,001 functions with 13 groups
(`.build/goal-duplicate-blocks-after-negation.json`). Each remaining group was
read at its named decision point; the following dispositions apply to this
finite set:

| Tokens | Candidate | Contract disposition |
| --- | --- | --- |
| 64 | Resource and generic obligation state keys | Both use the work-list driver; one keys rich resource obligations, the other the generic coverage lattice. Domain state and guard keys remain caller-owned. |
| 56 | Spawn/resource classifier caches | Both invoke their authoritative classifier once and trace that result; actions, reasons and trace details belong to different evidence families. No parallel acceptance decision is introduced. |
| 55 | Field/global/enclosing-scope stores | Field/global transfers ask possible alias; captured-scope transfer asks derivation and a distinct destination. Shared provenance already supplies those queries. |
| 52 and 41 | Callback address / aggregate root | Callback locality walks only index/slice forms, with an eight-step bound; aggregate ownership follows field/index addresses without that bound and returns a root for exact access-path queries. A shared unconditional unwrap would change their precision boundaries. The inner 41-token match is part of the same comparison. |
| 51 | Stable Boolean identity arms | The Boolean type and identity construction for two stability sources is consolidated in `gohawk-dho.39`; computed values retain cycle restrictions and comparisons retain their separate policy. |
| 44 | Spawn/resource budget adapters | Candidate pool ownership, domain limits and observers differ; charging and exhaustion already use the shared search budget. |
| 42 | Lock handoff / returned ownership | Argument alias/derivation differs from result alias/aggregate ownership. Their traversal inputs and proof polarity cannot be exchanged. |
| 40 | Unproven completion results | Distinct unknown reasons now share one proof construction in `gohawk-dho.39`; unavailable provenance and budget/cycle/incomplete priority stay exact. |
| 38 | Deferred command capture/argument waits | Capture matching precedes argument alias matching intentionally: unknown captured completion must not be reordered behind argument success. Positional mapping and completion machinery are already shared. |
| 36 | HTTP error/nil claims | Error claims use derivation of the paired error; nil claims use alias of the resource. These are different assertion contracts, with repeated argument matches retained. |
| 36 | Parameter/result heap truncation | Two disjoint root inventories are explicitly marked truncated on unavailable projection. Parameter count and result count are different schema contracts. |
| 35 | Identity/projection/transfer proof literals | Similar record syntax carries different proof types and semantic reasons. These are domain conclusions, not duplicated evidence searches. |

Parent/current all-check scans of the resource and cancellation fixture packages
retain byte-identical nonempty diagnostic JSON (370,210 bytes), exit 3 and empty
stderr. Receipts are `.build/goal-negation-{parent,current}.json`. The retained
baseline executable, built before this change from production source `9f19d82`,
has SHA-256 `9074dc85a17845ea0adfc7f929a34946f9c281796d839fcb7ed6446a47141cff`;
the corrected executable has
`1250bd66cbbe9c083182bb25f0566e557413393269a0ceba290f0431eb714a28`.
These are pre-commit executable identities, not clean Git stamps. Their files
were not replaced during scans. The stable canonical gate passes all targets
(`.build/goal-negation-final-verify.log`), including ordinary tests (3 seconds).
The first gate found only new-table formatting; the canonical formatter corrected
it before the stable gate. The first ordinary suite also passed (76 seconds). No
precision FP removal, fact-schema change, full corpus replay or local race run
is credited; the production queue remains 15 sites.

Rune `gohawk-cnx` was separately reassessed from pinned source and current
`exclusive.go`. Its new mutex is stored in a shared map before acquisition, so
`heapmodel.ExclusiveAt` cannot supply the existing before-publication proof.
Proving registry-guarded first acquisition needs participant and publication
relationships not supplied by that query. This source assessment makes no new
latest-binary replay or correction claim; the issue remains open.

## Stable-identity and completion-result construction

Beads `gohawk-dho.39` consolidates the two same-policy construction groups
identified in the finite review at `ea057f8`. Lock `conditionIdentity` now
collects the two existing stability sources before one Boolean type/identity
construction. Parameters remain stable for the invocation; computed values
must be outside a control-flow cycle and cannot be comparisons. Equality and
inequality retain their separately formatted operand identity, while other
comparisons remain unknown. This does not change branch feasibility or infer
that a mutable field stays unchanged across distinct loads.

`CompletionRequest.unprovenCompletion` now initializes one unknown proof and
selects its reason/state before observing it once. An unavailable search retains
empty provenance even when its budget was exhausted. A searched body has local
SSA provenance; exhaustion takes priority over a completion inside a cycle,
which takes priority over incomplete nested work. Only a fully searched body
with no such uncertainty produces `EvidenceDisproven`/`EvidenceNotFound`.
The existing cycle rationale and pinned source link remain at that decision.

The new controls exercise actual public completion searches rather than
inventing combinations of internal flags. They cover missing/unavailable bodies,
recursive work, a cleanup loop followed by recursion, searched-body exhaustion,
and exhaustion before an unavailable body is visited. Each checks the complete
proof, no path claim, final reason/position/details and exactly one final
observation at the request. Lock controls cover Boolean and named-Boolean
parameters, stored NOT, load, call and phi results, excluded loop computation,
non-Boolean values and the separate equality/inequality policy. Actual SSA is
`.build/goal-{lock,completion}-construction.ssa.txt`; both dumps exit 0 with empty
stderr. The focused controls pass on the parent-source overlay (0.004/0.005
seconds), while current full lifecycle and lock packages pass (0.725/6.589
seconds) before the final observation-count assertion; the canonical gate
covers that retained assertion.

The nested-block scan covers the same 271 production files and 2,001 functions,
with 11 groups (`.build/goal-duplicate-blocks-after-proof-construction.json`).
Both construction matches are absent; the remaining finite groups retain the
source-backed contract dispositions above. This resolves the selected copies,
not differently structured or smaller classifier logic outside that scan.

Parent/current all-check lock fixture scans (`lockorder`, `orderedhelpers`,
`helperstate`) have identical nonempty diagnostic JSON (132,023 bytes), exit 3
and empty stderr. Their 1,179 lock trace events, including 221 decisions, match
as exact event multisets: reasons, outcomes, positions and details are retained,
without asserting concurrent event-file order. The resource/cancellation fixture
scan also retains identical nonempty diagnostic JSON (370,210 bytes), exit 3 and
empty stderr. Its baseline reuses the unchanged executable hash and successful
receipt from the preceding negation consolidation; it was not rerun solely to
produce a new filename.

Receipts are `.build/goal-proof-construction-lock-{parent,current}.{json,stderr}`
and the corresponding `.trace.jsonl` files. Resource receipts are
`.build/goal-negation-current.json/.stderr` (baseline) and
`.build/goal-proof-construction-resource-current.json/.stderr` (corrected).
The baseline production source is `ea057f8`; exact executable SHA-256 is
`1250bd66cbbe9c083182bb25f0566e557413393269a0ceba290f0431eb714a28`.
The corrected hash is
`5bf8c6ab4018268fb4a36c31200e7c3c2a6bb71e56e655e12de88b25304c099c`.
These identify immutable pre-commit executables, not clean Git stamps. The
canonical `make verify` gate passes every local target
(`.build/goal-proof-construction-verify.log`), including ordinary tests
(109 seconds), formatting, vet, lint, dead-code and local dogfood. No precision
FP removal, fact-schema change,
full precision-regression replay or local race run is credited; the production
queue remains 15 sites, and Rune publication and the broader classification
review remain open.

## Next verification

Beads `gohawk-dho.40` owns the next bounded source review: resource and
cancellation instruction classifiers, their local helper and return/edge routes,
and the shared lifecycle/storage/completion queries they select. The finite
11-group review does not cover all differently structured transitive policy.

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

## Cancellation return classification review

The source review in `gohawk-dho.40` found a concrete split in cancellation
classification, now tracked as `gohawk-dho.41`. This is a source fallback;
Codebase Memory graph tools are unavailable. The inspected route is bounded to
`cancellationownership/proof.go`, `labels.go`, `result_guards.go`,
`owner_structs.go`, `parent_context.go`, and the obligation loop in
`ssaflow/flow_obligation.go`. The resource comparison covers its action cache,
`classify` ordering and `result_guarded_defers.go`; it does not establish that
all resource/helper/edge predicates or their transitive implementations have
been reviewed.

| Input | Former route | Consolidated route and preserved boundary |
| --- | --- | --- |
| Ordinary cancellation use | `action` caches `classifyAction`, including opaque uses and parent-context uncertainty. | The same cache owns the combined label. Parent evidence remains unknown for the child. |
| Named-result deferred cleanup | The instruction callback runs first; a separate uncached return callback asks `resultGuardedReturn`. | `returnLabel` contributes to the instruction label. Exact cleanup wins over ordinary uncertainty; unknown remains opaque. |
| Returned cancel or fresh owner | The return callback sets transfer separately; a direct cancel's ordinary label says unknown. | One accepted transfer label and the same transfer flag accompany the final cached action. Visible-owner and written-once capture restrictions remain. |
| CFG-edge completion or selected Done receive | `edgeObligation` supplies edge-local evidence. | Remains separate: successor-specific evidence must not be cached as an instruction-wide discharge. |
| Resource result-guarded cleanup | `resultGuardedLabel` already participates in `classify` and the resource action cache. | Distinct method-cleanup requests and resource states remain local; no common policy engine is introduced. |

The actual merged-success SSA has two predecessors reaching one return after
`rundefers`. The parent invokes the uncached return classifier in both flow
states, emitting two identical `result-guarded-release` labels for the same
candidate/instruction. The new fixture fails the parent with two labels and
one decision. The corrected action cache emits one of each. Return queries no
longer repeat solely because flow revisits that instruction; this makes no
claim about a measured runtime or memory improvement. The shared flow still
owns coverage, branch feasibility and the final witness.

The production parent executable is source `ff205a5`, retained as
`.build/goal-return-classifier-parent`, SHA-256
`5bf8c6ab4018268fb4a36c31200e7c3c2a6bb71e56e655e12de88b25304c099c`.
The corrected executable is `.build/goal-return-classifier-current`, SHA-256
`55b4fba619a03304f0a904d2d368a5be93586b5724a8bb64080c5fc7f6e16c3c`.
The actual SSA is `.build/goal-return-merged.ssa.txt`. Parent/current
`-enable-all -json` fixture scans exit 3 with empty stderr and byte-identical
35,065-byte diagnostics. Their 179 final decisions match as multisets;
601 parent labels become 600 corrected labels, with the single repeated
instruction group removed. The direct returned-cancel label also changes from
unknown to accepted, matching its unchanged accepted final transfer proof.
Receipts are `.build/goal-return-merged-parent.*` and
`.build/goal-return-classifier-current.*`; the earlier parent scan before the
new fixture and failed path invocation are not equivalence evidence.

This is architecture progress, not an audited FP correction. The 15-site
production queue is unchanged. `gohawk-dho.40` still owns the remaining helper,
return/edge and storage-family review; the overall completion audit remains
unproven. No full precision-regression or local race run is part of this fix.

The corrected cancellation package tests pass (18.398 seconds). After adding
its direct-return label assertion, the final focused analyzer run passes
(8.342 seconds), as do the documentation architecture checks (0.649 seconds).
Canonical `make verify` passes ordinary tests (69 seconds), generation,
formatting, vet, lint, dead-code and local dogfood; its receipt is
`.build/goal-return-classifier-verify.log`. A stable gate after the final
assertion and review-note edits is recorded separately as
`.build/goal-return-classifier-stable-verify.log`.

## Resource and cancellation helper-query inventory

The `gohawk-dho.40` review continues from the return-path correction. The
following rows inventory selected inputs and their proof routes at source
`b2ea7e4`, plus the resource completion-label correction in `gohawk-dho.42`.
Source inspection covers query selection and these local policy boundaries;
it does not prove completeness of the shared engines' transitive bodies.

| Evidence family | Cancellation route | Resource route and disposition |
| --- | --- | --- |
| Exact helper completion | `proof.go:exactArgumentAction` asks `ProveCompletion` with `InvokeTarget`; only proven synchronous invocation releases. Imported positive invocation masks are consulted after unknown local evidence through `returned_cleanup.go:summaryInvokes`. | `contracts.go:releasesOrdinaryResource` asks lifecycle evidence with cleanup methods, selected imported masks and strict projection mapping. `cleanupReceiver` additionally consumes unchanged-result identity. These select different contracts from one completion engine. |
| Possible helper cleanup | Disproven exact completion can still produce unknown through the older may-alias invocation or returned-deferred-cleanup query. | `correlated_cleanup.go:provePairedErrorCleanupWithin` requires an exact resource/error relationship and an anywhere cleanup witness; `ambiguous_cleanup.go:proveAmbiguousHelperCleanupWithin` requires merged argument provenance and proven cleanup of that argument. Neither establishes cleanup of the exact acquisition. |
| Local observation | `localCallOnlyObserves` first asks `CallEffects.PreservesStorage`, then the cancellation-specific `cancellationUse` memo checks each exact argument/capture binding. Its conservative unavailable/cycle result is unresolved use. | `opaqueCall`, `proveAggregateOwnerEscapeWithin`, `provePossibleWrapperWithin` and `provePossiblyRetainedCallbackWithin` distinguish retain, asynchronous exposure and unknown effects at publication sites. A generic read-only test cannot replace cancellation invocation resolution or resource retention. |
| Capture storage | `result_guards.go` accepts a written-once cancel cell used only by directly deferred literals, with store-before-defer dominance. `owner_structs.go` separately restricts a captured cell to one visible owner field. | `captured_cleanup.go` handles current vs deferred cell observations, response Body guards and possible retention by unreadable callees. Broader mutation/exposure is unknown; the cancel cell restrictions must not be weakened by this resource policy. |
| Owner/destination identity | Fresh cancellation owners require visible field/return uses; parent context resolution observes storage at child creation. | `storage.go:proveResourceStorage` resolves an indirect destination at the store. `carried_values.go:proveCarriedDirectlyWithin/proveNestedCarryWithin` and Body handoff distinguish identity, projection and observation-time containment. These are separate questions, already supplied by storage/reaching helpers. |
| Wrapper traversal | Broad cancellation references allow ChangeInterface, ChangeType, Convert and MakeInterface for uncertainty; exact invocation does not inherit this broad relation. | `possible_wrappers.go:unwrapWrapperWithin` peels only ChangeInterface, ChangeType and MakeInterface. Wrapper chains are bounded and count possible retention only at suitable publication boundaries. The differing Convert policy is deliberate; no universal unwrap is appropriate. |
| Returned resource owner | Cancellation return evidence now contributes to its cached instruction label. | `flow.go:returnedResourceOwner` is asked only for a live obligation without exact or opaque coverage; owner/cleanup-bearing result evidence and local-owner aliases then prevent an uncovered return. It does not re-run final reporting policy. This late query and its path-dependent trace frequency remain separate from cancellation's cached return labels. |
| Edge evidence | `edgeObligation` requests invocation completion on the selected edge, then exact own-Done selection supplies uncertainty. | `resourceSuccessorStates` combines acquisition/presence feasibility, path guards, Rows exhaustion, collection cleanup and method completion on that edge. Optional acquisitions deliberately exclude generic completion. These inputs affect different state dimensions and cannot share an unconditional instruction label. |
| Result guards | `result_guards.go:outcomeOf` checks literal outcomes before brokered result guarantees, with the candidate budget and missing-provider boundary. | `result_guarded_defers.go` does the same outcome projection, but submits one cleanup-method request per contract. Shared `Guarantee.Outcome` mechanics already exist; request meaning and budgets remain domain-owned. |

Direct source ranges inspected in this continuation include cancellation's
recognized direct/call actions, exact argument completion, local observation,
returned cleanup and parent resolution; resource `classify`, ownership helpers,
ordinary/optional release selection, cleanup receiver/mask/callback selection,
return ownership and successor transitions. The related local collection and
captured-cleanup policy bodies, optional-acquisition proof, acquisition-contract
recognition and every shared engine implementation are not claimed fully
reviewed here. Graph tools remain unavailable; no negative graph claim is used.

The helper review found two concrete gaps. First, resource `releaseSettled`
combined proven completion and `EvidenceBudgetExhausted` into an exact action.
`gohawk-dho.42` replaces this Boolean policy with `releaseLabel`: exact evidence
is settled, exhaustion is unknown/budget-exhausted and loop-only completion
keeps its existing unknown reason. Other failures retain ordinary
classification. The pre-acquisition deferred may-release boundary still treats
exhaustion as uncertainty and preserves its existing early exit. No obligation
is inferred from exhausted evidence and no exported fact is strengthened.

The parent regression obtains an actual local SSA completion proof under a
one-step budget: unknown/budget-exhausted. Its old settlement predicate returns
true and fails the expected non-settlement assertion (0.004 seconds). The
corrected test checks the authoritative action/reason pair and adds exact,
conditional and loop-only public-query controls (0.008 seconds). The resource
analyzer tests pass (26.006 seconds). Actual SSA is retained in
`.build/goal-resource-exhaustion.ssa.txt`; its stderr is empty.

Parent production source is `b2ea7e4`, executable
`.build/goal-resource-exhaustion-parent`, SHA-256
`55b4fba619a03304f0a904d2d368a5be93586b5724a8bb64080c5fc7f6e16c3c`.
Corrected `.build/goal-resource-exhaustion-current` has SHA-256
`9fcd5e02b2e9fc56a1017a423e43b55321bab3328c410e2802f520e2455fa9eb`.
These identify exact executables, not clean-tree metadata. Both all-check
resource fixture scans exit 3 with empty stderr and identical 335,147-byte JSON;
their 1,029 final decisions and 5,858 labels match as multisets. Receipts use
`.build/goal-resource-exhaustion-{parent,current}.*`. These ordinary fixtures do
not force the exhausted query: the targeted public-query regression establishes
that boundary. No production FP removal is credited.

Second, ordinary resource helper completion allocates a standalone 250,000-step
budget per method, outside the candidate pool and its observer. The candidate
adapter test establishes pool behavior, not that every query uses it; its
comment now states that scope. `gohawk-dho.43` tracks routing those requests
through candidate-owned allowances without accidentally imposing the smaller
storage-query cap. This budget ownership issue is unresolved in dho.42.
`gohawk-dho.40` remains active; the completion audit and 15-site FP queue remain
open. No full precision-regression or local race run is part of this iteration.

The first canonical gate passed ordinary tests (65 seconds), vet, formatting,
generation, dead-code and dogfood but failed the contracts-file size limit.
The completion-to-label policy now lives in the existing classifier file;
its action/reason vocabulary is the same classifier concern, with no new
traversal or evidence engine. The comparison executable above predates this
mechanical move. The final gate receipt is
`.build/goal-resource-exhaustion-final-verify.log`; the earlier failing receipt
is `.build/goal-resource-exhaustion-verify.log` and is not a passing gate.

The corrected final canonical gate passes generation, formatting, vet, lint,
dead-code, local dogfood and ordinary tests (73 seconds). Documentation
architecture checks after the receipt-note edits pass (0.614 seconds), recorded
in `.build/goal-resource-exhaustion-docs.log`. The size-limit failure is resolved;
no full precision corpus or local race validation is credited.

## Candidate-owned ordinary resource completion budgets

`gohawk-dho.43` resolves the ordinary-method budget gap identified above.
`releasesResource` and `releasesOrdinaryResource` now belong to the existing
`resourceAnalysis` context instead of accepting parallel evidence, knowledge,
storage, owner and cleanup arguments. The optional-acquisition branch retains
its exact-phi policy and selects no generic helper completion. Ordinary storage
queries still receive `QueryBudget`; each method completion independently draws
`releaseSearchBudget` (250,000 steps) from the same candidate pool. The pool
remains 1,000,000 steps and supplies its observer. It bounds these selected
queries together, not every transitive query in the acquisition proof.

The query order, target, methods, coverage, imported mask and strict projection
mapping are unchanged. Exhausting the shared total now keeps ordinary completion
unknown, as already established by dho.42, rather than allowing a standalone
search beyond that total. This can conservatively lose diagnostics or exact
cleanup conclusions for expensive candidates. The pre-acquisition deferred
may-release query retains its separate bounded allowance and uncertainty;
no publication schema or unconditional declaration guarantee changes.

The actual classifier regression creates an acquisition and local cleanup
helper in SSA, then exhausts the candidate pool before classification. The
parent still returns settled and exposes no method-completion give-up through
that pool. The current returns unknown/budget-exhausted and records exactly
one completion observation at the helper call, naming Close. Replaying the
final test against an overlay of parent classifier/contracts source fails this
assertion (0.039 seconds); its larger-helper control passes. Corrected focused
tests pass (0.030 seconds), including a helper with more than QueryBudget
instructions in actual SSA. This prevents accidentally using the storage
allowance for completion. The resource analyzer tests pass (24.290 seconds).
Actual small-fixture SSA is `.build/goal-completion-pool.ssa.txt`, with empty
stderr; the larger fixture checks its SSA size directly in the regression.

Parent production source is `a4c1e78`, retained as
`.build/goal-completion-pool-parent`, SHA-256
`cd5cd67cd8eb1efc0788cbda344698894dd9d7a1bc4bb298a798321ef61d93d2`.
Corrected `.build/goal-completion-pool-current` has SHA-256
`fc076736eaed5e778cbe3ed912e48d4d413d414fb93fc727f7d62cf65979dec7`.
Hashes identify exact executables rather than clean-tree Git metadata. Both
all-check resource fixture scans exit 3 with empty stderr and identical
335,147-byte diagnostic JSON. Their 5,858 labels and 1,029 final decisions match
as multisets. Total trace events increase from 42,934 to 52,918 because ordinary
helper give-ups now reach the candidate observer. No equivalence of every
trace event is claimed. Receipts use `.build/goal-completion-pool-{parent,current}.*`;
the final parent overlay test is `.build/goal-completion-pool-final-parent-test.log`.

The broader classifier review remains active in `gohawk-dho.40`; unreviewed
local collection, captured-cleanup and optional-acquisition bodies and other
transitive budget ownership are not certified by this fix. The 15-site FP
queue is unchanged. No FP correction, complete architecture consolidation,
full precision-regression or local race run is credited.

Canonical `make verify` passes generation, formatting, vet, lint, dead-code,
local dogfood and ordinary tests (64 seconds); its receipt is
`.build/goal-completion-pool-verify.log`. Documentation architecture checks on
the added inventory pass (0.563 seconds), recorded in
`.build/goal-completion-pool-docs.log`.

## Resource/cancellation classifier review disposition

The remaining local evidence families in `gohawk-dho.40` were read at
`30631c8`. This finishes that finite input-to-proof inventory; it does not
finish the repository-wide consolidation audit. The review used source
fallback because graph tools are unavailable. Previously recorded child
corrections remain authoritative: cancellation return caching (`dho.41`),
resource exhaustion labels (`dho.42`) and ordinary completion budget ownership
(`dho.43`). No additional behavior change or FP correction is credited here.

| Inspected source family | Inputs, proof route and disposition |
| --- | --- |
| `resourcelifetime/local_collections.go` | `resourceAppends` requires an explicit appended acquisition (with only the selected interface boxing); `findLocalCollection` requires all append results to belong to one `SliceVersions` closure, a fresh origin and every version use to be understood. Whole-slice returns transfer; exact whole-slice helpers and complete element-release loops settle at their call/exit edge. Any other use declines the entire model and leaves ordinary append uncertainty. This is collection coverage, not scalar identity or a guessed loop count. |
| Selected collection adapters | `ssaflow/slice_elements.go:AppendedValues/SliceVersions/RangeElementLoop/ReadsElement` supply SSA append/version/index mechanics. `lifecycle/completion_element_releases.go:ElementLoopReleasesEach` is already shared by caller collections and summary inference: release must dominate every backedge, and element uses must match the cleanup receiver. `lifecyclefacts/element_discharges.go:ReleasesEachElement` selects an unconditional each-element discharge or a local same-package inference. No second per-element policy was introduced downstream. |
| `resourcelifetime/captured_cleanup.go` | `opaqueClosureCall` combines possible captured-cell cleanup, narrowly guarded HTTP Body cleanup, aggregate capture, asynchronous invocation and unreadable/retaining callees. `provePriorCleanupWithin` supplies the same uncertain cell/parent-cleanup boundaries before acquisition. A cleanup witness for a cell proves possible cleanup, never which stored acquisition was released. Deferred by-value arguments and read-only captures remain outside this cell contract. |
| Guarded captured HTTP cleanup | `proveGuardedCapturedBodyWithin` requires current stable content equal to this acquisition, and both caller/callee exposure checks must pass under one candidate allowance. `proveGuardedBodyCoverageWithin` uses Body-load identity and every-normal-return coverage with a nonnil assumption. Exhaustion cannot make that narrower positive witness succeed. This differs from cancellation's written-once cell/direct-defer ownership proof; a common broad capture traversal would erase the distinction. |
| Selected coverage adapter | `lifecycle/completion_search.go:MethodCallCoverage` delegates every-return coverage to the existing return/action witness and shared obligation walk, while anywhere coverage is an existential instruction witness. Captured-cell callers request anywhere coverage only for unknown classification; the guarded Body proof requests every-return coverage. These polarities remain explicit at their callers. |
| `resourcelifetime/optional_acquisition.go` | `proveOptionalAcquisitionWithin` requires one acyclic diamond, exact resource and paired-error phis, nil alternate edges and a repeated equality of the same operands. It pairs phi values with predecessor blocks through the shared adapter. Only the acquired merge successor is selected, and cleanup must target the exact resource phi through the selected transparent wrappers. Generic existential derivation and helper/edge completion remain excluded. |
| SQL parent/context classifier boundaries | `sql_parents.go:proveSQLParentCleanupWithin` and `contracts.go:cancelsTransactionContext` require known database/sql symbols and the exact receiver or paired context-constructor cancel. They yield uncertainty about parent-owned/asynchronous cleanup, never synchronous child release. `proveSQLParentIdentityWithin` uses shared point-in-time storage identity under the caller allowance, retaining the storage child cap. Resource SQL lifetime and cancellation-owner policy remain separate. |
| Constant comparison boundaries | Optional acquisition's `sameExactOperand` accepts SSA identity or equal nonnil constants of identical static types. `ssaflow/flow_paths.go:sameLiteral` compares return literals within one declared result and accepts nil literals; lock scalar comparison additionally requires equal constant kinds and its own bindings. These are different input/precision contracts, not a candidate for a universal equality predicate. The optional nil-constant boundary is retained, not silently widened. |

The local collection origin query accepts nil or a local MakeSlice; the code
checks freshness rather than asserting a particular initial length. Its release
proof covers every element of the exact range, independent of the length.
This distinction is recorded explicitly rather than using the nearby word
"empty" as stronger evidence than the implementation supplies.

Inspection covered the complete local collection, captured-cleanup and
optional-acquisition files, plus the selected shared entry bodies named above.
It did not cover every transitive range-counter/natural-loop helper, every
summary-inference function, every storage/retention engine or every acquisition
contract. Collection budgets and selected adapter behavior were inspected;
no global claim that every candidate query shares one pool is made. The
broader normalized-scan dispositions also retain their previously stated
scope and limitations.

This review's concrete mismatches are fixed in the three pushed child commits;
the remaining inspected comparison families have distinct-contract dispositions.
`gohawk-dho.44` now owns the next finite review: goroutine obligation discovery
and spawn classification. Process pre-Start/merged-command policy, deferred-loop
classification, producer protocol reasoning and lock evidence remain in the
larger audit's open scope. The 15 production FP locations and separate Rune
publication issue remain unresolved. Closing dho.40 certifies this finite
inventory and its child fixes, not complete architecture consolidation or the
absence of differently structured duplication elsewhere.

### Goroutine return classification consolidation

`gohawk-dho.44.1` follows the finite spawn-classifier review. Source fallback
found that `returnObligation` ran outside the cached instruction classifier in
both `spawnAnalysis.prove` and its guarded non-nil retry. The shared obligation
walk visits return instructions before its optional return callback
(`internal/ssaflow/flow_obligation.go`); therefore return ownership can use the
ordinary classifier without introducing another flow query or acceptance rule.
The selected source scope is those two flow calls, `classify.go`, its
containment adapters in `carries.go`, reason definitions and local tracing tests.
Graph tools remain unavailable; this is not a complete transitive-engine audit.

Actual SSA for `returnlabels.mergedReturn` has two predecessors at its shared
return, loading the captured channel and changing its direction before return
(`.build/goal-goroutine-return.ssa.txt`). The parent proof accepts that handoff
but emits no return label. The new trace regression fails on the parent with
zero labels while both final decisions remain present
(`.build/goal-goroutine-return-parent-test.log`, 0.124 seconds). Returning an
unrelated channel remains a diagnostic control. Return policy now runs through
`spawnAnalysis.action`, sharing its query cache and one label per instruction
across branch states and the guarded retry. Existing aggregate-projection
opacity is traced as `returned-signal-projection`; containment transfers are
traced as `returned-tracked-value`. The existing final outcomes are retained.

Scoped CLI scans of `goroutineownership`, `summaryjoins`, `processexit` and
`returnlabels` use the fixture GOPATH and select the goroutine check. Parent
binary `.build/goal-goroutine-return-parent` has SHA-256
`3649e54a79ab7af23aed663eb22a5094b104aa2aab2f70ffbbb7fe9ef24d282c`;
current binary `.build/goal-goroutine-return-current` has SHA-256
`04c6709f19705c176f2b6b9e8793112037cb853ac3cd8adcbd95901c711aa979`.
Their JSON diagnostics are byte-identical (115,686 bytes), both exit 3 with
empty stderr, and all 490 decision events agree as a multiset. Labels increase
from 175 to 188 as previously untraced returns become visible. This establishes
fixture diagnostic and final-decision preservation, not all-event equivalence
or production FP removal. No full precision corpus replay is run.

The same review found a separate proof-strength question: broad `consumes`
containment is promoted to exact transfer by returns and some stores, while
`carries.go` describes over-approximation as opacity-only. `gohawk-dho.44.2`
tracks the actual-SSA and consumer assessment needed before changing that
policy. The broader discovery/classifier review remains open in
`gohawk-dho.44`; the 15 production FP locations receive no credit from this
mechanical consolidation.

Validation: the focused merged-return regression passes in 0.292 seconds.
The first ordinary package run exposed the missing stable reason assertions;
they now cover both new labels. The first repository gate passed ordinary tests
in 70 seconds but failed the new test's complexity limit; extracting its label
assertion fixes that. A subsequent formatting failure was corrected with the
canonical `make fmt`. Final `make verify` passes with ordinary tests, formatting,
vet, lint, dead-code, generation and local dogfood
(`.build/goal-goroutine-return-final-verify.log`). No local race run or full
precision-regression replay was performed.

### Goroutine transfer proof strength

`gohawk-dho.44.2` corrects the transfer polarity exposed by the classifier
review. The finite source fallback covers `classify.go`, `carries.go`,
`spawnAnalysis.prove`, the reporter and selected `Storage.Same` identity
adapter. The reporter accepts both unknown and honored ownership; broad
containment was nevertheless promoted to exact transfer before the flow.
Graph tools remain unavailable, and the transitive heap engines are not fully
reviewed by this item.

Actual SSA (`.build/goal-transfer-strength.ssa.txt`) shows a phi between the
completion channel and another channel, a copied aggregate with its channel
field subsequently overwritten, and a wrapper that returns an empty struct
without retaining its argument. The parent declares every one honored. Mixed
and overwritten aggregate stores also receive `join-proven`. The focused
proof regression fails on those five parent outcomes in 0.192 seconds
(`.build/goal-transfer-strength-parent-test.log`). These fixtures remain
accepted by the reporter: uncertainty is not a new diagnostic.

Returns and external stores now share `transferAction`: `Storage.Same` must
prove identity with a tracked value for exact transfer credit. Possible
containment and incomplete budgeted identity stay unknown. Local stores remain
non-actions until handoff. A returned exact handle dominates an opaque sibling
result, so returning a completion handle beside an ambiguous aggregate still
honors the obligation. GoMock publication with broad argument containment is
also unknown: publishing configured results does not establish that they
contain the exact stream. This uses existing identity infrastructure and adds
no aggregate ownership solver or callee-name exception.

Fixture CLI scans select goroutine ownership in the fixture GOPATH over
`goroutineownership`, `summaryjoins`, `processexit`, `returnlabels` and
`transferlabels`. Parent `.build/goal-transfer-strength-parent` SHA-256 is
`82f3e4443341ad16b430b3296064057158c0608bd0ca3f3e976716f99ee63584`;
current `.build/goal-transfer-strength-current` SHA-256 is
`4fa348685c3a0578789ed8371299689e03f127397a4d880180cee7cc120d9978`.
Both exit 3 with empty stderr and identical 115,686-byte JSON diagnostics.
Both have 497 final decisions and 195 labels. Ten decisions change from
`join-proven`/accepted to `opaque-ownership-transfer`/unknown: the five new
controls and existing returned deferred-group, returned closure, returned
WaitGroup callback, mutable stored callback and nested mutable returned
callback cases. Every other decision agrees. Exact-store, merged exact-return,
exact-with-opaque-sibling and unrelated-channel diagnostic controls retain
their outcomes. No production FP removal or all-event equivalence is claimed.

A distinct review item, `gohawk-dho.44.3`, tracks broad `bindingCarries`
feeding helper joins and `MayAliasAny` feeding direct WaitGroup waits. Those
joins were located in source but are not certified by transfer-only controls.
The broader finite discovery/classifier review remains open in `gohawk-dho.44`.

The supplemental GoMock fixture uses a local stub of the documented `Call.Return`
symbol. Its scoped parent/current scans both exit zero with identical `{}`
diagnostic JSON and empty stderr (`.build/goal-transfer-strength-*-mock.*`).
The parent credits the configured result as an accepted transfer and ends with
`join-proven`; the corrected label and final decision are unknown. The final
focused proof regression, now including this contract and exact-handle-plus-
opaque-sibling controls, passes in 0.350 seconds.

Pinned production control containerd/stargz-snapshotter at
`624678b4e421947534cbf0618f9609853cccee0f` has a clean checkout. Parent/current
static scans of `./store`, using `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly` and
`GOWORK=off`, both exit 3 with identical nonempty 827-byte JSON and empty
stderr (`.build/goal-transfer-strength-stargz-*.json`). The reviewed worker
TP at `store/manager.go:193:2` is retained. Candidate tests, generators and
applications were not run; this is a scoped control, not a corpus replay.

Validation: the initial full changed-package run passes in 22.016 seconds;
the first canonical `make verify` passes all ordinary checks, including tests
in 108 seconds. After adding the GoMock contract control, final `make verify`
also passes (`.build/goal-transfer-strength-final-verify.log`), covering the
current source and fixtures with canonical formatting, vet, lint, generation,
dead-code and local dogfood. Documentation conformance is checked separately
again after recording these receipts. No local race or full precision corpus
replay is run. The production FP queue remains 15 locations.

### Goroutine join bindings at call boundaries

`gohawk-dho.44.3.1` corrects exact join credit from possible call-site binding.
The finite source fallback covers `classify.go` direct receiver acceptance,
helper argument/capture binding, `Storage.Same`, the existing summary join
adapter, and the fixture proof harness. Graph tools remain unavailable. The
internal helper search and `isSignal` were read far enough to identify their
may-relation use, but are not certified by this call-boundary correction;
`gohawk-dho.44.4` owns that separate review.

Actual SSA (`.build/goal-join-binding.ssa.txt`) supplies a phi between the
worker's channel and another channel to a receiving helper, passes an aggregate
whose channel field was overwritten, and calls Wait on a phi between the
worker group and another group. The parent calls each `join-proven`. The new
proof regression fails for all three in 4.254 seconds
(`.build/goal-join-binding-parent-test.log`). A supplementary owner-receiver
control likewise calls Close on a phi of the captured object and another
object (`.build/goal-join-binding-owner.ssa.txt`).

Direct WaitGroup and lifecycle acceptance now requires the shared storage
identity proof for the receiver. Possible receiver identity remains unknown,
with a stable `possible-join-receiver` label. A source-visible helper's
must-join reaches the worker only after exact identity matches the supplied
argument or capture to the tracked value; past containment and aggregate
projections preserve unknown helper use. The existing exact summary adapter
remains authoritative before that fallback. Exact helper and WaitGroup controls
still honor the obligation, while the unrelated-channel diagnostic remains.

The repeated fixture-to-proof loops in concurrency, transfer and new join
binding tests now share `assertSpawnProofs`. It runs the diagnostic contract
harness, checks the first launch's authoritative proof and requires every named
case to be found. Later waiter launches retain their separate obligations;
existing summary evidence tracing remains checked by its original assertion.
Focused concurrency, transfer and join proof controls pass together in
8.894 seconds; the supplementary mixed-owner control passes in 4.960 seconds.

Scoped fixture CLI scans select goroutine ownership over `goroutineownership`,
`summaryjoins`, `processexit`, `returnlabels`, `transferlabels` and `joinbindings`
in the fixture GOPATH. Parent `.build/goal-join-binding-parent` SHA-256 is
`662fed52e2e59ee5ecde2bb8b3ce99a1ce97182138e6405a55080a5a171d014e`;
current `.build/goal-join-binding-current` SHA-256 is
`eda518c15c0381ba6768066283f5856d7c7d452926452f66e846dfb109e1fe24`.
Both exit 3 with empty stderr and identical 116,512-byte diagnostic JSON
(`.build/goal-join-binding-*-final.json`). Both emit 506 final decisions and
202 labels. Five final decisions change from `join-proven`/accepted to
`opaque-ownership-transfer`/unknown: the four new uncertain-binding controls
and the included dependency function `testing.runExample`. Every other final
decision agrees. The two uncertain direct receivers emit the new unknown label
with their launch candidates. These scans precede the final mechanical move
of that label's reason selection into the direct classifier; final validation
checks the resulting code below. No production FP removal is credited.

The final unrelated-channel control receives from a closed channel, making its
reported normal return feasible. With this fixture and direct reason selection
moved into `directJoinAction`, final binary `.build/goal-join-binding-final`
SHA-256 is
`4eb4ca56a347235bdf73306da60aa877777ca2a2dddacd0c4e012dc13b0adf36`.
Final scoped scans against the same parent again exit 3 with empty stderr,
identical 116,512-byte JSON, 506 final decisions and 202 labels; the same five
outcomes change to unknown (`.build/goal-join-binding-parent-control.*` and
`.build/goal-join-binding-final.*`). The final focused join-binding regression
passes in 16.275 seconds.

Pinned production control containerd/stargz-snapshotter remains clean at
`624678b4e421947534cbf0618f9609853cccee0f`. Parent and the pre-reason-relocation
corrected binary scan `./store` with `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`
and `GOWORK=off`; both exit 3 with empty stderr and identical nonempty 827-byte
JSON (`.build/goal-join-binding-stargz-*.json`). The reviewed worker TP at
`store/manager.go:193:2` is retained. These are static scans, with no candidate
tests, generators or applications executed. No full precision corpus replay
or local race run is performed. The 15 production FP locations are unchanged.

The canonical gate initially passed ordinary tests in 132 seconds but found a
complexity regression in the call dispatcher. Returning the action and stable
reason together from `directJoinAction` removes that second reason-selection
branch. `make verify` then passes all checks, including ordinary tests in
72 seconds (`.build/goal-join-binding-final-verify.log`). A focused trace test
also checks one label per mixed WaitGroup/mixed owner/exact WaitGroup receiver,
with stable phase, reason, outcome, classifier label and candidate association.
The final gate is rerun for that added trace control below.

Exact receiver identity does not itself prove that an arbitrary lifecycle
method observes worker completion. That remaining contract question, including
name-based Close/Stop acceptance, is explicitly tracked in `gohawk-dho.44.5`.
This call-boundary correction establishes a necessary identity gate, not a
complete lifecycle-method semantics model. `gohawk-dho.44.3` and the broader
finite classifier review remain open pending their outstanding proof scopes.

The focused trace regression passes in 4.919 seconds. Final `make verify` with
that control passes (`.build/goal-join-binding-trace-verify.log`), covering
canonical formatting, generation, vet, lint, dead-code, ordinary tests and local
dogfood. Documentation conformance is rechecked after recording the receipts.

### Lifecycle-method participation versus worker completion

`gohawk-dho.44.5` reviews lifecycle-method acceptance in the spawn classifier.
Source fallback covers `directJoinAction`, helper-effect projection, the
helper search's tracked-kind contract, `callbackClosesSibling`, guarded-join
consumers and the final worker reporter. Graph tools remain unavailable.
Retained callback cleanup uses positive owner-method coverage only to identify
cleanup participation and returns an unknown worker action. Guarded helper
coverage also supplies only an unknown final proof. Neither requires converting
owner method coverage into an exact worker join.

Actual SSA for `ownerparticipation.directClose` loads a captured object whose
Boolean field feeds a worker's blocking send; the parent calls a no-op Close.
The method neither observes nor completes that channel. The visible helper
`closeOwner` only invokes that method and returns
(`.build/goal-owner-participation-direct.ssa.txt` and
`.build/goal-owner-participation-helper.ssa.txt`). Parent proof controls fail
in 0.133 seconds for direct and deferred Close, helper Close, and no-op Stop,
Shutdown, Wait and Kill: all were called `join-proven` despite no completion
observation (`.build/goal-owner-participation-parent-test.log`).

The direct classifier now preserves these existing accepted patterns as
unknown `owner-lifecycle-participation`. Exact identity does not turn a method
name into worker completion. The helper classifier has one `boundHelperAction`
projection: positive owner coverage becomes unknown; a channel/group effect
needs exact supplied-value identity to become a join. Helper search still
answers coverage for the requested tracked kind, so the retained sibling
cleanup query keeps its owner-method coverage without inventing a worker join.
No parallel cleanup search, structural owner solver, or naming exemption is
added. Helper-body may-derivation remains the separate `gohawk-dho.44.4` scope.

`ownerparticipation/owners.go` pairs misleading no-op method names with a real
completion receive and an unrelated owner diagnostic. Exact WaitGroup controls
remain in `joinbindings`; local/imported summary join controls remain in
`summaryjoins`. Focused owner, join-binding, receiver-trace and concurrency
proof tests pass together in 12.465 seconds. Extended trace coverage checks
one unknown label for direct, deferred and helper owner cleanup as well as
exact and uncertain group receivers; focused owner/trace tests pass in
3.826 seconds.

Fixture GOPATH CLI scans cover `goroutineownership`, `summaryjoins`,
`processexit`, `returnlabels`, `transferlabels`, `joinbindings` and
`ownerparticipation`. Parent `.build/goal-owner-participation-parent` SHA-256:
`b6b3108adc454709e5d671716cc9fe86a8d27f17c454e6c9ad0974d5edbae6a0`;
current `.build/goal-owner-participation-current` SHA-256:
`b4a4b9e652a8b361878c361de4eaa039a3104bb71fc981b0df3bbf3c2f1b5479`.
Both exit 3 with empty stderr and identical 117,372-byte JSON diagnostics.
Both have 516 final decisions and 211 labels. Eight final decisions become
`opaque-ownership-transfer`/unknown: the seven new uncertain method controls
and the existing deferred asserted-owner fixture previously accepted as
`deferred-join-before-spawn`. Every other final decision agrees. The actual
receive and unrelated-owner diagnostic retain their outcomes. No production
FP removal or all-event equivalence is credited.

The early caller-owned stop/context branch of `lifecycleProof` was also read
and located: it returns honored for a lifetime bound before consulting local
flow, while local/receiver context bounds are already unknown. Its distinct
caller-transfer contract and actual SSA need their own review, now tracked in
`gohawk-dho.44.6`. The finite obligation/classifier review is not complete.

Pinned production control containerd/stargz-snapshotter is clean at
`624678b4e421947534cbf0618f9609853cccee0f`. Parent/current static scans of
`./store` with `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly` and `GOWORK=off`
both exit 3 with identical nonempty 827-byte JSON and empty stderr
(`.build/goal-owner-participation-stargz-*.json`). The reviewed worker TP at
`store/manager.go:193:2` remains. Candidate tests, generators and applications
were not executed.

`make verify` passes all ordinary gates on the first run
(`.build/goal-owner-participation-verify.log`), including changed-package tests
in 25.874 seconds and the full ordinary suite in 61 seconds, generation,
canonical formatting, vet, lint, dead-code and local dogfood. Documentation
conformance is rechecked after recording these receipts. No full precision
corpus replay or local race run is performed. The 15 production FP locations
receive no correction credit from this proof-strength change.

### Caller lifetime bounds versus completion-handle transfer

`gohawk-dho.44.6` reviews `lifecycleProof`, `callerSuppliedValue`,
`spawnedParameterIsReceived`, `bindingIsExternallyOwned`, `receivesAnywhere`
and `workerReceiveSearch`. Graph tools remain unavailable, so this is a finite
source fallback, not a complete transitive review of storage or lifecycle
engines. The receive search is keyed by function and local value and finds a
receive on any path. Its may-derivation, call bindings, recursion and budget
rules provide possible lifetime evidence, never completion before parent return.

Actual SSA shows the raw stop/context workers receiving before sending on a
separate fresh completion channel. The explicit helper form supplies a
read-only channel argument to the worker, which calls `receiveStop(input)`
before the independent completion send
(`.build/goal-caller-bound.ssa.txt` and
`.build/goal-caller-bound-final.ssa.txt`). The captured pointer-to-channel helper
form does not pass the existing channel type guard. Its uncovered-send
diagnostic is retained; no matcher widening or FP removal is credited.

Caller stop/context bounds now return `GoroutineUnknown` with their existing
reasons. The existing completion-ownership loop runs before these weaker
bounds, once, preserving its internal factory-opacity, external-transfer and
opaque-group ordering. A caller-owned completion channel or group remains
`GoroutineTransferred`; an incoming stop/context input cannot replace that
contract with an honored join. Local/receiver context, synctest and exhaustion
outcomes remain unchanged. No second acceptance engine or traversal is added.

`callerbounds/bounds.go` pairs raw/helper lifetime bounds, caller-owned
completion handles, an exact local receive, ignored context, local stop and the
unsupported captured-cell helper. The initial parent proof test fails in
4.516 seconds; the captured helper mismatch is investigated and retained as a
diagnostic control. Final proof controls pass in 4.864 seconds, and combined
proof/trace controls pass in 9.342 seconds. The trace requires one authoritative
decision with its candidate and position association for each selected case.

Final fixture CLI scans cover `goroutineownership`, `summaryjoins`,
`processexit`, `returnlabels`, `transferlabels`, `joinbindings`,
`ownerparticipation` and `callerbounds`. Parent binary SHA-256:
`ad6de69692a2e055f888118ac5704af57a656c7748a48174dc44af483a675b9b`;
current binary SHA-256:
`3c3d6d97509225864dba5524294265344edb62e15192d9f133b80ae2c2306aa5`.
Both exit 3 with empty stderr and identical 119,758-byte JSON diagnostics
(`.build/goal-caller-bound-*-final.json`). Both have 529 final decisions and
212 labels. Comparing decision multisets by function, check, candidate and
position avoids dependence on concurrent trace order. Seven decision keys
change: four new caller bounds and the existing unobserved-display stop bound
become unknown; the new caller channel/group controls retain acceptance with
the external-owner transfer reason. Every other final decision agrees. This
does not claim equivalence of all trace events.

Pinned stargz-snapshotter remains clean at
`624678b4e421947534cbf0618f9609853cccee0f`. Parent/current static `./store`
scans both exit 3 with empty stderr and identical 827-byte nonempty JSON
(`.build/goal-caller-bound-stargz-*.json`). The reviewed worker TP at
`store/manager.go:193:2` remains. Candidate tests, applications and generators
were not run.

The first `make verify` passes ordinary tests in 66 seconds but fails a
test-only modernize lint check. The test uses `bytes.SplitSeq` in the final
revision. Final `make verify` passes all gates, including ordinary tests in
57 seconds, canonical formatting, generation, vet, lint, dead-code and local
dogfood (`.build/goal-caller-bound-final-verify.log`). Documentation conformance
also passes after recording these receipts. No full precision corpus replay
or local race run is performed. The production FP queue remains 15 locations
and the wider `gohawk-dho.44` review remains incomplete.

### Exact and possible completion observations

`gohawk-dho.44.4` covers the direct/selected receive classifier, every source
consumer of `isSignal`, counted drains, guarded joins and helper-internal
receive/Wait/recursive binding. Graph tools remain unavailable. Source fallback
reads those functions, the shared storage identity and may-derivation contracts,
and the edge trace projection. This is a finite consumer review; it does not
recertify the entire points-to graph or every obligation-discovery engine.

Actual SSA in `.build/goal-receive-identity-pilot.ssa.txt` shows a receive phi
choosing the worker's completion channel or a separate closed channel. The
helper receive and WaitGroup controls make the same choice inside the callee,
after an exact call-site binding. The parent calls all three `join-proven`.
Expanded storage SSA shows the worker sending through field zero while the
parent receives from field one, and a helper storing its supplied channel into
a cell before overwriting that cell with a different channel
(`.build/goal-receive-identity-storage.ssa.txt`). Both also have parent accepted
join decisions. Selected-edge SSA shows distinct return arms for the mixed
handle and the exact handle (`.build/goal-receive-identity-edge.ssa.txt`).

Direct positive receive queries now require `Storage.Same` for a channel-valued
tracked signal. The previous may-alias/aggregate-root matcher remains separate
as `possibleSignal`, yielding unknown observations on the same direct or
selected paths. A default or unrelated arm never inherits that observation.
The counted drain retains its single-sender/count proof and returns exact,
unknown or no edge action according to handle identity. Guarded-join queries
retain the possible matcher because their result supplies only uncertainty.

Helper coverage similarly distinguishes exact storage identity from possible
derivation for completion channels and groups. One `instructionJoins` query
accepts the caller-selected match policy for direct observations and nested
bindings. A possible completion witness keeps the helper unknown; only exact
coverage of every normal return yields a join. One `receiveEdgeAction` supplies
both the initial scan and every-return coverage. Owner-method coverage remains
possible lifecycle participation, projected as unknown by worker consumers;
it is not upgraded into completion. Existing memoization, recursion, search
allowances and summary coverage contracts remain in place.

Edge evidence now stores the authoritative action together with its reason.
The emitter projects that action, removing its independent list of reasons
that should be traced as unknown. The receive trace checks one classifier label
per direct/helper control and the candidate-associated unknown selected edge.
`receiveidentity/receives.go` pairs mixed, sibling, overwritten and nested forms
with exact direct/helper observations and unrelated/default-arm diagnostics.

Initial parent proof controls fail in 4.441 seconds for four mixed forms.
The initial corrected analyzer package suite passes in 42.624 seconds. Final
focused proof, edge/label trace, helper budget/memo and concurrency-summary
controls pass in 13.438 seconds. A trace test initially counted two separate
proof instances because the proof assertion intentionally creates a fresh
analysis after the normal run; it now uses one normal analyzer run to verify
the cache, independently of the proof-strength test.

Final fresh-path CLI receipts cover `goroutineownership`, `summaryjoins`,
`processexit`, `returnlabels`, `transferlabels`, `joinbindings`,
`ownerparticipation`, `callerbounds` and `receiveidentity`. Parent SHA-256:
`09e1e0ab02000f5f98f657b2746cec6034d6a47f76dc3d29d726a2537099dcf4`;
current SHA-256:
`897c21854dea7c485b40bf700c22f7c2d00c2d3a4d2f732af32d187639d826a2`.
Both exit 3 with empty stderr and identical 121,398-byte diagnostic JSON
(`.build/goal-receive-identity-*-receipt.json`). Each has 543 final decisions
and 225 labels. Comparing decision multisets by function, check, candidate and
position finds ten changed keys: eight new uncertain observations change from
accepted to unknown; two existing slice controls remain unknown, now with
`shared-storage-signal` rather than `loop-join-unproven`. All other final
decisions agree. Earlier reused `*-final.trace.jsonl` paths appended events
from multiple runs and are superseded by the fresh `*-receipt.trace.jsonl`
artifacts; they are not comparison evidence. No all-event equivalence or
production FP removal is claimed.

The first validation gate catches helper-search cyclomatic complexity at 25;
the consolidated selected-edge action removes the duplicated matching query.
Final `make verify` passes all gates, including ordinary tests in 104 seconds,
canonical formatting, generation, vet, lint, dead-code and local dogfood
(`.build/goal-receive-identity-final-verify.log`). Documentation conformance
also passes after recording the evidence. The clean pinned stargz checkout at
`624678b4e421947534cbf0618f9609853cccee0f` has identical nonempty 827-byte
parent/current `./store` diagnostics, exit 3 and empty stderr. The reviewed TP
at `store/manager.go:193:2` remains
(`.build/goal-receive-identity-stargz-*.json`). Scans disable CGO and workspace
and use read-only module mode. Candidate tests, generators and applications are
not run. No full precision corpus replay or local race run is performed. The
production FP queue remains 15 sites; the broader architecture and
obligation-discovery completion claims remain unproven. In particular,
receiver matching before asynchronous launch guards requires a separate
completion-contract review, beyond the identity correction recorded here.

### Launched observers versus caller completion

`gohawk-dho.44.7` reviews launch polarity in `callAction`, `directJoinAction`,
helper `instructionJoins`/`callEscapes`, `proveSummaryJoin` and returned-waiter
composition. Graph tools remain unavailable; this is finite source fallback,
not certification of every transitive lifecycle or concurrency engine.
Actual SSA in `.build/goal-async-wait.ssa.txt` shows the parent launching
`(*sync.WaitGroup).Wait` and returning without observing it. The helper
`launchWait` likewise launches Wait and returns. Both parent traces credit
the worker as `join-proven`; the parent proof test fails for both in
5.315 seconds (`.build/goal-async-observer-parent-test.log`). Exact receiver
identity does not make either observer run in its caller's invocation.

The caller classifier now handles launches before synchronous receiver,
summary or library contracts. Positive argument/capture consumption yields
unknown `launched-helper`; an unrelated launched call yields no worker action.
`opaqueCallAction` consolidates the identical consumption boundary for launched
and unreadable calls. The helper search rejects launches before receiver
coverage, and tests their handoff before read-only receiver bookkeeping. Its
`helperCallCarries` shares the argument/capture query for launched and opaque
callees. The existing summary join rejects Go instructions already; its
synchronous/deferred contract remains unchanged. No new search, scheduling
heuristic or completion mode is added.

`asyncobservers/waits.go` pairs direct/helper-launched group observers with
synchronous, deferred and helper-deferred waits, a launched visible helper, and
an unrelated observer whose worker still reports. Focused observer, helper and
concurrency controls pass in 9.608 seconds. Expanded observer/receiver trace,
helper memo/budget and concurrency controls pass in 18.318 seconds. One shared
`assertClassifierLabels` assertion replaces the repeated receiver/observer
trace loop, requiring exact reason, outcome, label, candidate, position and one
label per selected instruction. Proof outcomes remain independently asserted
through `assertSpawnProofs`.

Fresh fixture CLI receipts cover `goroutineownership`, `summaryjoins`,
`processexit`, `returnlabels`, `transferlabels`, `joinbindings`,
`ownerparticipation`, `callerbounds`, `receiveidentity` and `asyncobservers`.
Parent binary SHA-256:
`fb26673f17560128f30dedc7818e677dc1458f5c61dec34e083a7c84b54c9114`;
current binary SHA-256:
`f12ff6813a6b5b847415f2640c5c7b93fc0cefebbe755914c8fe29e529da9458`.
Both exit 3 with empty stderr and identical 122,234-byte diagnostic JSON
(`.build/goal-async-observer-*-receipt.json`). Each has 555 final decisions
and 231 labels. Comparing decision multisets by function, check, candidate and
position finds only two changed keys: direct/helper asynchronous Wait controls
become unknown `opaque-ownership-transfer` instead of accepted `join-proven`.
All other final decisions agree. No all-event equivalence or production FP
removal is credited.

The first gate passes ordinary tests in 78 seconds but catches classifier
cyclomatic complexity at 21. Consolidating opaque-call consumption removes
that duplicated branch. The next gate catches three accidentally qualified
names in the proof-case table after the trace assertion refactor; those names
are restored to the short names expected by the proof helper. Corrected
`make verify` passes all gates, including ordinary tests in 50 seconds,
canonical formatting, generation, vet, lint, dead-code and local dogfood
(`.build/goal-async-observer-corrected-verify.log`). Documentation conformance
is checked after recording these receipts.

The clean stargz checkout at `624678b4e421947534cbf0618f9609853cccee0f`
has identical nonempty 827-byte parent/current `./store` diagnostics, exit 3
and empty stderr. The reviewed TP at `store/manager.go:193:2` remains
(`.build/goal-async-observer-stargz-*.json`). Static scans disable CGO and
workspace and use read-only module mode; candidate tests, generators and
applications are not run. No full precision corpus replay or local race run
is performed; the unresolved production FP queue remains 15 sites.
The original helper/direct identity scope is now covered by the exact binding,
internal identity, owner participation and launch-polarity corrections. The
wider obligation discovery and architecture completion claims remain unproven.

### Recursive completion handoffs and cache cutoffs

`gohawk-dho.44.8` reviews `helperSearch.use`, recursive coverage and escape
consumers, `CallGraphMemo.Summarize`/`Compose`/`WithFunction`, and the final
helper binding projection. Graph tools remain unavailable, so the evidence is
finite source fallback, actual SSA and focused behavioral controls. It does
not certify every transitive summary consumer or solve recursive execution.

Actual `.build/goal-recursive-observer.ssa.txt` shows `recursiveWait` calling
itself with the same group on one branch and waiting on that group on the base
branch. Its caller supplies one and has a worker settling that group. The
immutable parent reports the worker as unjoined
(`.build/goal-recursive-observer.trace.jsonl`). The parent fixture test fails
in 4.619 seconds for direct and mutual forwarding, and for a recursive cutoff
whose answer was incorrectly `actionNone`
(`.build/goal-recursive-helper-parent-test.log`).

The helper's unavailability policy now returns unknown for recursion as well
as budget exhaustion. The nested call positively carries the tracked value;
an unsupported recursive body cannot establish absence of a completion
handoff. Existing body-unavailable handling remains in the call classifier,
and no positive cleanup witness is invented at that boundary. Shared memo
mechanics remain unchanged: recursive cuts invalidate dependent answers,
which are not retained for future paths. Independent exact observations can
still cover every normal return and yield a join despite an earlier opaque
call. No recursion count, unrolling, new summary component or proof engine is
introduced. This conservative boundary can miss actual omissions behind
recursive forwarding, recorded in the fixture header and design note.

`recursivehelpers/waits.go` pairs direct/mutual forwarding with an explicit
final Wait and an unrelated-group diagnostic. The classifier trace requires
one unknown helper label for each forwarding form and a positive label for
the independent exact Wait. The retry test enters a receive helper on an
active call path, gets unknown, then reaches it on a fresh path and proves its
receive, testing that the cutoff answer is not cached. Focused recursive
proof/trace, retry, helper budget/memo and concurrency controls pass in
14.219 seconds (`.build/goal-recursive-helper-focused.log`). The public page
records the new recursive-helper acceptance; implementation limits stay in
the development design note.

Fresh fixture CLI receipts cover `goroutineownership`, `summaryjoins`,
`processexit`, `returnlabels`, `transferlabels`, `joinbindings`,
`ownerparticipation`, `callerbounds`, `receiveidentity`, `asyncobservers` and
`recursivehelpers`. Parent binary SHA-256:
`ab171dbbe8ce66e54cf309e2ff2dc677ac6275237da74e5e8f9e1a23f5cbe295`;
current binary SHA-256:
`92ea3cd85637cec8be471c26efc0d4a57c51c133d6b93c65b0b732e7670b47c1`.
Both exit 3 with empty stderr. Parent JSON is 124,660 bytes with 134 diagnostic
locations; current is 123,080 bytes with 132. Only the two new recursive
fixture reports disappear; no diagnostics are added, and every other complete
diagnostic agrees (`.build/goal-recursive-helper-*-receipt.json`). Parent has
562 final decision events and 232 labels; current has 560 and 234. Comparing
decision multisets by function, check, candidate and position finds the two
forwarding proofs becoming unknown instead of rejected, with their two
diagnostic-report events removed. Every other final decision agrees. This is
a demonstrated synthetic FP correction, not a removal from the frozen
15-site production queue or an all-event equivalence claim.

`make verify` passes all gates on its first run, including ordinary tests in
72 seconds, canonical formatting, generation, vet, lint, dead-code and local
dogfood (`.build/goal-recursive-helper-verify.log`). Documentation conformance
passes after recording the evidence. The clean stargz checkout at
`624678b4e421947534cbf0618f9609853cccee0f` retains identical nonempty
827-byte parent/current `./store` diagnostics, exit 3 and empty stderr. The
reviewed TP at `store/manager.go:193:2` remains
(`.build/goal-recursive-helper-stargz-*.json`). Static scans disable CGO and
workspace and use read-only module mode; candidate tests, applications and
generators are not run. No full precision corpus replay or local race run is
performed. The wider architecture and obligation-discovery review is still
incomplete; this closes the demonstrated recursive helper-availability gap.


## Nested notification promise coverage

`gohawk-dho.44.9` narrows obligation discovery after the actual parent SSA and
trace demonstrated six ambiguous forms being reported as missing joins:
conditional inner close, conditional inner send, conditional defer registration,
inner progress work, outer progress work and detached inner `go close`.
The immutable parent is source `669c6e3`, executable
`.build/goal-obligation-parent`, SHA-256
`6d4b4d9f8fe83f63a819361544fff97474464b2ab161949cafed137871a77c86`.
The pilot SSA and trace are `.build/goal-obligation-pilot.ssa.txt` and
`.build/goal-obligation-pilot.trace.jsonl`. Graph tools are unavailable; this is
bounded source evidence, not a transitive completeness claim.

| Discovery input | Authoritative selection and boundary |
| --- | --- |
| Direct send | `completionNotification` requires `terminalCompletion`; later work supplies no completion promise. |
| Direct close | The shared selector rejects detached and loop-local closes; `notifiesChannelOnEveryReturn` requires exact channel notification coverage and a return witness. |
| Nested notification | The same selector and channel return coverage apply in the inner closure. The exact outer invocation/registration must cover worker returns; synchronous invocation must also be terminal. |
| Aggregate signal mapping | Existing `signalSuppliedAtCall` mapping remains; its corrected rationale now says possible aggregate observations and handoffs are unknown, never exact joins. |
| Deferred group alternatives | Existing group discovery remains. Two fixtures lose a conditional nested signal but retain their independent group and unchanged proof outcomes. |

This consolidates notification operation selection previously duplicated in
three scans. It also moves the misplaced `terminalCompletion` rationale to the
function it explains. The new `notificationpromises` fixtures check six unknown
outcomes, three unconditional missing-join diagnostics and two exact joined
outcomes. The parent fails all six unknown cases; the corrected focused test
passes. One older close-or-send diagnostic followed by arbitrary deferred
cleanup is removed as an accepted false negative, with the gap in its fixture
header and design note: its actual SSA runs arbitrary cleanup after the send,
so that send cannot promise worker completion.

Fresh 12-package fixture receipts are
`.build/goal-notification-{parent,current}-receipt.{json,trace.jsonl,err}`.
Both scans exit 3 with empty stderr. Diagnostics change from 140 to 134;
JSON sizes are 130,906 and 124,826 bytes. Final decision events change from
579 to 573, and classifier labels from 240 to 236. Six new proofs change from
unowned-return to no-completion-obligation, with their reporting events removed.
Two existing proof details change signals from one to zero while retaining
their group, reason and outcome; all other final decisions agree as multisets.
This comparison excludes the deleted coverage fixture and is not all-event
trace equivalence or a frozen-corpus replay. The corrected immutable binary is
`.build/goal-notification-current`, SHA-256
`ceeb24d29ffd759ef35162a0d580a5a4eabfee5d17ca9c6ba3dc717711a38945`.

No production FP removal is credited. The 15-site production queue and frozen
verdicts remain unchanged. Parent `dho.44` remains open: exact signal/group
mapping, discovery budget ownership and the remaining selected adapter inventory
are not certified by this notification coverage correction. Broader
process/defer/producer/lock review and Rune publication also remain open.


The canonical `make verify` gate passes on the final source, including ordinary
tests (85 seconds), formatting, vet, lint, dead-code and local dogfood;
`.build/goal-notification-verify.log` retains the receipt. Documentation checks
also pass after this inventory update. Pinned clean stargz
`624678b4e421947534cbf0618f9609853cccee0f`, `./store`, scanned statically with
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly` and `GOWORK=off`, yields identical
827-byte JSON, exit 3 and empty stderr before/after. Its reviewed true positive
at `store/manager.go:193:2` remains. Receipts are
`.build/goal-notification-stargz-{parent,current}.{json,err}`. No candidate tests,
generators or applications, full precision-regression corpus or local race run
were executed.


## Exact and stable completion bindings

`gohawk-dho.44.10` follows discovery into worker-to-caller mappings. Source
`9ec8367` pilot SSA and trace demonstrate two false obligations: a terminal
send through a phi of two channel parameters picks the first possible actual,
and a channel reassigned inside a worker maps to its first initializer. The
callers receive the actual signaled channel, yet the parent reports both.
The minimized fixture adds a third demonstrated FP: a capture reassigned before
launch is also mapped to its old initializer. Its latest value is stable and
its caller receives that exact channel. Parent focused tests fail all three;
the corrected test passes, as does the ordinary analyzer package suite
(72.853 seconds on the first implementation).

`completionValueAtCall` now asks existing exact storage identity for worker
bindings. A captured scalar/pointer cell requires read-only worker effects and
`Storage.StableContent` at the spawn; a raw captured struct address retains
object identity. Direct signal and group discovery, deferred group alternatives
and aggregate-root fallback share that mapping. The nested notification route
requires exact read-only capture identity and retains the cell until the outer
launch maps its stable contents. Possible binding identity cannot establish an
obligation; aggregate ownership/element observation remains unknown. No shared
possible-binding helper changes contract, no caller branch specialization is
added, and uncertain forms deliberately permit false negatives.

Caller mapping and aggregate projections now live in `completion_bindings.go`,
a focused evidence family distinct from notification and group promise
selection in `obligation.go`. This also resolves the first local gate's
file-length failure. Its other failure was the public detection paragraph's
ninth line, corrected by shortening that paragraph. Nine new proof-strength
cases cover mixed channel/group parameters, replaced captures, stable snapshots,
exact parameter/capture/nested signal diagnostics and receives, and a group
control. Existing fixtures retain their diagnostics.

The parent executable `.build/goal-discovery-mapping-parent` has SHA-256
`b838817e69f7c7e0fd422f1be14a208ddf5afca3bf0c128ff362e72355f98280`;
actual pilot IR is `.build/goal-discovery-mapping.ssa.txt`, trace
`.build/goal-discovery-mapping.trace.jsonl`. The final executable
`.build/goal-binding-final` has SHA-256
`5a24c7abba52ed7bb374941ec22dcbd1433ec544ca91ebf6537e412d28ef72eb`.
Fresh 13-package fixture receipts are
`.build/goal-binding-{parent,final}-receipt.{json,trace.jsonl,err}`.
Both scans exit 3 with empty stderr, 130,569/128,139 bytes of JSON and 141/138
diagnostics. Decisions change from 589 to 586; classifier labels remain 238.
Three new reporting events disappear. Mixed/replaced signals become unknown
without an obligation; the stable snapshot becomes join-proven. The mixed group
remains unknown with no obligation, and one existing nested-worker transfer
remains unknown while losing uncertain signal/group discovery. All other final
decisions agree as multisets. Final JSON, decision and label multisets also
agree with the pre-extraction implementation; no all-event equivalence is
claimed.

Graph tools remain unavailable: evidence is bounded source inspection and
actual SSA/trace, not exhaustive transitive coverage. Production FP queue and
frozen verdicts remain unchanged at 15 unresolved sites. `dho.44.11` records
the next concrete source gap: discovery budgets are standalone or absent, and
constructor discovery runs before the probe/observer setup. The new mapping's
queries are bounded but not yet charged to the candidate pool. Parent `dho.44`
and the overall architecture objective therefore remain active, along with
broader process/defer/producer/lock and Rune publication work. No production
FP credit is claimed for these synthetic regressions.


Final `make verify` passes after the focused extraction and prose correction;
`.build/goal-binding-final-verify.log` retains all local target receipts.
Documentation checks pass after the inventory update. Pinned clean stargz
`624678b4e421947534cbf0618f9609853cccee0f`, `./store`, scanned statically with
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, yields byte-identical
827-byte JSON, exit 3 and empty stderr in
`.build/goal-binding-stargz-{parent,final}.{json,err}`. Its reviewed
`store/manager.go:193:2` true positive remains. No candidate tests, generators
or applications, full precision-regression corpus or local race run were run.


## Observed positive discovery budget

`gohawk-dho.44.11.1` implements the positive-discovery subset of `.44.11`.
The actual oversized-worker SSA from source `6e6e087` and parent constructor
regression show that discovery could invent a report after an unbounded census.
The parent focused test fails with `unowned-return`. The corrected constructor
sets its candidate probe before queries, draws one `SummaryBudget` allowance
from the existing candidate pool and retains that allowance's availability.
Census steps, exact/stable bindings, group/nested notification coverage,
terminal tails and synchronous callback wrapper invocation share it. The shared
obligation flow's tri-state entry walk supplies coverage; a single adapter
charges examined instructions as well as expanded path states. No second
return-coverage engine is introduced.

A local or candidate-pool cutoff produces whole-candidate unknown with reason
`completion-discovery-budget-exhausted`, even if a positive signal was found
before a later census missed another handle. Constructor follow-up queries
stop then; cutoff evidence is attributed to the spawn and distinguishes pool
exhaustion. The focused tests include oversized real SSA, attributed trace,
a 64-step pool that finds a signal before cutting off, and a fresh full-budget
missing-join proof. No incomplete discovery is memoized. Existing notification
and exact binding controls pass, along with ordinary lifecycle/goroutine package
tests (0.646/69.579 seconds on the first implementation).

`spawn.go` now owns state, probe initialization, allowances and constructor
orchestration; `obligation.go` owns promise selection. This separates two
existing responsibilities and fixes the initial file-length lint failure.
The ordinary spawned invocation engine is now exposed as
`lifecycle.ProveSpawnedInvocation` with an explicit caller budget. Its unused
Boolean facade was removed; existing semantic and cutoff tests now call the
same authoritative structured proof. The derived lifecycle helper inventory
is regenerated. Initial local gates also caught overlong source lines and one
obsolete test-only facade comparison; these are corrected rather than waived.

Immutable parent `.build/goal-binding-final` implements production source
`6e6e087`, SHA-256
`5a24c7abba52ed7bb374941ec22dcbd1433ec544ca91ebf6537e412d28ef72eb`.
Final `.build/goal-discovery-budget-final` has SHA-256
`15f7e0e51bb729d37365fef2f4863d3d70f00f294f033b44ea4c3e9eef19dc1d`.
Actual pilot SSA is `.build/goal-discovery-budget-pilot.ssa.txt`;
`.build/goal-discovery-budget-pilot-{parent,final}.{json,trace.jsonl,err}` retain
its observed change: parent exit 3, 678-byte diagnostic JSON; final exit 0,
2-byte JSON, explicit attributed budget evidence and unknown discovery decision.
Both have empty stderr. This is an intentional accepted false negative for an
oversized worker, not production precision credit.

Fresh 13-package fixture receipts in
`.build/goal-discovery-budget-{parent,final}-receipt.{json,trace.jsonl,err}`
have byte-identical 128,139-byte JSON, exit 3 and empty stderr. They retain 586
final decision events and 238 classifier labels. Three dependency proofs in
`testing` remain unknown but now explicitly name discovery exhaustion;
all other final decisions agree as multisets. No all-event equivalence is
claimed. Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`,
`./store`, statically scanned under `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`,
`GOWORK=off`, retains byte-identical 827-byte JSON, exit 3 and empty stderr in
`.build/goal-discovery-budget-stargz-{parent,final}.{json,err}`. The known
`store/manager.go:193:2` true positive remains.

Graph tools are unavailable; this is bounded source/SSA/trace evidence. Parent
`.44.11` remains active for relay, owner and pipe constructor adapters and
other standalone suppression budgets. The aggregate-root and completion-defer
suppression wrappers still delegate to their single implementation under the
prior policy; they are outside this positive-discovery budget claim.
The imported callback claim fallback reads the existing declaration fact;
`CalleeClaims` does not introduce a second body search. None of these statements
certifies broader transitive helper completeness. Parent `.44`, the wider
architecture goal, Rune publication and the 15-site production queue remain
open. No full precision-regression, local race run or candidate tests,
generators or applications were executed.


Final `make verify` passes with receipts in
`.build/goal-discovery-budget-publish-verify.log`, including generated helper
inventory, ordinary tests, formatting, vet, lint, dead-code and local dogfood.
The last ordinary-test run takes three seconds with passing receipts reused
from prior runs; it is not a fresh three-second performance measurement.
The final reference check also required updating the handwritten shared-helper
map to the structured spawned invocation API. This child can close with the
focused budget contract verified; its parent and the overall objective cannot.

## Exact relay bindings and constructor adapter budgets

Beads `gohawk-dho.44.11.2` follows the positive-discovery correction in
`779ff1b`. Actual pilot SSA shows a captured group initialized from one
WaitGroup, reassigned to a second before launch, and waited on by the caller.
The old relay adapter selected the first initializer and reported the relay
despite the caller's exact wait on its actual group. Relay group and channel
mapping now reuse the existing exact/stable completion binding query. Relay
closes and instruction classification share one exact channel matcher.

Relay resolution/census, owner capture/method selection and pipe worker/binding
search share the constructor's observed discovery allowance. Each cutoff
records its phase and leaves the whole candidate unknown; partial completion
alternatives or owner/peer witnesses cannot establish their absence. The shared
budgeted capture helper retains its possible first-initializer semantics for
suppression consumers; that query does not establish an exact relay binding.
Method-set construction itself is not a claimed wall-clock bound.

Focused fixtures preserve an exact relay, reject an unrelated wait and extra
local blocking work, and retain unknown for caller-owned work. Actual SSA
cutoff tests cover relay, owner and pipe phases with attributed trace evidence;
shared capture tests cover exhausted and fresh allowances. Focused receipts are
`.build/goal-relay-cutoff-test.log`; full affected-package tests pass in
`.build/goal-relay-package-test.log`. The corrected commentary/reference check
passes in `.build/goal-relay-architecture-corrected-test.log`.

Immutable parent `.build/goal-relay-parent`, source `779ff1b`, has SHA-256
`9e9e7e24613210633fc62560442dd2aeffb9a5047f28d11edc4a9cb7c1f297b3`.
Corrected `.build/goal-relay-current` has SHA-256
`9247339c04ff8f920d7a673dcdd28c84d26a856080e6c468b3ace6384e2ffc65`.
The corrected artifact predates the final rationale comment and test formatting
edits; its hash identifies the exact executable scanned. Pilot SSA is retained
in `.build/goal-relay-pilot.ssa.txt`; the parent trace and scan are
`.build/goal-relay-parent.trace.jsonl` and `.build/goal-relay-pilot.scan.txt`.
Corrected `.build/goal-relay-pilot-current.{json,err,trace.jsonl}` has exit 0,
2-byte JSON and empty stderr, replacing the parent's relay diagnostic with
exact join evidence. This is a minimized correction, not production FP credit.

Fourteen scoped fixture packages retain receipts in
`.build/goal-relay-{parent,current}-receipt.{json,err,trace.jsonl}`. Parent has
141 diagnostics in 130,538-byte JSON, 594 final decisions and 241 labels;
corrected has 140 diagnostics in 129,756-byte JSON, 593 final decisions and
242 labels. Both exit 3 with empty stderr. Final-decision multisets differ only
by the reassigned relay's violation/report removal and exact join acceptance;
one exact Wait label is added. No all-event equivalence is claimed.
Pinned stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`, retains
byte-identical 827-byte JSON and its reviewed `store/manager.go:193:2` TP in
`.build/goal-relay-stargz-{parent,current}.{json,err}` (exit 3, empty stderr).
External scans are static, with CGO disabled, readonly modules and GOWORK off.

Graph tools are unavailable; evidence is bounded source, SSA and trace review.
Post-constructor standalone suppression budgets remain parent `.44.11` scope.
The broader architecture goal and 15-site production queue remain open. No
full precision-regression, local race run or candidate tests, generators or
applications were executed.

Final `make verify` passes in `.build/goal-relay-ready-verify.log`, covering
ordinary tests, generation, formatting, vet, lint, dead-code and local dogfood.
Earlier failed runs exposed an overlong test line and missing rationale inside
the pipe query; both are corrected. Follow-ups `.44.11.3` and `.44.11.4` record
the retained-owner and caller-bound/relay-dependency budget review respectively.

## Retained-owner census availability

Beads `gohawk-dho.44.11.3` follows constructor discovery in `939be44`.
Call and selected-context retained-owner queries now draw one observed allowance
for worker resolution, send/output guards, binding enumeration, retained-value
search, cleanup targets and opaque worker tails. Factory returned-cleanup and
legacy sibling callbacks use that allowance instead of a standalone budget.
The shared unreadable-callback engine gains a budgeted capture/referrer/body
census; its default entry point delegates to the same implementation and keeps
possible opaque consumption on exhaustion.

The classifier consumes a structured retained-owner proof. Exhaustion supplies
an unknown label at that call or selected edge, with stable
`retained-owner-budget-exhausted` spelling and attributed evidence. It does not
make an independent early return safe or establish a worker join. Actual SSA
tests exercise that label through the shared obligation flow: a path through
the call is uncertain, while a path bypassing it is violated. Fresh unrelated
owner and context queries remain negative within this policy. Factory tests
distinguish unavailable target discovery from fresh cleanup witnesses; shared
callback tests preserve the existing visible, opaque, derived and unrelated
capture outcomes. Focused receipts are
`.build/goal-retained-budget-focused3.log`.

Reachability collection previously preceded spending. The retained-owner call
and tail checks now use `InstructionsReachableAfterWithin`, whose default
facade delegates the same back-edge policy with no caller allowance. Its SSA
test distinguishes zero allowance, partial census and fresh complete census;
partial instructions never prove an unvisited instruction unreachable. No new
analyzer-local graph walk is introduced.

Factory cleanup discovery and worker publication guards move to cohesive
`cleanup_targets.go` and `worker_publication.go`; retained-owner provenance and
its structured result stay in `retained_owners.go`. Unused method-selection
and completion-defer facades are removed after their callers select the
budgeted implementations. Known API rationale and pinned links move with their
decision points. No summary schema or published declaration guarantee changes.

This is bounded source/SSA review because graph tools remain unavailable.
Underlying alias, access-path and call-result/referrer mechanics still contain
queries without a supplied allowance. `.44.11.5` records that transitive review;
`.44.11.4` retains caller-bound and relay-dependency routes. This census change
does not certify a wall-clock or whole-engine bound. The wider architecture
goal and 15-site production FP queue remain open. No production correction is
credited; no full precision-regression or local race run is performed.

Static stargz control uses pinned clean
`624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
`-enable=goroutineownership -json`, CGO disabled, readonly modules and GOWORK off.
Parent artifact `.build/goal-relay-current` is retained from the prior relay
change (SHA-256
`9247339c04ff8f920d7a673dcdd28c84d26a856080e6c468b3ace6384e2ffc65`).
Corrected `.build/goal-retained-budget-current` has SHA-256
`f0177e838480d8bb9df73e9f68ad316f50919860bbf170382c6ec114016413b1`.
It includes the census/proof changes before final unused-facade removals;
the hash identifies the exact executable, not a clean-tree source revision.
`.build/goal-retained-budget-stargz-{parent,current}.{json,err}` both exit 3,
retain byte-identical 827-byte JSON, empty stderr and the reviewed TP at
`store/manager.go:193:2`. Candidate tests, generators and applications are not
executed. The commentary/reference/public-prose check passes in
`.build/goal-retained-budget-architecture.log`.

Final `make verify` passes in `.build/goal-retained-budget-final-verify.log`,
including ordinary tests, generated helper inventories, formatting, vet, lint,
dead-code and local dogfood. The first gate passed ordinary tests but found the
two unused facades; the final gate verifies their removal. Child `.44.11.3`
can close for the direct census/availability work, while its parent and the
broader consolidation objective remain active for the recorded remaining scope.

## Caller-bound and relay-dependency allowances

Beads `gohawk-dho.44.11.4` follows retained-owner census work in `b888f20`.
Caller channel/context/receiver and local-cancellation queries now use the
supplied observed allowance for worker resolution, instruction/select and
binding census, receiver-field store checks, storage and opaque callback scans.
Local cancellation reuses the authoritative obligation flow with bounded
states and instruction labels. A cancellation defer registered before launch
still requires dominance; conditional and asynchronous cancellation cannot
establish coverage. The now-unused standalone worker-resolution and publication
facades are removed. Caller lifetime evidence moves into `caller_bounds.go`,
leaving relay and lifecycle-method discovery cohesive in `lifecycle.go`.

The lifecycle decision checks availability immediately after each query before
naming a bound. Caller queries share one allowance and preserve their prior
order/reasons; exhaustion remains `worker-receive-budget-exhausted`. Relay
dependency resolution, group discovery and local cancellation share its own
allowance, with `relay-dependency-budget-exhausted` and attributed phase
evidence. Neither a partial field-write census nor a callback cutoff supplies
a named ownership witness. Both unavailable outcomes are unknown, never joins.

Shared possible spawn mapping and load-alias leaf traversal gain budgeted
entry points; default callers delegate the same candidate-selection engine.
Shared ordered reachability similarly preserves its exact existing direction.
The relay filter still considers a participant that can reach the launch;
it is not replaced with forward-only reachability. This change makes no claim
that later participants now qualify. The direct caller receive census retains
its existing policy; helper chains continue through the shared value-specific
worker receive engine, whose select-state enumeration now spends the allowance.

Actual SSA tests in `caller_budget_test.go` exercise fresh/cutoff channel,
receiver, cancellation and relay decisions with attributed trace phases.
Cancellation tests pair exact and preceding-defer coverage with conditional
and asynchronous negatives. Shared `call_binding_budget_test.go` checks
unavailable/fresh possible capture mapping and ordered reachability in both
directions. Existing distinct-binding, recursive, opaque and memo-cutoff
receive controls pass. Focused receipts are
`.build/goal-caller-budget-focused2.log`. Initial focused lint/commentary checks
found the unused resolver facade and missing defer rationale; corrected checks
pass in `.build/goal-caller-budget-{lint,commentary}-corrected.log`.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is statically scanned with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-retained-budget-current`
has SHA-256
`f0177e838480d8bb9df73e9f68ad316f50919860bbf170382c6ec114016413b1`;
corrected `.build/goal-caller-budget-current` has SHA-256
`f2e58273086c499015b33354a2f1438dc8c6bf616da2b1ab13d29b42b6b095de`.
Both are immutable pre-commit artifacts; corrected includes the query changes
before the final unused resolver removal and rationale comment. Fresh
`.build/goal-caller-budget-stargz-{parent,current}.{json,err}` both exit 3,
retain byte-identical 827-byte JSON, empty stderr and the reviewed TP at
`store/manager.go:193:2`. No candidate tests, generators or applications run.

Graph tools remain unavailable; this is bounded source, SSA and trace evidence.
`.44.11.5` now explicitly includes transitive identity, call-result/referrer,
metadata allocation and initial flow guard/index setup costs across retained
and caller-bound routes. No whole-engine wall-clock bound or broad completion
is certified. The overall goal and 15-site production FP queue remain open;
no production FP correction, full precision-regression or local race run is
credited by this change.

The first canonical `make verify` passes in
`.build/goal-caller-budget-verify.log`. Final source review then limits relay
reachability probes to sends and other worker launches, the only instructions
that can supply its existing witness. This avoids repeated order/CFG queries
for unrelated instructions while retaining the census and reachability policy.
Focused checks pass again in `.build/goal-caller-budget-focused3.log`.
Final immutable `.build/goal-caller-budget-final` has SHA-256
`02ff7a5a52bc5348b4906cc6e37c866fac7eb790b9859c7a8f2844637611bf65`.
Its fresh `.build/goal-caller-budget-stargz-final.{json,err}` retains the same
827-byte parent diagnostic JSON, exit 3 and empty stderr. This final artifact
includes the resolver/publication facade removals, rationale and reduced probe
census; earlier corrected-artifact receipts remain intermediate evidence.

Final `make verify` passes in `.build/goal-caller-budget-final-verify.log`,
including ordinary tests, generated helper inventory, formatting, vet, lint,
dead-code and local dogfood. The final receipt documentation check also passes
in `.build/goal-caller-budget-receipt-docs.log`. Child `.44.11.4` can close for
the supplied-allowance and immediate availability contract. Parent `.44.11`
remains active for `.5` transitive costs and `.6` shared census consolidation;
the broader goal is not complete.

## Shared budgeted instruction census

Beads `gohawk-dho.44.11.6` follows caller-bound work in `95ebf73`.
`ssaflow.InstructionsWithin` now owns block-order instruction iteration and
spending before each yield. Early consumer exit stops immediately; partial
iteration retains availability on the supplied budget. It selects no proof
policy, feasible paths, call bindings or memo keys. Default `InstructionsOf`
collects typed instructions through the same engine with no caller allowance.

Nine direct census routes reuse it: two worker publication guards, caller
channel/field-write/cancellation scans, retained-owner opaque work, relay
participants, the worker receive engine and shared callback evidence. Select
states, arguments, capture selection and identity remain caller concerns.
Receive search explicitly returns unknown after iterator cutoff, preserving
its incomplete-memo boundary. Caller receive stops before helper search when
the census is unavailable. Shared unreadable callbacks preserve possible
opaque consumption and stop immediately on a body cutoff. No new semantic
guarantee, fact schema or reporting path is introduced.

`instruction_census_test.go` verifies actual SSA order, zero and partial
allowances, exact complete spending, early stop and default typed collection.
Existing caller/receiver/cancellation, retained-owner path-local flow,
factory/selected-context, callback and receive-memo cutoff/fresh controls pass
in `.build/goal-census-focused.log`. Commentary passes in
`.build/goal-census-commentary.log`. Final canonical `make verify` passes in
`.build/goal-census-verify.log`, including ordinary tests, generated inventory,
formatting, vet, lint, dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
uses static `-enable=goroutineownership -json` scans with CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-caller-budget-final` has
SHA-256 `02ff7a5a52bc5348b4906cc6e37c866fac7eb790b9859c7a8f2844637611bf65`;
corrected `.build/goal-census-current` has SHA-256
`c345ce4f0df143928c8a1fb9280e224ab52d8a0f35c1e5c5f95253558e9cf6ca`.
These immutable pre-commit artifacts identify the exact executables scanned.
`.build/goal-census-stargz-{parent,current}.{json,err}` both exit 3, retain
byte-identical 827-byte JSON, empty stderr and the reviewed TP at
`store/manager.go:193:2`. No production FP correction is credited.

Graph tools remain unavailable; this is bounded source/SSA/trace review, not
an exhaustive duplication audit. Parent `.44.11` remains active for `.5`
transitive identity, metadata and flow-setup costs. Other body/selected-block
census sites are outside the nine migrated routes. The broader goal and
15-site production FP queue remain open. No full precision-regression,
local race run or candidate tests, generators or applications are performed.


## Shared reaching-value visit budget (dho.44.11.5.1)

At parent `0c4b5b6`, wrapper traversal and phi fan-out in `ReachingWalk`
were outside leaf-only allowances. The existing fold now accepts
`Within(budget)` and charges value visits, including wrappers and revisits.
Must-branches retain independent visited sets and share the allowance;
possible folds stop at the first witness. A nested leaf that exhausts the
allowance cannot return positive fold evidence. Nil budgets retain default
traversal policy and caller-selected opaque forms stay opaque.

Possible spawned load mapping and retained-owner/pipe discovery use the shared
fold allowance; duplicate leaf charges are removed. Their existing availability
checks retain authoritative unknown cutoffs. Actual SSA tests cover wrappers,
phi branches, independent must-branches sharing one pool, early witnesses,
manual marks and nested-leaf cutoff. Both affected packages pass in
`.build/goal-fold-budget-focused.log`. Canonical `make verify` passes in
`.build/goal-fold-budget-verify.log`, including ordinary tests, formatting,
vet, lint, generated inventory, dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-census-current` has
SHA-256 `c345ce4f0df143928c8a1fb9280e224ab52d8a0f35c1e5c5f95253558e9cf6ca`;
current `.build/goal-fold-budget-current` has SHA-256
`38b02d28c1703ee204c070fc37df7bcd32891a92fb4ec7d3b98ebc6875a5b60f`.
Fresh `.build/goal-fold-budget-stargz-{census,fold-budget}.{json,err}` scans
both exit 3 with identical 827-byte JSON, empty stderr and the reviewed TP
at `store/manager.go:193:2`. These hashes identify immutable pre-commit binaries.

This bounds value visits, not map-clone costs, nested heap identity, metadata
allocation or initial flow setup. Parent `gohawk-dho.44.11.5` remains active
for that transitive scope. Graph tools remain unavailable; evidence uses scoped
source and actual SSA rather than an exhaustive graph audit. No production FP
correction is credited; the 15-site queue and broader goal remain open. No full
precision-regression, local race or candidate tests/generators/apps are run.


## Shared call metadata allowance (dho.44.11.5.2)

At parent `b652eb8`, bounded caller/retained-owner queries charged enumeration
only after `CallBindings` and `ClosureBindingPairs` had materialized slices;
result selection scanned call referrers without charging. Shared lazy
`CallBindingsWithin` and `ClosureBindingPairsWithin` now charge before yielding
and avoid binding slices. Existing default collectors use the same drivers.
Argument/capture order and the two-pass capture-first spawned selection remain.
The shared lifecycle unreadable-callback query uses the same capture driver.

`CallResultWithin` retains exact tuple-slot selection and the single-result
call representation, charging referrers before inspection and stopping at the
first matching result. Cleanup-target, local cancellation and pipe-peer
lookups use it. Cleanup and cancellation stop before downstream work on
selection cutoff; existing authoritative availability checks preserve unknown.
The partial census/result answer cannot become a negative absence proof.

`call_metadata_budget_test.go` constructs actual SSA to test argument/capture
order, shared-pool cutoff, early stop, default parity, tuple lookup cutoff,
completed absent-slot lookup and single-result spending. Focused tests pass in
`.build/goal-metadata-focused.log` for ssaflow and goroutineownership; the
selected lifecyclefacts regex matches no tests, so its domain coverage comes
from canonical `make verify` in `.build/goal-metadata-verify.log`. That gate
passes ordinary tests, formatting, vet, lint, generated inventory, dead-code
and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-fold-budget-current` has
SHA-256 `38b02d28c1703ee204c070fc37df7bcd32891a92fb4ec7d3b98ebc6875a5b60f`;
current `.build/goal-metadata-current` has SHA-256
`74d19d6dfce3bc04e4c5156c8e2f2208a54bc6878006d0de86c402d9ddcfc4ca`.
Fresh `.build/goal-metadata-stargz-{fold-budget,metadata}.{json,err}` scans
both exit 3 with identical 827-byte JSON, empty stderr and the reviewed TP
at `store/manager.go:193:2`. Hashes identify immutable pre-commit binaries.

Parent `gohawk-dho.44.11.5` remains active for heap identity, access paths,
dominance and initial flow setup. In particular, heap graph result publication
in `internal/heapmodel/store_heap_apply.go` retains its own unbounded result
lookup; this change is not a whole-engine bound. Graph tools remain unavailable;
evidence uses scoped source and actual SSA. No production FP correction is
credited; the 15-site queue and broader goal remain open. No full
precision-regression, local race or candidate tests/generators/apps are run.


## Shared structural identity allowance (dho.44.11.5.3)

At parent `2d4438d`, caller-capture projection and retained-owner field mapping
entered structural identity/access-path recursion without their allowance.
`StructurallyIdenticalWithin`, `AccessPathStepsWithin`,
`ValueIsAccessPathFromWithin` and `ProveIdentityWithin` now share the caller's
budget across structural comparisons, reaching folds, projection recursion and
path-step comparison. Default facades delegate with nil allowance and retain
existing must-identity, wrapper, field/index and load policy. Path-step equality
is centralized instead of separately maintained by `SameAccessPath` and
`ProveIdentity`; the latter's inaccurate memoization comment is removed.

Caller-owned capture and retained-owner projection consumers use the bounded
queries. Interrupted `ProveIdentityWithin` returns structured unknown with
`EvidenceBudgetExhausted`, never different-path evidence or a credited owner.
Actual SSA controls in `identity_budget_test.go` cover agreeing converted phi
alternatives, separate loads, static paths and differing/dynamic indexes,
shared-pool cutoff and interruption during the last path-comparison step.
Focused tests pass in `.build/goal-identity-focused.log`; final canonical
`make verify` passes in `.build/goal-identity-final-verify.log`, including
ordinary tests, generated inventory, formatting, vet, lint, dead-code and local
dogfood. Initial lint findings in the new test were fixed by separating access
path and corresponding-identity scenarios; production code was unchanged.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-metadata-current` has
SHA-256 `74d19d6dfce3bc04e4c5156c8e2f2208a54bc6878006d0de86c402d9ddcfc4ca`;
current `.build/goal-identity-current` has SHA-256
`5769fca01d32e685caec4f65992bd805cb987a3fe6c2c06b60cc3525382d7190`.
Fresh `.build/goal-identity-stargz-{metadata,identity}.{json,err}` scans
both exit 3 with identical 827-byte JSON, empty stderr and the reviewed TP
at `store/manager.go:193:2`. Hashes identify immutable pre-commit binaries.

Parent `gohawk-dho.44.11.5` remains active for heap graph identity, storage's
structural calls, other consumers, dominance and initial flow setup. Map-clone
and graph costs are not bounded by these value-visit allowances. Possible alias
queries are not replaced by exact identity. Graph tools remain unavailable;
evidence uses scoped source and actual SSA. No production FP correction is
credited; the 15-site queue and broader goal remain open. No full
precision-regression, local race or candidate tests/generators/apps are run.


## Storage identity cutoff boundary (dho.44.11.5.4)

At parent `02ffac2`, `Storage.Same` used unbounded structural identity before
and after load resolution, and `Content` could replace an exhausted
reaching-write proof with positive graph evidence. Storage now shares its
allowance with the bounded structural engine. Raw and resolved identity phases
use one structured structural/graph decision; writes-only graph exclusion is
preserved. A first unavailable load stops before resolving the second one.
Exhaustion remains observed unknown and cannot fall through to graph evidence.
The final raw graph retry is retained because graph cache publication can
change after registered summaries invalidate an entry; no fixed-generation
assumption is introduced.

Actual SSA controls in `store_identity_budget_test.go` cover distinct equivalent
field selections, zero-budget direct identity, writes-only and ordinary policy,
observed cutoff, shared candidate-pool availability and contents with a warmed
graph. Existing snapshot/aggregate/deferred controls pass in
`.build/goal-storage-identity-focused.log`; final cutoff controls pass in
`.build/goal-storage-identity-focused-final.log`. A one-file Go overlay restores
parent `store_model.go` while retaining the new tests: both cutoff tests fail
with positive evidence in `.build/goal-storage-identity-parent-counterfactual.log`
(exit 1), showing they distinguish the defect. Canonical `make verify` passes in
`.build/goal-storage-identity-verify.log`, including ordinary tests, formatting,
vet, lint, generated inventory, dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-identity-current` has
SHA-256 `5769fca01d32e685caec4f65992bd805cb987a3fe6c2c06b60cc3525382d7190`;
current `.build/goal-storage-identity-current` has SHA-256
`d91268f500c0c1eb92ebbb6762f8bb7d34a9c24656544f3cdff23357510c35e8`.
Fresh `.build/goal-storage-identity-stargz-{identity,storage-identity}.{json,err}`
scans both exit 3 with identical 827-byte JSON, empty stderr and the reviewed TP
at `store/manager.go:193:2`. Hashes identify immutable pre-commit binaries.

Parent `gohawk-dho.44.11.5` remains active for graph construction/waiting and
internal query costs, other structural consumers, dominance and flow setup.
`Storage.collect` still uses unbounded `InstructionMayFollow`; reaching-write
setup uses unbounded dominance and instruction indexing. These are concrete
remaining routes, not covered by this identity cutoff correction. Graph MCP
tools remain unavailable; evidence uses scoped source and actual SSA. No
production FP correction is credited; the 15-site queue and broader goal remain
open. No full precision-regression, local race or candidate tests/generators/apps
are run.


## Storage ordering and initial flow position (dho.44.11.5.5)

At parent `7b7d409`, storage used unbounded reachability, sole-initializer
dominance and instruction indexing beneath its allowance. The existing index
scan is exposed as `InstructionIndexWithin`; budgeted dominance shares the
same-block order engine with possible-follow queries. Defaults delegate with
nil allowance, retaining valid SSA order. Unindexed instructions supply no
ordering evidence. Cross-block dominance charges its constant-time tree check.

Storage address-use collection asks budgeted possible-follow and returns
unknown before admitting or discarding a referrer if ordering cuts off.
Reaching-write setup charges sole-initializer dominance and observation
indexing. Caller cancellation's preceding-defer check uses budgeted dominance.
The shared obligation walk now charges its initial index lookup; cutoff is
uncertain with no witness or classifier invocation, never honored or violated.
Initial dominating-guard extraction remains independent review scope.

Actual SSA controls in `flow_setup_budget_test.go` and
`store_flow_budget_test.go` cover block order, same/cross-block dominance,
candidate-pool cutoff, incomplete setup, fresh honored/violated witnesses,
sole initializer and address-use order cutoff. Existing storage snapshot,
aggregate, deferred and caller-bound controls pass in
`.build/goal-storage-flow-focused.log`; final new tests pass in
`.build/goal-storage-flow-new-tests-final.log`. A three-file Go overlay restores
parent storage model/reaching and obligation-flow consumers while retaining
current helpers and tests. Both cutoff tests fail in
`.build/goal-storage-flow-parent-counterfactual-final.log` (exit 1), showing
the new assertions detect the bypass. Final canonical `make verify` passes in
`.build/goal-storage-flow-final-verify.log`, including ordinary tests,
formatting, vet, lint, generated inventory, dead-code and local dogfood.
Initial lint failures were fixed with distinct fixture arguments and a
mechanical sole-write selector that flattens the initializer decision.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-storage-identity-current`
has SHA-256 `d91268f500c0c1eb92ebbb6762f8bb7d34a9c24656544f3cdff23357510c35e8`;
current `.build/goal-storage-flow-current` has SHA-256
`4c34f269067a339bc157dd5566ee27ac9bed13f8da18b0e8a591475ba81ca58c`.
Fresh `.build/goal-storage-flow-stargz-{storage-identity,storage-flow}.{json,err}`
scans both exit 3 with identical 827-byte JSON, empty stderr and the reviewed TP
at `store/manager.go:193:2`. Hashes identify immutable pre-commit binaries.

Parent `gohawk-dho.44.11.5` remains active for graph construction/waiting and
internal query costs, other consumers, dominating guards and remaining flow
setup/feasibility costs. This is not a whole-query wall-clock bound. Graph MCP
tools remain unavailable; evidence uses scoped source and actual SSA. No
production FP correction is credited; the 15-site queue and broader goal remain
open. No full precision-regression, local race or candidate tests/generators/apps
are run.


## Initial guard setup allowance (dho.44.11.5.6)

At parent `74328fc`, the shared obligation walk charged its start position but
seeded guards through unbounded dominator/condition/address/cycle/store queries.
`GuardsDominatingWithin` now shares the flow allowance through that initial
setup. Public default decoding and guard collection delegate to the same
engines with nil allowance. The bounded cycle query reuses the shared CFG walk;
a cycle-search cutoff cannot establish computed-condition stability.
Invalidating-store census is lazy and uses the existing budgeted order query.
Interrupted setup supplies no guard seed and the obligation walk returns
uncertain with no witness or classifier invocation, rather than judging returns
from missing correlation or pruning paths from partial guard evidence.

Initial scan/invalidation evidence is isolated in `flow_guard_setup.go`;
`flow_guards.go` retains identity and state transitions. Existing guard-limit,
loaded/stable distinction, negation parity and mutation policy are preserved.
Actual SSA controls in `guard_setup_budget_test.go` cover stable, loaded and
mutated guards, shared-pool/setup cutoffs, acyclic/loop computed conditions,
nested field identities, negation decoding and fresh uncovered-return witnesses.
Existing condition/guard-rerun/negation controls and final new tests pass in
`.build/goal-guard-setup-focused-final.log`. Early lint passes in
`.build/goal-guard-setup-lint.log`; canonical `make verify` passes in
`.build/goal-guard-setup-verify.log`, including ordinary tests, formatting,
vet, lint, generated inventory, dead-code and local dogfood.

A one-file Go overlay restores the parent's obligation-flow consumer while
retaining current helpers/tests. `GuardSetupCutoffStopsObligation` fails in
`.build/goal-guard-setup-parent-counterfactual.log` (exit 1), proving the control
detects the bypass of initial guard availability.
Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-storage-flow-current`
has SHA-256 `4c34f269067a339bc157dd5566ee27ac9bed13f8da18b0e8a591475ba81ca58c`;
current `.build/goal-guard-setup-current` has SHA-256
`920cf946f283fc323948009240f94ea8df4c93a0f62c00f6ac279a9502df2537`.
Fresh `.build/goal-guard-setup-stargz-{storage-flow,guard-setup}.{json,err}`
scans both exit 3 with identical 827-byte JSON, empty stderr and the reviewed TP
at `store/manager.go:193:2`. Hashes identify immutable pre-commit binaries.

Parent `gohawk-dho.44.11.5` remains active for graph construction/waiting and
internal query costs, other consumers and downstream flow costs. Path-state
`After`/`Forget`/`Key` work, successor feasibility and edge guard decoding still
have independent costs; initial setup charging does not cover them. Leaf string
and type rendering and CFG seed allocation are not a claimed wall-clock bound.
Graph MCP tools remain unavailable; evidence uses scoped source and actual SSA.
No production FP correction is credited; the 15-site queue and broader goal
remain open. No full precision-regression, local race or candidate
tests/generators/apps are run.


## Flow state and edge guard allowance (dho.44.11.5.7)

At parent `d3ae2d4`, obligation state keys were constructed before the state
allowance check, instruction and guard-state work remained independent, and
exhausted return/termination callbacks could still contribute proof. The shared
work-list engine now exposes `WalkStatesWithin`: every queued visit, including
revisits, spends before constructing its key. Interrupted key and step answers
are rejected before admission. The obligation engine shares its allowance
through instruction visits, guard key/filter work and edge condition decoding.
Store and rerun-result invalidation use one identity filter. Default transitions
still use the same engines with nil allowance; stable contradictions prune and
loaded contradictions remain opaque. Caller cancellation removes its duplicate
instruction-visit charge while retaining nested storage work charges.

One obligation walk owns the outcome and return witness. Exhaustion in a
classifier, return, edge, successor or termination callback produces uncertain
coverage with no witness before its answer can settle, violate or prune a path.
Actual SSA controls in `flow_state_budget_test.go` cover loaded/stable guards,
store/key/edge cutoffs and callbacks; work-list controls cover revisits and
interrupted key/step admission. The affected ssaflow/goroutineownership package
suites pass in `.build/goal-flow-state-focused-final.log`; final early lint
passes in `.build/goal-flow-state-lint-final.log`. A one-file Go overlay restores
the parent obligation consumer with current helpers/tests. The return and
termination cutoff controls fail in
`.build/goal-flow-state-parent-counterfactual.log` (exit 1), detecting stale
proof availability. No production FP correction is credited.

The first canonical gate passed ordinary tests and dogfood but found three
facades used only by tests. `GuardCondition` and `GuardAddressIdentity` now live
as test probes in `flow_guards_export_test.go`; the lifecycle mutation control
uses `PathGuards.After` and the unused production `Forget` facade is removed.
Focused ssaflow/lifecycle controls and deadcode pass in
`.build/goal-flow-state-cleanup-{tests,deadcode}.log`. Final canonical
`make verify` passes in `.build/goal-flow-state-final-verify.log`, including
ordinary tests (129 seconds), formatting, vet, lint, generated inventory,
dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-guard-setup-current`
has SHA-256 `920cf946f283fc323948009240f94ea8df4c93a0f62c00f6ac279a9502df2537`;
current `.build/goal-flow-state-current` has SHA-256
`aa3ea696c593af44b7dee90aa7b3ad3c4baf480d6e24802fe39f3cb3a2fe9cf2`.
Fresh `.build/goal-flow-state-stargz-{guard-setup,flow-state}.{json,err}` scans
both exit 3 with identical 827-byte diagnostic JSON, empty stderr and the
reviewed TP at `store/manager.go:193:2`. Hashes identify immutable pre-commit
binaries, not clean-tree VCS stamps.

Parent `gohawk-dho.44.11.5` remains active for graph costs, other consumers,
successor feasibility internals, deferred termination census and callback
internal work. `gohawk-dho.44.11.5.8` records the concrete RunDefers census and
dominance gap. Leaf rendering and allocation costs also remain independent;
this is not a whole-query wall-clock bound. Graph MCP tools remain unavailable;
evidence uses scoped source and actual SSA. The 15-site production FP queue and
broader goal remain open. No full precision-regression, local race or candidate
tests/generators/apps are run.


## Deferred termination allowance (dho.44.11.5.8)

At parent `945acbe`, the obligation engine rejected exhausted callback answers
but deferred termination still collected every registration and checked default
dominance beneath the flow allowance. `InstructionTerminatesWithin` now owns
that same termination policy with caller-supplied allowance. Its default facade
delegates with nil allowance. Direct call dispatch spends before consulting a
callback and rejects its interrupted positive answer. `RunDefers` uses the lazy
shared instruction census and budgeted exact dominance. Conditional registration
is still insufficient; a terminating defer must dominate its execution point.
The obligation driver supplies its existing allowance and becomes uncertain
when termination work is unavailable before it can prune the path.

Actual SSA controls in `flow_termination_budget_test.go` cover unconditional,
conditional and unrelated defers, every allowance shorter than a complete query,
candidate-pool cutoff, call callback cutoff and fresh callback evidence.
A nested-census flow control retains honored coverage for a fresh unconditional
deferred exit and returns uncertain at cutoff. Focused ssaflow/lifecycle controls
pass in `.build/goal-deferred-termination-focused.log`. A one-file overlay
restores the parent obligation consumer while retaining current helpers/tests;
the nested-census cutoff control fails in
`.build/goal-deferred-termination-parent-counterfactual.log` (exit 1), detecting
the uncharged consumer. Early lint passes in
`.build/goal-deferred-termination-lint-final.log` after correcting an integer-range
style issue. Canonical `make verify` passes in
`.build/goal-deferred-termination-verify.log`, including ordinary tests
(69 seconds), formatting, vet, lint, generated inventory, dead-code and dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-flow-state-current`
has SHA-256 `aa3ea696c593af44b7dee90aa7b3ad3c4baf480d6e24802fe39f3cb3a2fe9cf2`;
current `.build/goal-deferred-termination-current` has SHA-256
`14dea83235d978e6d92e932a810eee76a813782b18c28a3cdeef344c256d1bd0`.
Fresh `.build/goal-deferred-termination-stargz-{flow-state,deferred}.{json,err}`
scans both exit 3 with identical 827-byte diagnostic JSON, empty stderr and the
reviewed worker TP at `store/manager.go:193:2`. Hashes identify immutable
pre-commit binaries, not clean-tree VCS stamps.

Parent `gohawk-dho.44.11.5` remains active for successor feasibility internals,
library-contract inference, nested callback work, heap graph costs and other
consumers. `gohawk-dho.44.11.5.9` records default literal/phi/helper-return
feasibility charging, preserving the existing helper's independent 128-instruction
cap. Bound-value and assumed-non-nil feasibility remain distinct review scope.
Leaf rendering and allocation also retain independent costs; this is not a
whole-query wall-clock bound. Graph MCP tools remain unavailable; evidence uses
scoped source and actual SSA. No production FP removal is credited; the 15-site
queue and broader goal remain open. No full precision-regression, local race or
candidate tests/generators/apps are run.


## Literal successor feasibility allowance (dho.44.11.5.9)

At parent `e709dc6`, edge dispatch shared the flow allowance but default literal
feasibility inspected predecessor phis and helper bodies independently of it.
`FeasibleSuccessorsWithin`, `BranchValueWithin` and `BranchBoolWithin` now share
that allowance through value/incoming-edge visits and the existing literal
helper return census. Comparison operands use the same allowance and an
interrupted first operand does not initiate the second query. An incomplete
helper agreement never supplies a literal. The primitive keeps all successors
at caller cutoff; the obligation driver sees exhaustion before judging paths.
Custom successor callbacks are checked before further narrowing when they
exhaust that shared allowance.

The existing 128-instruction helper cap retains its independent policy:
over-cap helpers supply no literal evidence without exhausting an available
caller allowance. This does not expand inference through forwarding calls,
deferred mutation, mixed returns or opaque bodies. Default value and feasibility
facades delegate to the same engines; the Boolean facade had only test callers
and is removed in favor of `BranchBoolWithin` with nil allowance in those tests.
Literal evidence is extracted into `flow_branch_literals.go` (190 lines),
leaving `flow_paths.go` (389 lines) with CFG/order and assumed-path evidence.
Existing pinned precision rationale comments move with their owning code.

Actual SSA controls in `flow_literal_budget_test.go` cover predecessor selection,
agreeing/mixed helper returns, deferred mutation, result extraction, integer
comparisons, interrupted visits and the exact helper-cap boundary. A flow
control is honored with fresh literal evidence and uncertain when its helper
census exceeds the caller's remaining allowance. Focused ssaflow/lifecycle
controls pass in `.build/goal-literal-flow-focused-final.log`; early/final lint
and deadcode pass in `.build/goal-literal-flow-{lint-final,deadcode}.log`.
A one-file overlay restores the parent successor policy while retaining current
helpers/tests. The nested helper-feasibility cutoff control fails in
`.build/goal-literal-flow-parent-counterfactual.log` (exit 1), detecting the
unbudgeted consumer. Canonical `make verify` passes in
`.build/goal-literal-flow-verify.log`, including ordinary tests (67 seconds),
formatting, vet, lint, generated inventory, dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-deferred-termination-current`
has SHA-256 `14dea83235d978e6d92e932a810eee76a813782b18c28a3cdeef344c256d1bd0`;
current `.build/goal-literal-flow-current` has SHA-256
`f1f3df28738374173bf2b61f15a7b16664a6edee4beac88b40da109d9e0d1820`.
Fresh `.build/goal-literal-flow-stargz-{deferred,literal}.{json,err}` scans both
exit 3 with identical 827-byte diagnostic JSON, empty stderr and the reviewed
worker TP at `store/manager.go:193:2`. Hashes identify immutable pre-commit
binaries, not clean-tree VCS stamps.

Parent `gohawk-dho.44.11.5` remains active for bound-value and assumed-successor
feasibility (`gohawk-dho.44.11.5.10`), library-contract/custom-hook internals,
heap graph costs and other consumers. Leaf rendering, type-system internals
and allocation also retain independent costs; this is not a whole-query
wall-clock bound. Graph MCP tools remain unavailable; evidence uses scoped
source and actual SSA. No production FP removal is credited; the 15-site queue
and broader goal remain open. No full precision-regression, local race or
candidate tests/generators/apps are run.


## Bound-value and assumed-successor allowance (dho.44.11.5.10)

At parent `92e9be7`, literal successor evidence shared the flow allowance but
bound-condition and assumed-nonnil/type queries did not. Successor policy now
passes the same allowance through `FixedValues.HoldsWithin`,
`DecidedSuccessorWithin`, `NarrowWithin` and the assumed-successor engine.
Negation, `DefinitelyNilWithin` and exact structural identity reuse the existing
shared traversal engines. Nil folds preserve the selected transparent forms
and keep interface boxing opaque. Caller cutoff supplies no decided condition;
primitive narrowing leaves its input edges unpruned and the policy rejects
unavailable evidence before judging paths. The unused `DecidedSuccessor` facade
is removed; its test callers use the budgeted engine with nil allowance.

Bound-value filtering and the two assumed-edge forms now share one successor
membership filter, replacing three equivalent loops. Compatible concrete
assertions require exact receiver identity; incompatible assertions and foreign
fields retain both edges. Type-check dispatch is charged, with type-system
internals still separate costs. Assumption mechanics and their pinned rationale
comments move to `flow_assumptions.go`, separate from CFG/order queries.

Actual SSA controls in `flow_assumptions_budget_test.go` cover bound booleans,
returned negation, nil comparisons/conversions, boxing, mixed phis, exact/foreign
fields and compatible/incompatible assertions. Every incomplete visit allowance
supplies no positive proof. Whole-flow controls retain fresh exact coverage
and return uncertain with no witness at every allowance shorter than the
complete query. Focused ssaflow/lifecycle tests pass in
`.build/goal-assumed-flow-focused-final.log`; final early lint and deadcode pass
in `.build/goal-assumed-flow-lint-final-clean.log` and
`.build/goal-assumed-flow-deadcode.log`. Initial test corrections include the
Outcome type spelling, a formatter-joined long assertion, and SSA normalization
of `if !flag` by swapping edges. A returned negation separately pins the actual
NOT traversal; `.build/goal-assumed-flow-bound-probe.log` records the branch
condition that corrected the mistaken expectation.

A consumer-bypass overlay selectively gives bound and assumed queries nil
allowance while retaining current helpers and tests. Both consumer controls
fail in `.build/goal-assumed-flow-bypass-counterfactual.log` (exit 1), detecting
the two uncharged routes. This is a selective bypass, not a complete parent
source restoration. Canonical `make verify` passes in
`.build/goal-assumed-flow-verify.log`, including ordinary tests (66 seconds),
formatting, vet, lint, generated inventory, dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-literal-flow-current`
has SHA-256 `f1f3df28738374173bf2b61f15a7b16664a6edee4beac88b40da109d9e0d1820`;
current `.build/goal-assumed-flow-current` has SHA-256
`b7c7866f9e9e6dba0a91aaa15dd27a861293bd2992b5e106b7b11411db60d1d5`.
Fresh `.build/goal-assumed-flow-stargz-{literal,assumed}.{json,err}` scans both
exit 3 with identical 827-byte diagnostic JSON, empty stderr and the reviewed
worker TP at `store/manager.go:193:2`. Hashes identify immutable pre-commit
binaries, not clean-tree VCS stamps.

Parent `gohawk-dho.44.11.5` remains active for library-contract/custom-hook
internals, heap graph costs and other consumers. `gohawk-dho.44.11.5.11` records
the separate queue/termination work in normal-return reachability and its use
in result-fact termination inference, where cutoff must never become a positive
no-return guarantee. Leaf rendering, type-system internals and allocation also
retain independent costs; this is not a whole-query wall-clock bound. Graph MCP
tools remain unavailable; evidence uses scoped source and actual SSA. No
production FP removal is credited; the 15-site queue and broader goal remain
open. No full precision-regression, local race or candidate tests/generators/apps
are run.


## Normal-return reachability allowance (dho.44.11.5.11)

At parent `c899906`, normal-return reachability owned a separate block queue
and visited set and asked unbounded termination queries beneath result-fact
inference. The return search now delegates queued/revisit handling to
`WalkStatesWithin`, sharing instruction and termination visits with the caller.
`NormalReturnProof` records state, reason and a positive return witness;
`ProveNormalReturnWithin` distinguishes a found return, completed search finding
none, and unavailable evidence. Interrupted callbacks, pool cuts or deferred
census work cannot prove either reachability or absence. Default Boolean
facades delegate with nil allowance and retain the same CFG policy.
Reachability lives in `flow_return_reachability.go`, apart from order/obligation
compatibility queries in `flow_paths.go`.

Result-fact inference supplies its allowance and requires a completed negative
proof before setting neverReturns. Its duplicate callback-dispatch charge is
removed; engine lookup keeps its own shared charges. Recover blocks retain the
existing no-termination-claim policy, even for a recognized terminating defer.
The existing FunctionSummaries/CallGraphMemo Compose boundary discards answers
computed after exhaustion and cuts dependent cache entries. The existing domain
publication loop exports only Available summaries; neither cache nor writer
needs a second decision engine.

Actual SSA controls in `flow_return_budget_test.go` cover normal returns, loops,
direct/deferred exits, conditional registration, every incomplete allowance,
pool and callback cutoffs, and missing entries. Result controls in
`termination_budget_test.go` leave exactly enough allowance for lookup and the
initial census, verify unavailable termination summaries, then recover on a
fresh query through the same engine. A scoped publication probe runs the actual
domain writer on an internally generated static SSA fixture: an over-budget
Heavy body publishes no fact, a direct-exit control publishes termination, and
a larger subsequent query recovers Heavy's complete summary. Wire-format and
cross-package tests remain separate controls.

Focused controls and early lint pass in
`.build/goal-return-flow-new-tests-final.log` and
`.build/goal-return-flow-lint-final.log`. Initial test setup was corrected to
respect deferred functions' actual SSA recover entries and scope publication
assertions to the root package rather than dependencies. A one-file overlay
restores parent result inference while retaining current helpers/tests. Both
the direct cutoff and Heavy publication controls fail in
`.build/goal-return-flow-parent-counterfactual.log` (exit 1), detecting inference
that bypasses the shared reachability allowance. No incorrect production
termination fact or FP correction is inferred from that budget counterfactual.
Canonical `make verify` passes in `.build/goal-return-flow-verify.log`, including
ordinary tests (58 seconds), formatting, vet, lint, generated inventory,
dead-code and local dogfood.

Pinned clean stargz `624678b4e421947534cbf0618f9609853cccee0f`, `./store`,
is scanned statically with `-enable=goroutineownership -json`, CGO disabled,
readonly modules and GOWORK off. Parent `.build/goal-assumed-flow-current`
has SHA-256 `b7c7866f9e9e6dba0a91aaa15dd27a861293bd2992b5e106b7b11411db60d1d5`;
current `.build/goal-return-flow-current` has SHA-256
`778d89ecf968803aadc643c433e62d94ee8c6a8667fb68a51482290ce99fc1d1`.
Fresh `.build/goal-return-flow-stargz-{assumed,return}.{json,err}` scans both exit 3
with identical 827-byte diagnostic JSON, empty stderr and the reviewed worker
TP at `store/manager.go:193:2`. Hashes identify immutable pre-commit binaries,
not clean-tree VCS stamps.

Parent `gohawk-dho.44.11.5` remains active for library-contract/custom-hook
internals, heap graph costs and other consumers. `gohawk-dho.44.11.5.12` records
result-case inference calling lifecycle.ReturnsParameterUnchanged with separate
reachability, compatibility flow and storage allowances. Resource and lock
flows also retain separate termination consumers. Leaf rendering, type-system
internals and allocation remain independent costs; this is not a whole-query
wall-clock bound. Graph MCP tools remain unavailable; evidence uses scoped
source and actual SSA. No production FP removal is credited; the 15-site queue
and broader goal remain open. No full precision-regression, local race or
candidate tests/generators/apps are run.


## Returned-parameter allowance consolidation (2026-10-02)

Beads `gohawk-dho.44.11.5.12` replaces the default return-identity consumer
with `lifecycle.ProveReturnedParameterWithin`. Positive normal-return
reachability and exact same-type identities at every return use the shared
reachability, obligation and storage engines with one caller allowance.
Unknown identity never settles a return; exhaustion never proves absence of
a counterexample. The Boolean facade delegates to that authoritative proof.
No fact schema or application-specific declaration assumption is added.

Actual SSA controls cover direct and branching identity, mixed phi values,
interface boxing, aggregate wrappers, no-return loops and the existing
entry-reachable deferred/recovery shape. Every insufficient allowance and an
exhausted parent pool return unknown. Result inference retries successfully
after cutoff. The existing publication harness now also covers a large
identity body: the current writer declines the interrupted fact, retains a
small identity control and recovers on a fresh wider query. Restoring only
parent relations.go fails that publication control; this demonstrates work
allowance enforcement, not a production false-positive correction.

Focused lifecycle/result tests and early lint pass. Canonical make verify
passes generation, module verification, vet, formatting, lint, dead-code,
local dogfood and ordinary tests. Receipts are
`.build/goal-returned-identity-{focused-final,lint,verify,counterfactual}.log`.
The pinned stargz store comparison at
624678b4e421947534cbf0618f9609853cccee0f retains the reviewed worker report:
both static scans exit 3 with identical 827-byte JSON and empty stderr.
Parent `.build/goal-return-flow-current` SHA-256 is
778d89ecf968803aadc643c433e62d94ee8c6a8667fb68a51482290ce99fc1d1;
current `.build/goal-returned-identity-current` SHA-256 is
45d9beaf864df89248c0a8f9b807e82089329b489615fe3c5870a19a60cb0663.
These identify immutable precommit binaries, not clean-tree VCS stamps.

Child `.13` tracks conditional result relation consumers that still call
default state walks, assumption folds/successors and forwarded-call metadata.
Heap graph construction, type-system internals, custom/library contracts,
other consumers and allocation costs remain independent scope. Graph MCP
is unavailable; claims use scoped source and actual SSA. The broader goal
and 15-site production FP queue remain open. No full precision replay,
local race or candidate tests/generators/applications were run.


## Conditional result allowance consolidation (2026-10-02)

Beads `gohawk-dho.44.11.5.13` adopts shared bounded state, reaching-value,
condition and successor APIs in conditional result inference. The caller
summary allowance now covers queued block visits, assumption folds and
pruning. Exhausted assumptions stop before leaf fallback. Forwarded result
slots are decoded in constant work without wrappers/referrers, so one
charged dispatch precedes the existing callee summary request; no additional
metadata traversal facade is introduced. Existing exact nilness, witness,
paired-return and forwarding policy is retained.

Actual SSA predicate, branching side-effect and forwarded-pair controls
cover every insufficient allowance and fresh-query cache recovery. Explicit
zero allowance and exhausted-parent-pool tests cannot decide an assumed
literal. Restoring parent relations.go fails the zero-allowance control.
The publication harness now includes a large conditional predicate: an
interrupted summary cannot publish, while a fresh wider query recovers its
case. That harness is extracted into publication_budget_test.go with focused
control checks after crossing the lint complexity threshold. The larger
publication case also cuts off under the parent; no differential publication
correction is claimed for it.

Focused result tests, lint and canonical make verify pass, including ordinary
tests, local dogfood, generation, module verification, vet, formatting and
dead-code. Receipts use `.build/goal-conditional-results-*.log`; the final
counterfactual is `counterfactual-final.log`. At stargz pin
624678b4e421947534cbf0618f9609853cccee0f, both store-only static controls exit
3 with identical 827-byte diagnostic JSON and empty stderr, retaining the
reviewed abandoned-worker report. Immutable parent binary
`.build/goal-returned-identity-current` SHA-256:
45d9beaf864df89248c0a8f9b807e82089329b489615fe3c5870a19a60cb0663;
current `.build/goal-conditional-results-current` SHA-256:
6a318282e23fc31713c62c6eacee77ec01915fec3a0bba5cc611d481584ae009.
These are precommit artifacts, not clean-tree VCS stamps.

Child `.14` records storedResultQuery.resolve passing a reaching walk without
its budget for unconditional and nested stored-value results. It includes a
review of remaining result consumers. Other resource/lock walks, graph costs,
type-system and custom/library internals remain open. Graph MCP tools are
unavailable; scoped source and actual SSA supply the evidence. No whole-query
wall-clock bound or production FP removal is claimed; the 15-site queue and
broader goal remain active. No full precision replay, local race or candidate
tests/generators/applications were run.


## Result fold and census consolidation (2026-10-02)

Beads `gohawk-dho.44.11.5.14` attaches the result allowance at
storedResultQuery.resolve, including wrappers, phi visits and revisits before
leaf evidence. Nested storage resolution retains the shared fold's visited
history rather than opening another traversal. The explicit transparent forms,
typed-nil boxing and storage evidence policy are unchanged. compute and
resultRelation now consume InstructionsWithin instead of duplicating block /
instruction censuses. Recovery returns remain included; the summary requires
availability after enumeration, so an interrupted census cannot establish a
guarantee or absence of a counterexample.

Actual SSA literal, ChangeInterface, agreeing/disagreeing phi and stored-value
controls pass. Every insufficient allowance and a spent parent pool decline
positive value evidence; fresh summary queries recover after cutoff. Restoring
only parent storage.go fails the leaf-only allowance controls for literal and
interface conversion. The publication harness adds a large agreeing-phi body,
refuses its interrupted fact and recovers a nonnil guarantee on a fresh wider
query. This is allowance enforcement, not an audited production FP correction.

The bounded source review covers all six production files in resultfacts:
results.go and relations.go use the shared census/state/termination mechanisms;
storage.go owns allowance attachment and calls bounded writes-only storage;
facts.go admits only Available summaries; reasons.go defines boundary text;
describe.go renders published claims. Direct source-slot/callee metadata is
constant dispatch; leaf/callee inference already spends the summary allowance.
Signature scans, type-system internals, imported fact validation/serialization,
rendering, allocation and downstream graph construction have independent costs.
This review does not establish a whole-query wall-clock or whole-run bound.
Graph MCP tools are unavailable; evidence uses scoped source and actual SSA.

Focused result tests, lint and canonical make verify pass (including ordinary
tests, dogfood, generation, module verification, vet, formatting and dead-code).
Receipts use `.build/goal-result-fold-*.log`; the final focused/lint receipts
are census-tests.log and census-lint.log, and the final parent control is
counterfactual-final.log. The pinned stargz store comparison at
624678b4e421947534cbf0618f9609853cccee0f retains the reviewed worker report:
both static scans exit 3 with identical 827-byte JSON and empty stderr.
Immutable parent `.build/goal-conditional-results-current` SHA-256:
6a318282e23fc31713c62c6eacee77ec01915fec3a0bba5cc611d481584ae009;
current `.build/goal-result-fold-current` SHA-256:
483b7ac406a7f6022d6a2375922dab961ffa89e9dcbd55318b9485c95062f7ce.
These identify precommit artifacts, not clean-tree VCS stamps.

Child `.15` records resourcelifetime's existing observed candidate pool and
its separate default setup/work-list/key/guard/termination/successor routes.
Treat them as one resource-flow concern while preserving its optional/error
acquisition states. Other lock flows, graph costs, library/custom callbacks
and identity consumers remain open. The broader goal and production FP queue
are still active. No full precision replay, local race or candidate tests,
generators or applications were run.


## Resource flow pool consolidation (2026-10-02)

Beads `gohawk-dho.44.11.5.15` extracts the resource state machine into
flow_walk.go and makes proveResourceFlow authoritative for setup, coverage,
availability and its leak witness. Index, acquisition reachability, dominating
guards, queued states, guard keys/invalidation, termination summaries and edge
extension share the existing observed candidate pool. Acquisition reachability
retains its SummaryBudget per-query cap. Activation, optional acquisition,
error/presence edges and repeated-guard uncertainty retain their existing
policy; this does not substitute the generic obligation state machine.

The existing bounded guard and successor engines are promoted as Within APIs,
and the generic obligation walk uses those same implementations. Termination
summary inference receives the walk allowance; truncated broker literal
feasibility keeps all successors. Four default APIs became test-only probes
and moved out of production; consuming lifecycle/resource tests use the
bounded API with nil for default-policy comparisons. The extracted successor
function explains activation versus contradiction at its precision boundary.
The stale evaluator responsibility comment is corrected.

Actual SSA cleanup/leak and dominating-guard controls exercise every
insufficient allowance. A sibling classifier query can exhaust the root pool
without setting the walk child's exhausted flag; both limits now invalidate
the proof and clear any tentative witness. Ignoring the parent reproduces the
cutoff-witness failure at allowance 13. A selective nil walk allowance also
fails cleanup/leak and guarded controls. Final counterfactual receipts are
`.build/goal-resource-flow-complete-{parent-cut,unshared}-control.log`.
No audited production FP removal is credited.

The final canonical gate passes generation, module verification, vet,
formatting, lint, dead-code, local dogfood and ordinary tests:
`.build/goal-resource-flow-reviewed-verify.log`. Earlier gates exposed
now-test-only APIs, dependent test references and a rationale span; those were
fixed rather than waived. Focused commentary/helper-reference/documentation
checks pass in `.build/goal-resource-flow-architecture-final.log`.

| Pinned static resource control | Parent | Complete source |
| --- | --- | --- |
| ozontech/cute at 9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3, ./... | Exit 3; reviewed test.go:614:13 leak present. | Exit 3; identical 980-byte JSON; leak retained. |
| ferro-labs/ai-gateway at d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4, ./internal/admin/repository ./mcp | Exit 0; corrected resource finding absent. | Exit 0; identical 2-byte empty-object JSON; absence retained. |

All stderr is empty. Scans use -enable=resourcelifetime -json,
CGO_ENABLED=0, GOFLAGS=-mod=readonly and GOWORK=off. Pins and original
reviews are in indirect-destination-followup-2026-10-01.tsv; no new review is
inferred from diagnostic presence alone. Immutable parent binary
`.build/goal-result-fold-current` SHA-256:
483b7ac406a7f6022d6a2375922dab961ffa89e9dcbd55318b9485c95062f7ce;
complete binary `.build/goal-resource-flow-complete` SHA-256:
f5212b1f6ff660717ae4e9e53502ba548e6b2d8dce4d793cc6d4669b9d48f04d.
The complete binary precedes the final rationale-comment edits; executable
proof behavior is unchanged. These are precommit artifacts, not clean-tree
VCS stamps. Receipts use `.build/goal-resource-flow-{cute,ferro}-{parent,complete}`.

Child `.16` records remaining pre-acquisition and resource-specific presence,
error and owner-query costs. Graph construction, type-system/custom/library
internals and other consumers remain independent scope. Graph MCP is
unavailable; scoped source and actual SSA supply the evidence. No whole-query
wall-clock bound is claimed. The broader architecture and production FP work
remain active. No full precision replay, local race or candidate tests,
generators or applications were run.

## Resource ownership classifier integration review

Beads `gohawk-dho.44.11.5.18.1.5` covers integration of the shared ownership
evidence into resource classifiers. Its nine children group the work by evidence
family. The current source review checks their decision owners and consumers:

| Family | Authoritative decision and availability boundary |
| --- | --- |
| Stored resource/destination | `storage.go:resourceStorage` delegates to `proveResourceStorage`; both settled classification and opaque consumption use the same result. Interrupted proofs are not memoized. |
| Returned wrappers | `flow_returns.go:returnedWrapperWithin` delegates to one result/dominance/containment proof. Unknown results stay uncached and both return policy and instruction classification consume it. |
| Direct/nested carrying and possible wrappers | `carried_values.go` owns the exact selected traversal forms; `possible_wrappers.go` owns the existing constructor depth and retention policy. These may-evidence queries remain distinct from every-return returned-ownership guarantees. |
| Callback and imported loop consumption | `carried_callbacks.go` shares captured-binding matching and the carried-value proof. Callback retention and imported loop release consume its structured result. |
| Observed aggregate escape and paths | `ownership.go` asks observation-time containment once, then effect or contents evidence. `proveAggregateContentsEscapeWithin` propagates the shared relation/path cutoff before the existing whole-aggregate fallback. |
| Publication, async exposure and captured owners | `ownership.go` has one call-result publication census and async-exposure proof; `captured_cleanup.go` owns captured aggregate matching. Their classifiers preserve unknown cutoff reasons. |
| Local call effects | All three resource broker consumers use `CallEffectsWithin` with their allowance and retain the local child cap. The now-unreachable default-only broker method is removed. |

The source lookup covers the named files and their classifier/return consumers.
Graph MCP remains unavailable, so this is scoped source evidence. Each child
has actual SSA allowance/cutoff controls, focused validation and pinned scope
receipts recorded in the resource design note. The final effect child adds
independent aggregate/wrapper/foreign-store proofs, two authoritative classifier
controls and an ignored-cutoff overlay. The first local gate exposed the dead
default-only method; final validation follows its removal and reference update.

Remaining work stays explicit in the larger cost audit: pre-acquisition owner
discovery and the prior-registration callback adapter belong to
`gohawk-dho.44.11.5.17`; completion/context queries and guarded cleanup are
tracked by `gohawk-dho.44.11.5.19`. Graph, alias, type and binding internals
have separate boundaries. The two default
derivation calls still visible in `classify.go` concern ambiguous cleanup, not
the ownership families in this table. This review earns no production FP removal
and does not finish the broader consolidation objective or its 15-site queue.

Final validation passed after removing the unreachable broker wrapper:
`make verify` completed, including ordinary tests and repository dogfood.
Immutable `.build/goal-ownership-effects-final-current`, SHA-256
`200e719c1382a4f78fc20eb47f688a68edad73752e54d5db4520985f13f35945`,
keeps pinned Cute/Ferro JSON byte-identical to the observed-path binary, with
exits 3/0 and empty stderr. The ignored-cutoff overlay fails the three independent
proofs and both classifier controls. These receipts complete the bounded
ownership integration review; the larger cost and precision audits remain open.

## Recursive returned-owner completion review

`gohawk-dho.44.11.5.18.1` is reviewed against its original recursive engine,
constructor, storage and view-binding requirements at production source
`9353eb7`. The five direct children are closed; the ownership integration child
has its nine-family review above.

`lifecycle/store_returns.go` keeps alias dispatch at the aggregate query entry,
with the existing aggregate/value pair guard. Stored-address traversal delegates
to `StoredIntoWithin`, whose shared work driver owns address cycles and charges
referrer visits. Same-path lookup uses `AccessPathStepsWithin` and
`SelectionsOfWithin`; captured ownership shares bounded binding and cell-value
mechanics. The final ownership proof checks child/pool availability before
publishing either a positive or completed negative result.

`store_constructors.go` shares the search allowance through arguments, body
census, summary dispatch and one every-return obligation query. It preserves
the nil/error-only unsuccessful-construction exception. `flow_returns.go`
retains distinct possible ownership and strict returned-wrapper guarantees;
projection/view binding cutoff is unknown before method-set fallback.
The broker delegates declared views to the fact binder, which retains its
storage child cap and ambiguous-alias exclusion.

Constructor parameter indexing was checked against actual SSA before approving
the review. Direct methods carry the receiver in `Args`; invoked interface
methods have no static callee and are declined by this engine. The new
`store_constructor_binding_test.go` covers direct, dynamic and boxed-concrete
call shapes and interrupted allowance. The completed decline means this model
found no owner; it does not prove that an opaque constructor cannot retain input.

Fresh focused lifecycle, broker and resource controls passed, covering recursive
ownership, delegated successful constructors versus uncovered returns, stored
and copied aggregates, callback/pool cutoff, wrapper depth and returned-view
binding. Existing production source remains covered by the last canonical gate
and pinned Cute/Ferro receipts; the new regression and this review receive their
own local completion gate. Source evidence is scoped because graph MCP remains
unavailable. Graph/type/alias, callee-resolution and summary-hook internals retain
separate cost scope. Pre-acquisition and cleanup uncertainty remain `.17` and
`.19`; no new production FP removal or full precision replay is credited.

The review's completion gate passed (ordinary tests 7 seconds, repository
dogfood 26 seconds), with lint and architecture checks passing as well.
The constructor regression records the actual direct and invoke call shapes.
Production proof source is unchanged from `9353eb7`, so its previously recorded
immutable Cute/Ferro receipts remain applicable; no new scoped scan is claimed.
The next concrete pre-acquisition boundary is owner discovery: the instruction
census and alias deduplication run before pool creation. Child `.44.11.5.17.1`
tracks sharing that census and the existing storage proof under one allowance.

The enclosing `gohawk-dho.44.11.5.18` return-disposition requirements are also
covered: `proveResourceReturn` requests the shared returned-owner proof and
`ReturnedMayAliasAnyWithin`, checks child/pool availability, and admits an
uncovered-return witness only after both complete. The fresh return controls
check direct/nested handoff, an unrelated return, possible owner alias and a
scalar observation, with interrupted proofs carrying no leak witness. The
recursive child review and existing scoped receipts finish this bounded return
family; pre-acquisition and cleanup uncertainty remain in the larger cost audit.

## Resource owner-discovery census

`gohawk-dho.44.11.5.17.1` replaces the pre-pool manual owner scan with
`owner_discovery.go`. The existing observed candidate pool is initialized once
before discovery and reused by subsequent flow queries. Instruction visits,
storage dispositions and owner alias deduplication share its release-query
allowance. A completed census atomically installs possible owners and the
classifier's existing exact-store cache. Interrupted evidence discards both
outputs and produces budget uncertainty before flow classification. Possible
owners still imply neither release nor exact ownership; foreign, local,
unrelated and opaque destinations retain the original disposition policy.
The memory-writer exemption remains ahead of all candidate work.

Actual-SSA controls cover those destination families at every insufficient
allowance, a child cutoff with parent allowance remaining, fresh complete
commitment, storage-cache reuse without an available pool, and ordinary
leak/release flow. The unbounded-discovery overlay fails all four family cutoff
controls and the child cutoff. The initial generated fixture exceeded 250,000
calls and took two minutes; the final controls inject a small allowance at the
same authoritative discovery boundary and complete in milliseconds, preserving
the production quota. The late-populated captured-owner control uses the same
census. Graph, alias and type internals retain independent cost scope; earlier
acquisition predicates and prior-cleanup registration remain in `.17`.

The canonical `make verify` gate passed (ordinary tests 56 seconds, repository
dogfood 28 seconds), with focused lint and architecture checks also passing.
Immutable `.build/goal-owner-discovery-current`, SHA-256
`0947b3048ab947c9742acb8a6235e3fa3e0e5bb2c65bfd41e19e064cf69cc00a`,
keeps pinned Cute (`9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`, `./...`)
and Ferro (`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`,
`./internal/admin/repository ./mcp`) resource JSON byte-identical to the final
ownership-effects binary. Terminal exits are 3/0 and stderr is empty. Cute's
known TP remains and Ferro's corrected statement-storage FP stays absent.
No additional FP removal, new full-corpus precision result or full precision
replay is credited.

## Prior cleanup registration census

`gohawk-dho.44.11.5.17.2` replaces the default Boolean prior-registration
query with `provePriorCleanupWithin`. Both instruction censuses share the
existing candidate pool; the result carries the authoritative reason and
witness instruction for tracing. Deferred witnesses retain precedence over
known testing cleanup registrations. Interrupted evidence returns budget
uncertainty before the ordinary resource flow; a complete may-cleanup witness
still supplies opaque consumption, never exact release.

The captured-cell query now uses shared bounded binding, instruction and access-
path traversal. It preserves the former anywhere cleanup policy without adding
an every-return guarantee. The same proof feeds `opaqueClosureCall`; cutoff
cannot make a deferred callback transparent. Prior testing registrations use
the existing single-step closure recognizer and ordinary carried-value proof,
so the obsolete `carriedWithinClosure` adapter is removed. SQL parent identity
queries retain independent storage costs; child `.17.3` tracks those shared
contracts across both prior registration and ordinary classification. Earlier
acquisition predicates and graph/alias/type internals remain separate work.

`prior_cleanup_test.go` records actual SSA for mutable captured cleanup, testing
registration, unrelated captures, by-value defers, later registration, a leak
and exact release. Controls cover every insufficient allowance, a child cutoff
while the parent remains available, fresh recovery and deferred-closure
classifier cutoff. An overlay ignoring both proof allowances fails all six
prior-registration controls, all three captured-cell controls, the child cutoff
and the classifier cutoff. Focused controls pass in 0.060 seconds.

The first focused gates exposed nested control flow and missing reason-
precedence rationale; both are corrected. The canonical `make verify` gate
passes (ordinary tests 75 seconds, repository dogfood 38 seconds). The immutable
`.build/goal-prior-cleanup-current`, SHA-256
`a261d98b3bcc8c7c3affa5df5b4050866de58ab44de14ef5720925ca9e6ba3e5`,
keeps the prior owner-discovery Cute/Ferro JSON byte-identical, terminal exits
3/0 with empty stderr. Pins and scopes remain those recorded above. The known
Cute TP remains; Ferro's corrected storage report stays absent. These scoped
receipts credit no new FP correction or full-corpus precision result. No full
precision replay ran.

## SQL parent identity consolidation

`gohawk-dho.44.11.5.17.3` extracts DB statement-parent and Tx rows-parent
contracts into `sql_parents.go`. One structured proof supplies prior deferred
registration and ordinary classification. The original symbol and exact-parent
policies remain: DB.Close gives uncertain pooled-statement retirement, and
finishing the exact Tx gives uncertain asynchronous rows cleanup. Neither is
synchronous child release. Tx-prepared statements must resolve to the exact
constructor result; replaced or sibling statements retain their obligations.
Durable contract rationale and motivating pinned links moved with this policy.

Point-in-time identity and Stmt receiver resolution retain separate QueryBudget
child caps but spend their caller's allowance. Child cutoff propagates unknown
before either consumer can continue; a complete unmatched storage query means
no contract recognized, not proof that parents differ. The prior-defer helper
keeps captured-cell, SQL parent and paired-context reason precedence without
repeating the parent decision. Paired-context cancellation remains its separate
mechanical contract. Graph/type costs and earlier acquisition predicates remain
independent. Child `.17.4` tracks the pre-acquisition defer census, which still
allocates a fresh completion allowance for each dominating defer.

`sql_parents_test.go` covers twelve exact/sibling/mixed/replaced/saved parent
cases, captured DB cells, direct Tx rows and Tx-prepared statements at every
insufficient allowance. Actual SSA in `.build/goal-sql-parent-fixture.ssa.txt`
shows distinct loads before/after captured-cell reassignment and exact Prepare
result extraction. Both consumers have cutoff/fresh reason controls. A long
actual conversion chain exhausts the storage child while the parent remains
available; the short chain succeeds. Focused SQL controls pass in 0.197 seconds.
The ignored-allowance overlay fails all twelve proof controls and both ordinary
classifier controls. A second overlay ignoring child availability fails the
storage-cap control with an incorrect completed negative while the parent is
still available. Existing conditional SQL-parent regression fixtures remain.

Immutable `.build/goal-sql-parent-current`, SHA-256
`ef1f9413166b880e192b38f7e7bfa71a3348fd7e1e473c9876dd58bd04c4aa5c`,
keeps the pinned Cute/Ferro resource JSON byte-identical to the prior-cleanup
binary, terminal exits 3/0 and empty stderr. Pins/scopes remain those recorded
above. Cute's known TP remains and Ferro's corrected storage FP stays absent.
No new production FP correction or full precision replay is credited.

The first canonical gate failed the existing candidate-observed helper
completion test: spending SQL allowance before symbol applicability intercepted
an unrelated helper's exhaustion event. Applicability now precedes the query,
so unrelated calls retain their authoritative completion path. The focused
SQL/prior/helper controls pass in 0.278 seconds; the ignored-allowance overlay
still fails the same twelve proof and two classifier controls on final source.
The final immutable `.build/goal-sql-parent-final-current`, SHA-256
`6198b1062b66d2d456fea3ffd623346606f8d5a8019eaef33863d9e3cb745910`,
was built separately and its terminal pinned Cute/Ferro results remain
byte-identical to the prior-cleanup baseline, exits 3/0 and empty stderr.

The final canonical gate passes, including ordinary tests (86 seconds) and
repository dogfood (44 seconds). Final documentation/architecture checks pass.
The bounded SQL parent task is complete; earlier acquisition work remains open.

## Pre-acquisition deferred completion

`gohawk-dho.44.11.5.17.4` replaces the default defer census and independent
per-defer completion budgets with one structured query in
`prior_deferred_completion.go`. The candidate infrastructure is constructed
once after existing memory/HTTP/error/optional gates, with the final optional
resource binding. Its observed pool now feeds the dominating-defer instruction
census and every nested CoverageAnywhere completion request, then owner
discovery, prior registration and ordinary flow. A completed witness retains
prior-defer may-release uncertainty, never exact settlement; cutoff is explicit
budget unknown rather than a completed negative.

Six actual-SSA controls cover mutable conditional capture, unrelated and
by-value defers, non-deferred registration, later registration and no cleanup.
Multiple-defer, child-cutoff and fresh controls retain the same source policy.
A nested fixture has enough allowance for its whole caller census but too
little for the deferred body: giving completion a fresh independent budget
fails that control. The ignored-census allowance overlay fails all six family
controls and the child cutoff. Existing full-flow leak/release, prior-cleanup
and helper-observer controls remain passing. Actual SSA receipts are in
`.build/goal-prior-deferred-fixture.ssa.txt`.

The nested recovery control exposed a real shared cache defect, child
`.17.4.1`: `LocalEvidence.Completion` retained EvidenceBudgetExhausted under a
key that contains no allowance. A later larger query returned that stale
cutoff. Interrupted proofs now bypass cache publication; complete proofs retain
ordinary reuse. A layer-local actual-SSA regression checks cutoff/no-cache,
fresh successful completion and completed cache reuse. Reinstating the old
cache publication fails it. Resource nested fresh recovery also passes. The
per-request CallContract lookup's separate cache-policy question is tracked
under `.44.11.5.20`; it is not assumed fixed by this budget correction.

The canonical gate passes, including ordinary tests and repository dogfood.
Immutable `.build/goal-prior-deferred-current`, SHA-256
`95b2c0abea7f1c355dae7c2f548ed743f730aa7b7dc92b6607ec58dcd8accf2b`,
keeps pinned Cute/Ferro resource JSON byte-identical to the final SQL-parent
binary, exits 3/0 with empty stderr. Pins/scopes remain those recorded above;
Cute's known TP remains and Ferro's corrected statement-storage report stays
absent. No new FP correction or full precision replay is credited. Earlier
HTTP/context/error/optional predicates and graph/type internals remain separate
cost scope; parent `.17` and the broader objective remain open.

## Request-specific completion contract cache

`gohawk-dho.44.11.5.20` confirms that LocalEvidence's outer completion memo
omitted the per-request CallContract policy. Actual SSA contains a caller
forwarding its exact resource into a visible empty contract callee. A trusted
request hook recognizes that exact argument and Close query; without the hook,
the body supplies no cleanup. Before the fix, all four policy-order controls
fail: a cached negative blocks a later accepted contract, and a cached accepted
contract remains a positive guarantee after the hook is removed or rejects it.

CallContract now joins the existing per-request lookup bypass. Callback identity
is not a stable cache key, so only the request's internal memo may reuse those
answers. The fixed returned-summary policy still belongs to the evidence scope;
unadorned completed proofs retain ordinary memoization and interrupted proofs
remain uncached. No additional lookup engine or analyzer policy is introduced.
The CompletionRequest comment, architecture reference and generated lifecycle
API reference describe the same boundary.

`evidence_contract_test.go` covers none/accepts, accepts/none, accepts/rejects
and rejects/accepts transitions followed by a repeated policy. Corrected tests
pass with the ordinary memo-reuse and cutoff/fresh controls in 0.010 seconds.
Actual SSA is recorded in `.build/goal-completion-contract-fixture.ssa.txt`.
The first focused gates found test line length and a generated comment reference
that needed regeneration; both are corrected by the canonical workflow.

The canonical `make verify` gate passes, including ordinary tests and repository
dogfood. Immutable `.build/goal-completion-contract-current`, SHA-256
`b883426807cacc88c7a71550eaa017a30cb278062d937edf744043337e1fd98d`,
keeps pinned Cute/Ferro resource JSON byte-identical to the prior-deferred
binary, exits 3/0 and empty stderr. Pins/scopes remain those recorded above.
Cute's known TP remains and Ferro's corrected storage report stays absent.
The current production contract lookup callsite uses the direct conditional-case
search; this memo correction has no demonstrated production FP removal. No full
precision replay ran. Earlier acquisition and cleanup-uncertainty families remain
open in the broader consolidation objective.

## Canceled acquisition context census

`gohawk-dho.44.11.5.17.5` replaces the default pre-acquisition cancellation
call census with one structured proof. Exact standard constructor result-zero
context and result-one cancel pairing remains unchanged; only an ordinary
paired invocation dominating DB.PrepareContext, DB.QueryContext or DB.BeginTx
supplies the existing canceled-acquisition exclusion. Conn/Tx/Stmt entry behavior,
deferred/conditional calls, sibling/replaced contexts and opaque context
parameters do not acquire that guarantee. Pair applicability precedes scanning.

The census uses InstructionsWithin and a child allowance from the observed
candidate pool. Probe and pool are constructed after existing memory/HTTP policy
exclusions and handed unchanged to resourceAnalysis after acquisition-error and
optional-resource binding. Later owner, defer and ordinary flow queries reuse
that same pool; cutoff returns explicit budget unknown before leak evidence.
Paired context contracts now live together in acquisition_context.go, retaining
all durable API rationale and motivating pinned links. Later transaction
cancellation remains uncertain asynchronous cleanup rather than exact rollback.
No alias walk or deadline timing inference is introduced.

Actual-SSA controls cover four positive API/cause cases, five eligible negative
ordering/identity variants, five excluded API/opaque forms, every insufficient
allowance, child cutoff with parent available, fresh recovery and complete-flow
canceled versus independent statement-leak witnesses. Focused controls pass in
0.152 seconds; ignoring census allowance fails all nine eligible families and
the child cutoff. The SSA receipt shows the exact context/cancel extracts and
ordinary cancel invocation in `.build/goal-acquisition-context-fixture.ssa.txt`.
Lint and architecture gates pass. The canonical completion gate passes,
including ordinary tests (75 seconds) and repository dogfood (33 seconds).

Immutable `.build/goal-acquisition-context-current`, SHA-256
`4a01def69a4c6f39b1b0fdb965811bd1dc62ff80467ae488b16a10f458840773`,
keeps pinned Cute/Ferro resource JSON byte-identical to the contract-cache
binary, terminal exits 3/0 with empty stderr. Pins/scopes remain those recorded
above; Cute's known TP remains and Ferro's corrected storage report stays absent.
No additional production FP correction or full precision replay is credited.
Child `.17.6` tracks optional-diamond reachability/phi census; earlier HTTP/error
predicates and graph/type internals remain separate work. The parent and broader
consolidation objective remain active.

## Optional acquisition diamond allowance

`gohawk-dho.44.11.5.17.6` shares the existing candidate allowance across cycle
exclusion, merge instruction/phi predecessor visits and nil alternatives. The
shared bounded reachability adapter delegates to the existing CFG engine;
there is no new traversal or parallel diagnostic decision. Strict direct
acyclic diamonds, unique exact resource/error phis and repeated equality/inverse
arm selection retain their previous policy. Completed declines continue normal
flow; cutoff returns budget unknown and discards all correlation fields before
any resource binding. HTTP/error predicates and graph/type internals remain
outside this change.

Actual-SSA controls cover exact/inverse guards, a retained leak, unrelated
guards/resources/errors, boxed typed-nil errors, cycles, insufficient allowances,
child cutoff and fresh recovery. The shared reachability control checks both a
reachable arm and unreachable sibling after a child cutoff with parent available.
Focused shared/analyzer controls pass (0.005/0.245 seconds). Ignoring the
allowance or discarding the final exhaustion check each fails all eight proof
families and the child-cutoff control. Actual SSA is retained in
`.build/goal-optional-acquisition-fixture.ssa.txt`, including the paired nil
phis, inverse comparison and boxed typed-nil alternate error. An initial gate
found one overlong test-table line; splitting it corrects lint. The final
canonical gate passes all checks (`.build/goal-optional-acquisition-final-verify.log`).

Immutable `.build/goal-optional-acquisition-current`, SHA-256
`8be792a45e7a6e0019fc1b43b172f6eef138ab3e74cc938ce069bada8a0bd036`,
retains byte-identical pinned Cute/Ferro resource JSON against the canceled-
context baseline: terminal exits 3/0, empty stderr. Pins and scopes remain those
recorded above. Cute's known TP remains and Ferro's corrected storage report
stays absent. No production FP removal, full precision replay or local race run
is credited. Children `.17.7` and `.17.8` track acquisition-error assertions and
HTTP boundary cost families; the parent and broader goal remain active.

## Acquisition-error assertion allowance

`gohawk-dho.44.11.5.17.7` moves the acquisition-error contract out of flow
orchestration into `acquisition_error.go`. One structured proof supplies the
existing exclusion: fatal require Error/NotNil on the acquired error, or a
nonfatal HTTP error claim dominating a Nil assertion on an alias of the resource.
Census, ordering, argument visits, derivation and alias dispatch share the
observed candidate pool through one child. Unrelated instructions are filtered
by exact registered assertion contract before requesting CFG evidence. This
avoids the former order query for every instruction without changing matches.
Graph construction and alias-query internals remain independent costs.

An interrupted census discards both assertion lists; the authoritative proof
returns budget unknown before flow may accept the exclusion. The existing
fatal versus nonfatal HTTP distinction, paired resource/error semantics and
motivating pinned rationale move together. No exported fact schema changes.
Actual-SSA controls cover 22 require/assert contract variants, functions and
methods, NotNil/interface conversion, exact and non-HTTP pairs, reversed
ordering, sibling branches, unrelated error/resource inputs, earlier claims and
ordinary leak/release flow. Every insufficient allowance, child cutoff with
parent available, fresh recovery and partial-list discard are exercised.
Focused tests pass in 0.182 seconds. Ignoring allowance fails every contract
variant and child cutoff; removing the census exhaustion guard publishes a
partial error-assertion list and fails its focused control. SSA is retained in
`.build/goal-acquisition-error-fixture.ssa.txt`. Focused lint found an overlong
signature, corrected by splitting its parameters.

Immutable `.build/goal-acquisition-error-current`, SHA-256
`2b2e59c0a6d93a82356943974ec5f4604eea974e456bbd9e315f1d8d7d5a203b`,
retains byte-identical pinned Cute/Ferro resource JSON against the optional-
diamond baseline: terminal exits 3/0, empty stderr. Pins/scopes remain those
recorded above. Cute's known TP remains and Ferro's corrected storage report
stays absent. The canonical gate passes all checks, including ordinary tests
(98 seconds) and repository dogfood (52 seconds), in
`.build/goal-acquisition-error-verify.log`. Focused documentation checks pass. No production FP removal or full precision
replay is credited; HTTP boundary `.17.8` and the broader goal remain open.

## Shared visible HTTP default effects

`gohawk-dho.44.11.5.17.8.1` consolidates HEAD's root default-mutation scan and
the local-header-only helper scan into `http_default_effects.go`. One engine
owns instruction census, exact default-client/transport operand detection and
visible-callee expansion. HEAD's root alone permits a default-client load used
exclusively by Do; helper summaries and local-server effects keep the strict
policy. The caller-specific root allowance is never cached as a declaration
summary. Existing default-effect symbols are reused instead of redeclared.

Instruction charging uses InstructionsWithin and retains the 4,000-step quota.
Recursion and shortened queries retain possible-modification answers, and the
existing shared memo discards interrupted summaries. No candidate-budget
composition, referrer/provenance policy, alias/type or operand-internal cost
change is claimed. Parent `.17.8` remains open for those families.

Actual-SSA controls distinguish direct and nested default-client Do, ordinary
and harmless helper bodies, client/transport stores, field mutation, nested
mutation and recursion. Insufficient allowances retain possible effects; a
fresh allowance on the same memo recovers a harmless body after child cutoff.
The counterfactual that lets the root allowance enter helper summaries fails
direct and nested Do strict-policy controls. The actual-SSA receipt is
`.build/goal-http-default-effects-fixture.ssa.txt`. A final focused control
also proves that a strict cached answer neither intercepts nor gets overwritten
by the root-only query; focused tests (0.218 seconds) and lint pass after that
addition. The canonical gate passes all checks, including ordinary tests
(75 seconds) and repository dogfood (34 seconds), in
`.build/goal-http-default-effects-verify.log`. Documentation checks pass.

Immutable pre-commit `.build/goal-http-default-effects-current`, SHA-256
`89d4d85c40e66a21f90534b32e8ec1d351ffa5b7a9baa8e9bb71e845cb59b738`,
retains byte-identical pinned Cute/Ferro resource JSON against the acquisition-
error baseline: terminal exits 3/0, empty stderr. Pins/scopes remain those
recorded above. Cute's known TP remains and Ferro's corrected storage report
stays absent. No production FP removal, full precision replay or local race run
is credited; parent `.17.8` and the broader consolidation goal remain active.

## HEAD request and client allowance

`gohawk-dho.44.11.5.17.8.2` composes HEAD origin, immutable request/header uses,
fresh/stable/captured client provenance and default effects under the observed
candidate allowance. Request and client models move into focused files, keeping
existing durable rationale and source links with the applicability proof. Shared
reaching folds replace direct recursive value walks while retaining direct
call/extract chains and opaque phis; backward origins and forward uses have
independent visited sets but one allowance. Referrer and capture visits charge
the same child. Known HTTP Do identity reuses the existing symbol declaration.

The default-effect child keeps its 4,000-step cap and explicit availability.
A shortened child no longer becomes a modified-client decline followed by
ordinary leak reporting. HTTP orchestration preserves the returned budget reason;
HEAD still supplies acquisition uncertainty, never exact cleanup. The pool now
starts after the memory policy exclusion and before HTTP queries and is reused
through later candidate binding and ordinary flow. Local-server provenance,
handler effects and graph/type/alias internals remain `.17.8.3`/broader work.

Thirteen actual-SSA cases cover exact/context/cloned HEAD, headers, default and
captured clients, GET, phi/opaque inputs, mutation/escape and configured clients.
Controls cover all insufficient allowances, child cutoff with parent available
and fresh recovery. A visible harmless helper exceeding the default child cap
retains budget unknown in complete resource flow. Ignoring caller allowance
fails eleven queried families plus child recovery; checking only the parent
instead of the default child fails availability and emits a leak on the same
fixture. SSA is retained in `.build/goal-head-allowance-fixture.ssa.txt`.
Focused tests pass in 0.291 seconds; lint and architecture pass. Initial gates
requested rationale for the two fold directions and the named TransparentNone
mask instead of literal zero; both are corrected. The final canonical gate
passes in `.build/goal-head-allowance-final-verify.log`, including ordinary tests
(4 seconds). No full precision replay or local race run was performed.

Immutable pre-commit `.build/goal-head-allowance-current`, SHA-256
`712060807c81a940a65b50325e2af7540fae92fccd5ed2ac4c9834a97a711014`,
retains byte-identical pinned Cute/Ferro resource JSON against the shared-default-
effects baseline: terminal exits 3/0, empty stderr. Pins/scopes remain those
recorded above. Cute's known TP remains and Ferro's corrected storage report
stays absent. No production FP removal is credited. Local-server `.17.8.3`,
the parent and the broader consolidation objective remain active.

## Local HTTP endpoint and writer allowance

`gohawk-dho.44.11.5.17.8.3` composes exact server/client referrers, selected handler
reaching-value resolution and visible default/writer effects with the observed
candidate pool. Endpoint provenance and writer framing now have separate files
and reasons to change. Existing exact identities, transparent forms, cookies,
nonframing headers, nonredirect statuses and visible-helper contracts remain.
Completed declines continue the ordinary resource proof; interrupted provenance
or the existing 4,000-step effect child returns explicit budget unknown. HEAD
uncertainty and positive local bodyless protocol evidence remain distinct.

Writer instruction, operand, alias-dispatch, binding and header visits charge
that child; shared default-effect operand visits also charge their supplied
allowance. Graph/type/alias and list materialization internals remain separate
costs. Existing memo invalidation permits fresh writer proofs after cutoff.
Thirteen actual-SSA cases cover direct/server-client/path acquisition, cookies,
headers, helpers, body writes, framing, redirects, changed endpoint/client,
opaque handlers, other clients and visible defaults. Controls include all
insufficient allowances, parent-available child cutoff, fresh writer memo recovery
and oversized header-only handler cutoff in complete resource flow.

Focused tests (0.770 seconds), lint and architecture pass. Lint found duplicate
HEAD/local cutoff and source-construction scaffolding; `http_allowance_test.go` now owns
those shared controls while each protocol keeps its source fixture. Ignored-
allowance and unchecked-child counterfactuals plus SSA receipts are retained
under `.build/goal-local-http-*`: ignoring allowance fails all thirteen families
and child cutoff; unchecked effect-child availability emits a leak on the
oversized header-only handler. Actual SSA shows the exact server URL field,
server Client receiver and selected handler boxing in
`.build/goal-local-http-fixture.ssa.txt`. The canonical gate passes all checks
in `.build/goal-local-http-verify.log`, including ordinary tests (50 seconds)
and repository dogfood (26 seconds).

Immutable pre-commit `.build/goal-local-http-current`, SHA-256
`de24eea63dfbf5d9b93f7138a695a3e204191c7e6f9e38501f7b0ae6d5fe9781`,
retains byte-identical pinned Cute/Ferro resource JSON against the HEAD baseline:
terminal exits 3/0, empty stderr. Pins/scopes remain those recorded above.
Cute's known TP remains and Ferro's corrected storage report stays absent.
No production FP removal, full precision replay or local race run is credited.

The HTTP parent `.17.8` has completed its scoped evidence families. Review of
broader pre-flow setup found acquisitionErrorResult still using default
CallResult before pool construction; `.17.9` records bounded result decoding
and availability before the broader `.17` can close. MemoryWriterExempt remains
its explicit one-wrapper/symbol/type policy exclusion. Graph/type/alias and list
materialization internals plus cleanup uncertainty `.19` remain broader work;
the overall consolidation objective is not complete.

## Acquisition error-result lookup and pre-flow completion review

`gohawk-dho.44.11.5.17.9` replaces the last unbudgeted input lookup found in
`evaluateResourceFlow`'s scoped pre-acquisition review. The memory-writer policy
exclusion stays first. The observed candidate pool is then constructed before
`proveAcquisitionErrorResultWithin`, which retains the last-error tuple/type
contract and delegates exact extract selection to `ssaflow.CallResultWithin`.
Child or shared-parent cutoff produces unknown with no value and stops the
entry before feasible owned paths are selected. A completed absent extract is
still permitted; no-error metadata exclusions do not start a query.

Nine actual-SSA lookup families cover pair/triple results, blank and unused
error bindings, a discarded call, scalar calls, non-error final slots, non-last
errors and concrete error implementations. Child/fresh and shared-parent
controls pass, along with full-flow cleanup and leak controls. An unbudgeted
counterfactual fails four queried families and the child control; unchecked
availability fails those families and both child and parent controls. A
separate overlay limits only the entry decoder to one visit and supplies a
three-result acquisition with an unknown global error. Its integration control
returns budget unknown. Removing the entry's availability check produces a
leak witness on the failed acquisition return (`state:2`, reason70), failing
the control. Overlay artifacts are `.build/goal-acquisition-result-*`.

The first completion gate found integer-range test lint and missing model
rationale in the enlarged entry. Both are corrected. The final canonical
`make verify VERIFY_TIMINGS=1` passes ordinary tests (53s), repository dogfood
(25s), vet, lint, deadcode, formatter, module verification and generation;
receipt `.build/goal-acquisition-result-completion-verify.log`. No local race
or full precision-regression audit was run.

The parent review matches each explicitly scoped pre-acquisition family to its
current authoritative decision and closed child receipts:

| Family | Current decision | Receipt owner |
| --- | --- | --- |
| Owner discovery | `discoverResourceOwnersWithin` publishes only complete owner/storage candidates | `.17.1` |
| Prior cleanup registration | `provePriorCleanupWithin` returns structured availability before consumption | `.17.2` |
| SQL parent identity | Exact parent proof uses bounded selection | `.17.3` |
| Prior deferred completion | `proveDeferredBeforeAcquisitionWithin` charges census and completion and preserves anywhere may-release uncertainty | `.17.4` |
| Canceled acquisition | `proveAcquisitionContextCanceledWithin` bounds the standard paired-cancel census | `.17.5` |
| Optional acquisition | `proveOptionalAcquisitionWithin` clears interrupted correlation | `.17.6` |
| Acquisition assertions | `proveAcquisitionErrorWithin` bounds fatal/nonfatal error evidence | `.17.7` |
| HTTP boundary | HEAD/local/default-effect proofs share caller allowance and check child availability | `.17.8` |
| Paired-error decode | `proveAcquisitionErrorResultWithin` distinguishes unavailable from absent input | `.17.9` |

The scoped `.17` requirement is now covered; this does not complete the broader
transitive-cost or consolidation objective. At that review, post-construction
guard discovery consumed a slice-only lifecycle API whose defer/capture census
was independent of the completion request allowance. New `.44.11.5.21`
recorded that discovery and partial-list availability review. Cleanup uncertainty
`.44.11.5.19`, graph/type/alias/list internals and the remaining production FP
families remain outstanding. Graph MCP tools were unavailable; this review uses
scoped source reads and makes no graph coverage or repository-wide absence claim.

Immutable `.build/goal-acquisition-result-current` has SHA-256
`8c996b7bd11e1959b414e28bd3ce7607d670e699ae1b44cb462ab0a7ef77ffab`.
Pins were confirmed before scoped scans: Cute
`9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3` (`./...`, exit 3) and Ferro
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`
(`./internal/admin/repository ./mcp`, exit 0). Resource-only JSON is byte-identical
to the preceding local-HTTP receipts (980/2 bytes), and both stderr files are
empty. Cute's known TP remains; Ferro's corrected storage FP stays absent.
These controls credit no production FP removal; the frozen queue remains at 15 sites.

## Result-guard discovery availability and setup cohesion

`gohawk-dho.44.11.5.21` closes the discovery gap recorded above. The sole shared
census is `lifecycle.ProveResultGuards`; `ResultGuardsProof` publishes guards only
on completed discovery. Instruction, capture, exact named-result and opposing
completion queries share the request allowance. The bounded named-result query
was the single-cell named-result query (replaced by the shared all-cell census
in `.27.6` below); the old default-only helper and slice-only
guard API are removed after both production consumers migrate. Completed opaque
completion answers keep the previous modeled-guard policy; cutoff is unknown,
with no partial list.

Resource setup publishes only the complete list across cleanup methods and
deduplicates deferred instructions within that new census. It does not filter
fresh results against a previously published list. The exact owner, collection,
prior-defer and prior-cleanup setup sequence now lives in `prepareResourceFlow`,
whose availability result keeps orchestration out of the evidence rules. This
extraction also resolves the entry's cyclomatic-complexity regression from the
new discovery check. Existing anywhere may-release semantics and trace labels
are preserved. Cancellation ownership, the second consumer found by the broader
source search, stops at unknown on unavailable discovery before its obligation
walk. Its deferred-capture filter policy is unchanged.

Shared actual-SSA tests cover error and Boolean guards, unconditional cleanup,
unrelated/wrong-cell/opaque captures and multiple defers, every insufficient
allowance, child cutoff with parent available, fresh recovery and partial-list
publication. Named-result selection has its own cutoff control. Resource tests
cover duplicate methods, repeated fresh discovery, close-on-error success-path
leak and close-on-success cleanup; cancellation tests distinguish ordinary loss,
release and a function whose discovery exceeds its 1,000-step child.

An unbudgeted census overlay fails all eight shared families and the child
control. Removing post-census availability checks publishes one partial guard
at allowance 193 in the two-guard fixture, failing the dedicated publication
control. Artifacts are `.build/goal-result-guard-*`. No production FP correction
is credited by these infrastructure controls, and the frozen queue remains at 15 sites.

The first focused lint pass identified an entry complexity regression and a
long test line, both corrected before the gate. Ordinary tests and repository
dogfood in the first gate pass; the only gate failure is the historical audit
reference to the removed slice API, corrected here. Generated shared-helper
references include the new proof APIs and remove obsolete symbols.

This completes discovery, not all result-dependent lifecycle costs. New
`.44.11.5.22` tracks per-return cell/store/outcome binding and reaching-defer
queries; `.44.11.5.23` tracks the cancellation deferred-capture filter. Cleanup
uncertainty `.44.11.5.19` and broader graph/type/alias costs remain open. Scoped
source reads were used because graph MCP tools are unavailable; no complete
repository graph or absence claim is made.

A flow integration overlay limits only resource guard discovery to one visit.
The setup proof returns budget unknown. Bypassing its availability check instead
returns accepted cleanup (`state:1`, reason50), masking the fixture's known
close-on-error success-path leak; the integration assertion fails. This is a
fixture proof boundary, not a production FP removal.

The final `make verify VERIFY_TIMINGS=1` passes ordinary tests (3s with current
receipts cached), repository dogfood (1s), vet, lint, formatter, deadcode, module
verification and generation. The first gate's uncached ordinary tests and
dogfood took 66s/38s; only its obsolete documentation citation failed.
Receipts: `.build/goal-result-guard-{verify,final-verify}.log`. No local race or
full precision-regression audit was run.

Immutable `.build/goal-result-guard-current` has SHA-256
`794f66a5250d38014c54bdf7f04549c4723c50a0a6df6c39e593671eb4c077de`.
Confirmed pinned Cute `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`
(`./...`, resource-only, exit 3) and Ferro
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`
(`./internal/admin/repository ./mcp`, resource-only, exit 0) keep byte-identical
JSON to the acquisition-result baseline (980/2 bytes), with empty stderr. Cute's
known TP remains and Ferro's corrected storage FP stays absent. Openase's
pinned cancellation controls are additionally replayed below.

Openase pin `e530faf137e764337d5beaaf68af3be159eb17aa` is confirmed;
`./internal/orchestrator` with every check enabled exits3 with byte-identical
JSON to the earlier program-entry-context corrected control (1,827 bytes,
empty stderr). Both reviewed cancellation TPs remain at
`runtime_launcher.go:414:22` and `runtime_process_lifecycle_slice.go:192:22`.
These three pinned scopes are controls, not a latest whole-corpus replay.

## Ambiguous cleanup identity and request availability

Cleanup uncertainty `.44.11.5.19` is divided by its actual evidence contracts:
`.19.1` direct/merged helper cleanup identity, `.19.2` exact correlated-error
cleanup with anywhere coverage, and `.19.3` captured HTTP Body stability and
body-nil coverage. The latter two remain open; their distinct meanings are not
folded into the ambiguous-identity decision.

`.19.1` extracts the classifier's direct condition and Boolean helper into
`proveAmbiguousCleanupWithin`. A completed positive direct receiver derivation,
or merged helper argument derivation plus exact actual-argument completion,
provides an unknown classifier label, never settlement of the exact caller
resource. Shared derivation and completion consume the same candidate child
across arguments and methods. Child or nested completion cutoff supplies budget
unknown rather than a completed absence of possible cleanup. Optional-acquisition
and non-call exclusions still decline before querying. Durable source rationale
links move with the authoritative policy; the classifier now requests its proof.

Ten actual-SSA families cover direct cleanup, merged receivers, merged helper
arguments, owner projections, unrelated origins, read-only/conditional helpers,
exact non-merged helper arguments and overwritten fields. Allowance controls
use fresh evidence per limit, so prior completion memoization cannot conceal
interruptions. A child cutoff leaves the pool available; a fresh query through
the same evidence then proves the existing boundary. Independent optional and
non-call controls require no query. Seven full-flow controls assert unknown for
merged direct/helper cleanup, honored exact direct cleanup and retained leak
witnesses for conditional, read-only, overwritten and unrelated forms. SSA and
outcome receipt: `.build/goal-ambiguous-cleanup-ssa.log`.

Ignoring the allowance or its availability fails all ten queried families and
the child control. An integration overlay limits only the ambiguous-classifier
query to one visit. The correct classifier returns unknown/budget with the
candidate pool available; bypassing that proof result returns a completed
none label, failing the control. Receipts are
`.build/goal-ambiguous-cleanup-{unbudgeted,unchecked,class-cut,class-unchecked}.log`.

Canonical validation passes before and after tightening fresh-evidence tests:
`make verify VERIFY_TIMINGS=1`, final ordinary tests 34s, repository dogfood 3s,
vet, lint, formatter, deadcode, module verification and generation. No local
race or full precision-regression audit is run. Immutable
`.build/goal-ambiguous-cleanup-current` has SHA-256
`3014b2da4d418695484913ed56aa9b819bc9c028274b707f5b1317ce32c2d55b`.
Confirmed Cute pin `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`
(`./...`, resource-only, exit 3) and Ferro pin
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`
(`./internal/admin/repository ./mcp`, resource-only, exit 0) retain byte-identical
JSON to the preceding result-guard binary (980/2 bytes), with empty stderr.
Cute's known TP remains and Ferro's corrected storage FP stays absent. This
change credits no production FP removal; the frozen queue remains at 15 sites.
Graph/type/alias and fact-selection callback internals retain independent costs.
Scoped source fallback is used because graph MCP tools are unavailable.

A separate current-helper FP reassessment is recorded as `.dho.4.1`. The pinned
coder source `0845a3bb9eddda5bfc22a94dd3598c90cb842451` shows the agent example
also constructs its child with CommandContext, then kills it on the normal path
before main returns. The process return decision still recognizes unused-command
uncertainty but has no shared one-time entry boundary, unlike cancellation.
This observation changes the next assessment question: can the existing entry
proof support bounded program-lifetime uncertainty without asserting Wait or
child termination? It is not a replay, implementation or credited correction.
Caller intent, loops, referenced entry functions and reusable callee guarantees
must remain distinct.

## One-time entry process ownership

`.dho.4.1` reuses the exact shared entry proof at the authoritative process
return decision. It accepts program-lifetime uncertainty without claiming Wait,
child termination or imported callee cleanup. The synthetic initializer is now
included in main-reference checks; aliases and tables reject at-most-once evidence.
Nine actual-SSA decision scopes, three reporting fixtures and four initializer
controls pass. Both entry-disabled and initializer-disabled source overlays fail
the expected acceptance/diagnostic boundaries. Exact settlement retains precedence.

Canonical gate `.build/goal-process-entry-verify.log` passes (ordinary tests 56s,
local dogfood 29s, vet, lint, formatter, deadcode, generation and module checks).
No local race or full precision replay is run. The
[production correction receipt](../../benchmarks/precision/audits/program-entry-process-followup-2026-10-02.md)
records immutable executable hashes, pins and explicit check selection: four
coder reports present before and absent after, with unknown final traces, and
of-watchdog's reviewed process TP unchanged. Initial analyzer-only scans left
the experimental check disabled and supply no correction credit. The queue
falls from 15 to 11 sites; frozen batch counts are unchanged. Broader architecture
and FP work remain open. Graph MCP tools remain unavailable; scoped source reads
and actual SSA provide evidence, not graph completeness.

## Correlated-error helper cleanup allowance

`.44.11.5.19.2` extracts the second cleanup-uncertainty evidence family into
`correlated_cleanup.go`. The former ownership Booleans are removed; one structured
proof owns exact argument identity, acquisition slot-zero/one correlation or a
later caller nil comparison, and an anywhere lifecycle cleanup witness. All
visits and nested completion spend the request allowance. Interrupted result
selection, a partial instruction census or a nested completion cutoff supply
budget unknown rather than an absent-cleanup label. A positive witness still
means possible cleanup, never settlement. Three-result factory pairing remains
unchanged instead of silently assigning new semantics to its last error slot.

Ten actual-SSA cases retain the previous evidence boundary: paired and later
caller-tested/reversed-nil cleanup, third-result pairing exclusion, uncompared
or previously tested errors, read-only and flag-only helpers, wrong resource and
wrapped error identities. Fresh evidence per limit avoids cache effects masking
interruptions. A child cutoff leaves the parent available and a fresh larger
request recovers. Six full-flow controls retain unknown cleanup and exact leak
witnesses. SSA/proof receipt: `.build/goal-correlated-cleanup-ssa.log`.

The unbudgeted source overlay fails all ten allowance controls and the child
control. A separate integration overlay limits only the correlated-classifier
request to one visit: unknown/budget is retained while the pool remains available.
Ignoring that structured unknown produces an incorrect completed none label
and fails the same integration assertion. Receipts are
`.build/goal-correlated-cleanup-{unbudgeted,class-cut,class-unchecked}.log`.
The canonical gate's first run found one overlong test construction line;
that line is formatted across its named fields before final validation.
No full precision-regression or local race run is part of this iteration.
Production correction credit is unchanged: the queue remains at 11 sites.
Graph MCP tools are unavailable; scoped source and actual SSA provide bounded
evidence. Type-system and underlying graph-construction costs remain independent.

The intermediate gate then required in-body rationale for the extracted proof's
identity and anywhere-coverage boundaries. Those comments now explain why a
possible origin cannot borrow correlation and why a witnessed release cannot
settle the obligation. Focused architecture validation passes in
`.build/goal-correlated-cleanup-architecture.log`.

Immutable `.build/goal-correlated-cleanup-current` has SHA-256
`6348b82e2df9322ba895240279088d8ef93ddef07a8ffcf1f276d775cc50bdbd`.
It implements parent `24622b0` plus this production proof change, before later
rationale-comment additions and test formatting. No executable was replaced
while scanning. Confirmed Cute pin
`9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3` (`./...`, resource-only, exit 3)
and Ferro pin `d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`
(`./internal/admin/repository ./mcp`, resource-only, exit 0) retain byte-identical
JSON to the preceding ambiguity-proof control receipts (980/2 bytes). Both have
empty stderr. Cute's known TP remains and Ferro's corrected statement-storage
FP stays absent. These are successful pinned package controls, not a new corpus
precision measurement, and credit no additional production FP corrections.

Final canonical validation passes in `.build/goal-correlated-cleanup-verify-stable.log`:
ordinary tests 52s, repository dogfood 26s, vet, lint, deadcode, formatter,
module and generated checks. Final documentation/commentary validation also
passes in `.build/goal-correlated-cleanup-docs-final.log`. No broader completion
claim is made; `.19.3` captured HTTP Body uncertainty and the other architecture
and production FP items remain open.

## Guarded captured HTTP Body allowance

`.44.11.5.19.3` extracts called-response capture stability and guarded Body
coverage from prior/deferred cleanup into `guarded_body_cleanup.go`. Its one
structured request requires exact stable cell contents and excludes caller/callee
pointer exposure. Binding visits, storage stability, pointer reaching folds,
derivation, instruction census and guarded-return coverage share the request
allowance. Storage/coverage cutoff is explicit unknown; a positive guarded
witness also means unknown may-cleanup, never exact release.

`lifecycle.ProveMethodCallCoverageWithin` shares the independent return/action
witness census and the ordinary obligation flow. Default MethodCallCoverage
and assumed-argument coverage delegate to that same engine. Every-return and
anywhere semantics retain no-return and nonnil boundaries; no second CFG flow
is introduced. Existing internal completion-search callers still select the
legacy nil coverage allowance, and constant block selection/type-conditioned
coverage has independent work. `.44.11.5.24` records that remaining integration
rather than claiming the entire lifecycle engine bounded. Type/alias and graph
construction costs remain independent.

Eight actual-SSA capture families retain stable, replaced, opaque cell/owner,
map exposure, Boolean guard, unrelated Body and field-replacement boundaries.
Fresh per-limit queries distinguish completed absence from cutoff unknown;
a child cut leaves the parent available and a fresh retry recovers. Four full
resource-flow controls retain unknown guarded cleanup and three leak witnesses.
Ten shared coverage cases under both modes preserve exact, conditional, absent,
no-return and nonnil-guarded answers, with their own child/fresh control. Actual
caller/closure SSA is retained in `.build/goal-guarded-body-ssa.log`.

Ignoring capture budgeting fails all eight families and the child control;
ignoring coverage budgeting fails all ten cases and its child control. Limiting
only the opaque-literal capture request to one visit retains unknown/budget with
an available pool. Ignoring that unknown gives an incorrect transparent literal
and fails the same assertion. Receipts are
`.build/goal-guarded-body-{unbudgeted,coverage-unbudgeted,class-cut,class-unchecked}.log`.
The first gate's behavior tests pass but the architecture requires lifecycle
filenames to name their completion family. The shared implementation and test
are moved to `completion_coverage.go` and `completion_coverage_test.go`;
the focused layering/commentary/documentation gate is rechecked.

Immutable `.build/goal-guarded-body-current` implements parent `1ac2d44` plus
this production change, before the later completion-family filename correction.
Its SHA-256 is
`a8ff5b4df608c638c01ff9306b94384c7a18a45e1a08793c2bd7f5e434485bfb`;
it is never replaced during scans. Cute pin
`9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3` (`./...`, resource-only, exit 3)
and Ferro pin `d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`
(`./internal/admin/repository ./mcp`, resource-only, exit 0) retain byte-identical
JSON to the preceding correlated-cleanup controls (980/2 bytes), with empty
stderr. Cute's known TP remains and Ferro's corrected statement-storage FP
stays absent. No new production FP credit is claimed; queue 11 and frozen
batch totals are unchanged. Scoped source fallback is used because graph MCP
tools remain unavailable. No full precision-regression or local race run is
part of this iteration.

A sibling-analyzer control also retains both reviewed Openase cancellation TPs:
pin `e530faf137e764337d5beaaf68af3be159eb17aa`, `./internal/orchestrator`,
all checks, exit 3, empty stderr and byte-identical JSON to the program-entry
control. Receipts are `.build/goal-guarded-body-openase.{json,err}`.
Every static scan uses `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`;
candidate tests, applications and generators are not run. These pinned scopes
are controls, not an exhaustive latest corpus measurement.

Final `make verify VERIFY_TIMINGS=1` passes in
`.build/goal-guarded-body-verify-final.log`: ordinary tests 78s, local dogfood 45s,
vet, lint, deadcode, formatter, generation and module verification. Focused
layering/commentary/documentation validation passes in
`.build/goal-guarded-body-docs-final.log`. The generated lifecycle helper reference
moves with the exported bounded API. The three classifier cleanup families are
now implemented; the separately recorded shared completion-engine request work
and broader consolidation/FP goals remain unachieved.

## Completion-search coverage integration

`.44.11.5.24` threads the completion search allowance into the canonical
witness/coverage engine, including constant-bound blocks and the exact-type
path. Ordinary public wrappers keep their nil-budget default and delegate to
the same implementation. Exact-type coverage preserves its existing anywhere
witness before the nonnil/type-constrained return query rather than silently
replacing that contract with the ordinary independent return witness.
Conditional coverage now shares queued-state, successor-policy and edge visits
with its instruction/result work. No second CFG coverage engine is introduced.

`ssaflow.ReachableBlocksAssumingWithin` delegates to the shared work-list and
bounded constant narrowing, preserves discovery order and discards interrupted
censuses. The unbounded wrapper delegates to it. Coverage cannot interpret a
partial set of blocks as complete evidence. An interrupted completion case
returns budget unknown, and publication's existing proven-case gate cannot
expose that claim. Memo composition discards interrupted answers; a fresh
larger child can recover without cached absence. Cutoff during an unavailable
callee's summary lookup retains budget reason without inventing body provenance.
An opaque metadata exclusion that spends nothing stays unavailable at limit zero.
Type-system, alias, binding setup and underlying graph construction retain
independent costs; this closes the coverage integration, not all lifecycle costs.

Four constant-bound branch/loop census controls check order and empty cutoff
results. Six actual-SSA completion families cover exact, fixed true/false,
nested, asserted concrete type and missing cleanup. Two case modes cover
result-conditioned and fixed-argument coverage. Two direct assumed-coverage
controls ensure the bounded census and flow are charged independently of the
completion predicate. Child/memo/fresh and unavailable-summary child/fresh
controls retain availability. The fact-pass control sends a complete small
case through the real case inference path and rejects a large interrupted
body with cleanup witnessed before cutoff; the published envelope has no
interrupted discharges. Per-limit proven-case controls cannot expose a path
at cutoff. Existing lifecycle, SSA and fact tests pass.

Coverage-unbudgeted and census-unbudgeted source overlays fail their explicit
zero-limit boundaries; their receipts are
`.build/goal-completion-coverage-{unbudgeted,census-unbudgeted}.log`.
The first canonical gate passes behavior tests and local dogfood but rejects
two overlong production expressions. Named assumption values fix both lines.
No full precision-regression or local race run is part of this iteration;
production FP credit remains unchanged at 11 unresolved sites.

Final canonical validation passes in `.build/goal-completion-coverage-verify-final.log`:
ordinary tests 76s, local dogfood 37s, vet, lint, deadcode, formatter, generation
and module verification. Actual caller/callee SSA and default outcomes are in
`.build/goal-completion-coverage-ssa.log`. The generated SSA helper inventory
includes the bounded reachable-block query.

Immutable `.build/goal-completion-coverage-current` implements parent `37b2c13`
plus this production change, SHA-256
`304cf932a11b18465dcfdaacb5e1440aa8d06ce5575f9a34377c348d15335791`.
Confirmed Cute pin `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`
(`./...`, resource-only, exit 3) and Ferro pin
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`
(`./internal/admin/repository ./mcp`, resource-only, exit 0) retain byte-identical
JSON to the guarded-Body controls (980/2 bytes), with empty stderr. Cute's known
TP remains and Ferro's corrected statement-storage FP stays absent. Scans use
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`; candidate tests,
generators and applications are not run. Neither executable is replaced during
scanning. These scoped controls earn no additional production correction or
whole-corpus precision claim. Broader cost/architecture and FP goals remain open.

The all-checks Openase sibling control at pin
`e530faf137e764337d5beaaf68af3be159eb17aa`, `./internal/orchestrator`, also exits
3 with empty stderr and byte-identical JSON retaining both reviewed cancellation
TPs. Receipts: `.build/goal-completion-coverage-openase.{json,err}`. These same
pins/scopes validate shared coverage use without claiming a latest full audit.
The cleanup classifier review `.19` can close once this `.24` dependency is
committed: its direct/merged identity, error correlation and guarded Body
children are already verified. Other transitive setup/identity work remains open.

### Return-specific result-guard binding (dho.44.11.5.22)

The current source review found that completed guard discovery still fed
unbounded per-return store and defer-registration searches. Both resource and
cancellation consumers now use `ResultGuard.ProveReachesReturn`, one structured
proof over the existing bounded dominance and possible-follow mechanics.
A complete disconnected search contributes no action; a possible registration
or interrupted search remains unknown. One return query allowance also covers
named-cell binding, outcome inference and deferred completion. Summary outcomes
use that same allowance after literal outcomes are considered.

`ValueAtReturnWithin` retains the exact cell's last store before `RunDefers` in
the return block and discards a partial selection at cutoff. The obsolete
unbounded wrapper was removed after deadcode identified its last production
consumer had migrated. Nil-budget calls preserve the default query policy.
Earlier-block assignments and recovery returns without a local store remain
unknown. The two-result conjunction test constructs its binding question
explicitly: discovery still does not infer conjunctions by varying one cell.
This change does not broaden declaration guarantees or resolve the separate
cancellation capture filter in dho.44.11.5.23.

Evidence is task-directed source fallback because graph tools are unavailable;
no graph completeness or whole-lifecycle cost bound is claimed. The shared
actual-SSA tests cover four return-binding families and two registration
families, child exhaustion with an available parent, interrupted outcome
callbacks and fresh queries. Consumer controls preserve released and skipped
cleanup answers after a cutoff. `Function.WriteTo` receipts are in
`.build/goal-return-binding-focused.log`. Counterfactual overlays bypassing
store selection or registration allowances fail at their zero-budget controls
in `.build/goal-return-binding-mutant-{store,reach}.log`.

Focused lifecycle, SSA and both consumer suites pass in
`.build/goal-return-binding-focused-final.log`. The initial verification exposed
a dead wrapper, an unchecked SSA dump error and excessive test complexity;
these were corrected rather than exempted. Final `make verify VERIFY_TIMINGS=1`
passes in `.build/goal-return-binding-verify-final.log`: tests 75s, dogfood 38s,
lint 17s with zero issues, generation 2s, vet 3s, formatting 3s, deadcode 6s
and module verification. No local race or full precision replay was run.

The immutable `.build/goal-return-binding-current` binary was built from
97215b9 plus the production change (before a later comment clarification),
SHA-256 `0f4e3e64078ef41e3c82b5f71c2364a1e2f905fd61686d934e2af555289f54d6`.
Scoped static scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`;
candidate tests, generators and applications are not executed. Cute at
`9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`, resource-only `./...`, exits 3,
retaining the reviewed leak at `test.go:614:13`. Ferro at
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`, resource-only
`./internal/admin/repository ./mcp`, exits 0 and retains the statement-storage
correction. Both JSON receipts are byte-identical to the preceding completion
coverage controls, with empty stderr. These controls credit no FP removals;
the recorded unresolved production queue remains 11 sites, not a fresh corpus
replay. The broader consolidation goal remains active.

Openase at `e530faf137e764337d5beaaf68af3be159eb17aa`, all checks on
`./internal/orchestrator`, exits 3 with empty stderr. Its JSON is byte-identical
to the preceding completion coverage control and retains both reviewed
cancellation leaks at `runtime_launcher.go:414:22` and
`runtime_process_lifecycle_slice.go:192:22`. Receipts use
`.build/goal-return-binding-{cute,ferro,openase}.{json,err}`.

### Cancellation deferred-capture allowance (dho.44.11.5.23)

Task-directed source review found that the cancellation result-guard filter,
store transparency query and directly deferred literal query still used default
once-stored-cell and instruction-order evidence. `WrittenOnceCellWithin` now
owns the same identity engine as the default wrapper, including nested lexical
read-only captures. `ReferrersWithin` shares use enumeration between that engine
and the consumer. The existing cancellation decision is structured as
`proveDeferredCaptureCellWithin` and `proveDeferredCaptureWithin`: exact store,
only directly deferred readers, registration after store. Metadata mismatch and
completed unsupported forms remain rejected; interrupted evidence is unknown.

Guard discovery and its capture filter share one allowance. The retained guard
list is published only after the full filter completes. Store cutoff produces
an unknown store label; deferred capture, guard selection and completion share
one query and stop before any unbounded fallback at exhaustion. Existing label
reasons and obligation flow remain authoritative. The entry point delegates
capture publication and store labeling to the same proof in `result_guards.go`.
The first gate caught a file-size regression in `proof.go`; moving those focused
responsibilities corrected it without raising the configured limit.

Actual SSA checks cover nine capture families: direct, multiple and nested
read-only defers; loaded calls, rewrites, nested writes, pre-store registration,
launches and handoffs. Wrong-target controls remain rejected. Child cutoffs do
not exhaust an available parent; fresh queries recover completed positive and
negative answers. A filtered two-guard census publishes no partial list.
Classifier cutoff controls retain unknown, and full cancellation flow controls
retain release, loss and two opaque ownership outcomes. Shared identity tests
reuse all six existing rejection-cause fixtures and add a statically large
nested reader that exhausts its child allowance. Test fixtures are compiled to
SSA, not executed. `.build/goal-deferred-capture-final-ssa.log` and
`.build/goal-deferred-capture-focused.log` retain `Function.WriteTo` evidence.

Focused SSA, lifecycle and cancellation suites pass in
`.build/goal-deferred-capture-extracted.log`; the expanded nested-reader tests
pass in `.build/goal-deferred-capture-nested.log`. Counterfactual overlays that
bypass the cell, nested-reader or capture allowance fail the zero-budget or
large-reader controls in `.build/goal-deferred-capture-mutant-{cell,nested,capture}.log`.
Final `make verify VERIFY_TIMINGS=1` passes in
`.build/goal-deferred-capture-verify-final.log`: tests 72s, dogfood 30s, lint 5s
with zero issues, vet 1s, formatting 1s, deadcode 4s, generation and module
verification. No local race or full precision replay was run.

The immutable `.build/goal-deferred-capture-final` binary is built from 3fe81a9
plus the final production change, SHA-256
`2dfc0bece63f4092322a33b54f81db5dee7d91365c68a7ec8c41fac7d2468028`.
All-check static scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`:
Openase `e530faf137e764337d5beaaf68af3be159eb17aa` on
`./internal/orchestrator` exits 3 with empty stderr, byte-identical to the
preceding return-binding receipt and retaining both reviewed cancellation
leaks. K8ssandra `2028d352ecb495de4b6e053d99d7a77b21eb5107` on `.` exits 0
with empty stderr and byte-identical empty JSON to its entry-context correction
receipt. `.build/goal-deferred-capture-{openase,k8ssandra}.{json,err}` contains
the observations. Candidate tests, generators and applications are not run.
No new FP correction is credited; the recorded unresolved production queue
remains 11 sites, not a fresh whole-corpus replay.

Other default once-stored-cell consumers remain in fixed-argument binding,
channel aliases, concurrency capture fields and cancellation returned-owner
storage. New dho.44.11.5.25 records their task-directed review before any caller
migration. Graph tools remain unavailable, so the evidence is scoped source
fallback, not a graph completeness claim. Closing .23 certifies this capture
filter and its publication/label integration, not a whole-cancellation cost
bound or completion of the broader consolidation goal.

The partial-publication counterfactual, assigning retained guards after the
first match, also fails the guard-census control at an
interrupted two-guard census (`.build/goal-deferred-capture-mutant-census.log`).
The final focused documentation/architecture check passes in
`.build/goal-deferred-capture-docs-final.log`.

### Fixed-argument binding census (dho.44.11.5.25.1)

The .25 source review identified four default once-stored-cell families. This
child completes fixed-argument binding in completion search and both lock-order
call sites; the channel, concurrency-field and cancellation-owner families
remain open in the parent. Graph tools are unavailable, so this is a scoped
source/usage review, not an exhaustive graph or whole-engine cost claim.

`ProveFixedArgumentsWithin` now returns `FixedArgumentsProof`: a complete
argument/capture outcome census, not a callee-behavior guarantee. Parameter and
capture visits, direct read-only cells, once-stored nested captures and nil-test
relevance share the owning allowance. Cutoff publishes no map, including a
Boolean binding collected before an interrupted later pointer argument.
Missing bodies preserve the completed empty metadata lookup without local SSA
provenance. Caller-fixed current cells retain their existing read-only-body
rule; mutable inferred cells stay unbound. Implicitly zeroed captured cells have
no store and remain unbound, while explicitly stored nil cells still bind.
Typed-nil interface boxing, irrelevant pointers and literal-first outcomes
retain their default behavior.

Completion consumes the structured census before assigning scoped constants,
coverage or memoization. A cutoff cannot prove even an early anywhere-cleanup
witness. Lock root and nested binding share the context allowance with bounded
block discovery. Budget-interrupted contexts drop acquisition witnesses at
the root, rather than reviving unconstrained acquisitions. Complete empty
binding censuses and non-budget depth limits retain the existing ordinary
declaration-summary fallback. Key, type, identity, alias preparation and ordinary fallback
summary costs remain independent. No partial map is a branch-pruning fact.

The added binding work took `callee_locks.go` across its size review trigger.
Its distinct caller-snapshot, embedded-field and constructor-slot mechanics
were extracted unchanged to `callee_lock_bindings.go`, preserving rationale
and pinned links. Summary traversal stays in the original file; its context
search remains cohesive around one allowance and fallback rule. Obsolete
`FixedArguments` and `ReachableBlocksAssuming` default wrappers were removed
once production consumers migrated; tests select the same nil-budget engines.
`ComparesWithNil` remains the default setup query used by summary guard discovery
and delegates to the same nil-relevance predicate as binding.

Actual SSA tests cover twelve argument/capture contexts, nested forwarded
captures, default semantic controls, child/available-parent/fresh queries,
metadata provenance and partial-map discard. The large completion fixture
places cleanup before irrelevant body work, isolating binding cutoff before an
anywhere witness; the same search recovers with a fresh allowance, without a
truncated memo. Lock controls retain both flag arms and demonstrate nested
binding cutoff even when its eventual body would be pruned. Fixtures are built
to SSA, not executed. `.build/goal-fixed-binding-final-ssa.log` contains the
focused semantic controls and `Function.WriteTo` receipts; final focused suites
pass in `.build/goal-fixed-binding-final-focused.log`.

Five counterfactual overlays fail in
`.build/goal-fixed-binding-mutant-{relevance,publication,completion,locks,root}.log`:
bypassing nil relevance publishes a supposedly complete map, publishing an
interrupted census exposes the first Boolean binding, bypassing completion's
binding allowance proves its early cleanup, and bypassing nested lock binding
makes an interrupted context appear complete. The root fallback mutation
revives an unconstrained acquisition after binding cutoff; its control
requires no acquisition witnesses. These are behavioral failures, not
compilation failures.

The initial gates exposed binding complexity, a long test expression, an
unexplained context-search span, an ambiguous earlier audit test citation and
a dead block-census wrapper. These were corrected without exemptions. Final
`make verify VERIFY_TIMINGS=1` passes in
`.build/goal-fixed-binding-verify-cutoff.log`: tests 56s, dogfood 24s, lint 5s
with zero issues, vet 1s, formatting 2s, deadcode 5s, generation 1s and module
verification 1s. No local race or full precision replay was run.

The immutable `.build/goal-fixed-binding-cutoff` binary is built from 255f630
plus the final production change, SHA-256
`6cef5a013a49633b3e7d9989aa665bc4a1202d10c319aea584c8b074c2d69d03`.
Scoped static scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`.
Openase `e530faf137e764337d5beaaf68af3be159eb17aa`, all checks on
`./internal/orchestrator`, exits 3 with empty stderr and byte-identical JSON to
the preceding deferred-capture control, retaining both reviewed cancellation
leaks. Cute `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`, resource-only
`./...`, exits 3 with empty stderr and byte-identical JSON to its return-binding
control, retaining the reviewed resource leak at `test.go:614:13`.
`.build/goal-fixed-binding-cutoff-{openase,cute}.{json,err}` holds the receipts.
Candidate tests, generators and applications are not run. These controls
credit no new FP removal; the recorded production queue remains 11 unresolved
sites, not a fresh corpus census. The .25 parent and broader consolidation
goal remain active.

The final root cutoff rule drops witnesses rather than falling back to an
unconstrained acquisition set. Complete empty binding censuses and non-budget
depth failures preserve the existing fallback. `callee_constants_budget_test.go`
includes an ordinary-summary control that does acquire, a cutoff that must not
revive it, and a fresh completed query proving the arm is pruned. Focused lock
and full architecture suites pass in `.build/goal-fixed-binding-root-reviewed.log`.

A separate read-only overlay probe exposed an inherited timing gap in the old
capture outcome contract: a zeroed local Boolean has its sole true store after
an early return, yet a deferred cleanup conditioned on it is proven by default.
Fixing the caller's cell to false for that return disproves completion. Actual
caller and deferred-body SSA plus both proofs are in
`.build/goal-fixed-binding-timing-probe.log`. New dho.44.11.5.25.2 records this
semantic question for its own temporal proof and regression work; the current
binding change preserves the old complete-query lookup policies. This is not a
verified production FP correction. Parent .25 and the broader goal remain open.


### Captured-outcome initialization order (dho.44.11.5.25.2)

The actual SSA probe above exposed a semantic error in fixed-argument binding:
a deferred literal guarded by `done` was credited with cleanup even when its
parent could return before the only `done = true` store. A synchronous literal
invoked before that store likewise received a true binding. The unique-write
identity contract applies after its store; it does not establish when a
closure reads the cell.

The shared once-stored census now retains its exact store internally. Its
public identity queries still return the same stored value and preserve their
existing budget and read-only rules. Fixed capture inference separately asks
`ssaflow.InstructionDominatesWithin` whether that store dominates closure
creation, sharing the binding allowance. There is no second store census or
launch-specific completion engine. Conditional and later initialization supply
no inferred binding. A late assignment before a synchronous invocation also
stays unbound by this creation-based contract; an exact caller-fixed current
cell can still bind the read-only callee. The existing return-specific named
result analysis supplies such outcomes after the return assigns them. Lexical
forwarding of an established free-variable binding remains unchanged.

The fixed-capture timing cases in
[fixed_arguments_timing_test.go](../../internal/ssaflow/fixed_arguments_timing_test.go)
record caller and callee SSA for ten contexts:
initialization before capture, an early deferred return, invocation before the
store, late initialization before invocation, initialization on the sole path
to capture, a conditional store, later nonnil initialization, and inferred or
caller-fixed named results. The completed census can be empty without asserting
that the parent leaks. Every cutoff discards its map; the child allowance and
fresh-query controls retain the existing publication contract.
The ordering-cutoff control in the same file isolates instruction indexing behind over a
local query's worth of unrelated instructions: ordering exhausts the child
while the parent remains available, and a fresh query recovers the binding.

The
[completion_capture_timing_test.go](../../internal/lifecycle/completion_capture_timing_test.go)
controls check eight cleanup contexts at the
consumer boundary, including both supplied named-result outcomes. The original
implementation fails the temporal controls before the change. An ignored source
overlay removing dominance restores those failures; another making dominance
unbounded fails the isolated ordering-cutoff control. The latter fixture first
selected a padding builtin rather than its deferred closure; selecting the
exact defer corrects that test setup. Both overlays fail behaviorally, rather
than failing to compile.

The focused SSA and lifecycle suites pass. The first full `make verify` passes
with ordinary tests in 107 seconds, dogfood in 51 seconds and zero lint issues;
no race test or full precision replay runs. The final gate additionally covers
the isolated ordering-budget control added after that first receipt. That gate
initially rejected a 163-column test condition and bare test-name citations
reserved for architecture tests; named budget evidence and fixture file links
correct those validation failures. The reviewed final gate passes with ordinary
tests in 6 seconds, dogfood in 1 second and zero lint issues in 4 seconds,
reusing unchanged-package results.

The immutable `.build/goal-capture-timing-current` binary is built from parent
`df21d6c` plus this production correction, before later test and prose edits.
Its SHA-256 is
`17de5c3e807259a7d76d24d94d2557b98ce5395c80bdaab9aa626aa2d1757de0`.
Openase at `e530faf137e764337d5beaaf68af3be159eb17aa`, all checks on
`./internal/orchestrator`, exits 3 with empty stderr and identical JSON to the
fixed-binding cutoff receipt; both reviewed cancellation true positives remain.
Cute at `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`, resource-only on `./...`,
also exits 3 with empty stderr and identical JSON, retaining the reviewed
resource true positive. The candidate tests, generators and applications are
not executed. This corrects the local temporal proof; it earns no production
FP removal. The recorded queue remains eleven sites, and parent .25 retains
its other once-stored consumers and broader evidence review.


### Bounded channel census and pre-initialization snapshots (dho.44.11.5.25.3)

The task-directed source review found one production channel-census consumer:
goroutine ownership's unobserved-signal acceptance. Graph/index tools remain
unavailable, so this review uses scoped source reads and repository symbol
searches. The former alias work queue, cell-referrer census, closure pairing
and static-argument mapping had no caller allowance. More importantly, the
actual SSA probe in `.build/goal-channel-prestore-probe.log` shows a saved nil
channel load before initialization included among aliases of the later
`make(chan int)`. Its `println` was then counted as an observation of the new
completion channel, reviving a missing-join diagnostic for a close-only worker.

`ssaflow.ProveChannelValuesWithin` replaces the default collector with one
structured census. Queued aliases, referrers, unique stores, instruction order,
capture pairs, callee arguments and target insertion spend the same allowance.
Cutoff discards every alias and use, including earlier positive observations.
A direct cell read dominated by its unique store is a modeled alias; a read
that dominates the store in an acyclic block is an older snapshot and is
excluded. A cyclic read can observe an earlier iteration of that store, so
reverse dominance alone cannot exclude it. Other ordering
and captures created before initialization are unavailable rather than guessed.
The exact store comes from the existing once-stored census. Unsupported moves
remain opaque uses, including the existing nested-capture decline; callee
parameters can receive other channels at other sites, so the result does not
claim exclusive identity or an execution path.

The analyzer's `signal_census.go` owns the unobserved-signal policy and returns
one structured proof. It shares the spawn allowance through local-channel
lookup and the census, then requires every use to be builtin close. A complete
non-close observation retains the ordinary obligation; incomplete or temporally
unavailable evidence stays unknown. Tracing consumes that same decision with
the stable `signal-census-unavailable` reason and attributes budget evidence to
the candidate's `signal-census` phase. There is no alternate tracing decision
or ordinary-summary fallback that can revive a truncated answer.

The
[channel_aliases_budget_test.go](../../internal/ssaflow/channel_aliases_budget_test.go)
controls record actual parent/closure SSA for ordinary capture, static
send-only forwarding, escaping storage, older snapshots, late captures,
conditional initialization, unsupported nested captures and cyclic pre-store
reads. The latter actual SSA control fails before the cycle restriction and
retains unavailable evidence afterward. Every allowance
boundary agrees with the completed default census or discards both outputs.
A larger census discards an already collected close use on child cutoff while
leaving its parent available; a fresh request completes. Consumer controls in
[signal_census_test.go](../../internal/analyzers/concurrency/goroutineownership/signal_census_test.go)
prove that the old nil snapshot creates no observation protocol, check
child/fresh behavior, and keep cutoff from reporting an unowned return. The
cutoff test also checks its attributed structured evidence event. Accepted
pre-store and diagnostic post-store receive snapshots are adjacent in
[unobserved_signals.go](../../internal/analyzers/concurrency/goroutineownership/testdata/src/goroutineownership/unobserved_signals.go).

Four ignored source overlays test the decision boundaries: restoring old
snapshot aliases revives the false diagnostic, ignoring the census allowance
fails partial-publication controls, and treating interrupted discovery as a
negative acceptance answer revives the diagnostic. Removing the cycle
restriction wrongly excludes a read that can observe a previous iteration's
store. Each fails its behavioral control rather than compilation. The initial
focused suites and `make verify` pass before the cyclic-read correction,
with tests in 71 seconds, dogfood in 40 seconds and zero lint issues in 14
seconds. No local race test or full precision replay runs.

Local-channel lookup charges its instruction/reaching-value traversal, but
`carries` still has independent heap-alias and stored-referrer work. This
correction does not certify all transitive goroutine queries as bounded.
The remaining once-cell families, concurrency field captures and cancellation
owner cells, stay in parent .25; heap/type/alias internals remain broader parent
scope. This is an executable local FP correction, not a removal credited
against the eleven recorded unresolved production sites.

The reviewed final `make verify` covers the cyclic-read restriction and passes:
ordinary tests 74 seconds, dogfood 32 seconds, zero lint issues in 12 seconds,
vet 2 seconds, formatter check 3 seconds and deadcode 5 seconds. The immutable
`.build/goal-channel-census-reviewed` binary is parent `0f5bf75` plus this final
production change, before later prose receipts. Its SHA-256 is
`8142f17e0ed0cea5e0982540d7729a084af9a9552f29c217f89a8bb19608bbcb`.
The parent control is `.build/goal-capture-timing-current`, SHA-256
`17de5c3e807259a7d76d24d94d2557b98ce5395c80bdaab9aa626aa2d1757de0`,
whose production source matches the preceding temporal correction.

At clean stargz pin `624678b4e421947534cbf0618f9609853cccee0f`, static all-check
`./store` scans with both binaries exit 3 with empty stderr and identical
827-byte JSON; the reviewed worker report at `store/manager.go:193:2` remains.
At clean Openase pin `e530faf137e764337d5beaaf68af3be159eb17aa`, the final
all-check `./internal/orchestrator` scan exits 3 with empty stderr and JSON
identical to the temporal-correction receipt, preserving both cancellation
true positives. The Openase run also enables goroutine tracing to a separate
file: all 2,494 JSONL events parse and the diagnostic JSON remains identical.
Candidates, labels, evidence, considered alternatives and decisions appear;
the synthetic cutoff control supplies the specific signal-census exhaustion
witness. Candidate tests, generators and applications are not executed.
These scoped receipts earn no removal against the eleven-site production
queue and do not replace a full-corpus census.

### Read-time concurrency spill paths (dho.44.11.5.25.4)

The actual SSA probe `.build/goal-concurrency-spill-timing-probe.log` shows a
nil load before `p = d` being named as parameter `d` by the old sole-store
shortcut. `ssaflow.WrittenOnceCellAtWithin` now shares the unique-store census
and instruction ordering at a caller-selected observation. Fixed-argument
binding chooses closure creation; concurrency spill paths choose the load.
These consumers share mechanics without a universal invocation policy.

Field-path folds, spill referrers, bounded storage equality, capture reads,
canonical load enumeration and nested capture forwarding spend the engine's
allowance. Captured-read cutoff returns no partial canonical read. An interrupted
canonical field search deletes its package-shared sentinel or negative result,
so a fresh allowance can retry. Complete positive identities remain reusable.
The declaration-relative fact schema and ordinary spill/reassignment bindings
are preserved. Publication still recomputes field metadata under its default
query; new dho.44.11.5.25.5 owns that separate allowance boundary. Package
write-once inventory construction, heap graph and type internals also retain
independent costs; this is not an all-query cost claim.

[spill_paths_budget_test.go](../../internal/passes/concurrencyfacts/spill_paths_budget_test.go)
records SSA for pre-store, post-store, entry-spill and reassigned snapshots,
checks every allowance boundary, and verifies canonical-cache child/fresh
recovery. Large read-only captures discard interrupted reads. A padded spill
isolates instruction-order cutoff and verifies that summary inference cannot
cache its truncated answer. Three ignored overlays fail behaviorally when
ordering is removed, spill searches ignore their allowance, or interrupted
canonical results are retained. An initial control selected an opaque callback
and then unmodeled printing before reaching the intended summary boundary;
inert scalar arithmetic isolates that query. Lint initially rejected the test's
nested setup; extracting the reusable cutoff check fixes the complexity gate.

The final `make verify` passes: tests 7 seconds, dogfood 2 seconds, zero lint
issues in 3 seconds, formatter check 1 second and deadcode 3 seconds. The initial
ordinary suite also passed in 79 seconds before the test-only lint refactor.
No local race test or full precision replay runs. The immutable binary
`.build/goal-concurrency-spill-current` is parent `48e2078` plus this production
change, SHA-256
`798eee66f06d0d4853fc61558391e67c34b8f7f1bbd311c054be920642a21ef8`.

At clean stargz pin `624678b4e421947534cbf0618f9609853cccee0f`, the all-check
static `./store` scan exits 3 with empty stderr and JSON identical to the
channel-census receipt, preserving the reviewed worker report. At clean Openase
pin `e530faf137e764337d5beaaf68af3be159eb17aa`, all checks on
`./internal/orchestrator` likewise exit 3 with empty stderr and identical JSON,
retaining both cancellation true positives. Candidate tests, generators and
applications are not executed. This local identity correction earns no removal
against the eleven recorded unresolved production sites. Parent .25 remains
active for cancellation owner cells and publication metadata; the broader
consolidation requirements remain unproven.

## Once-cell consumer family completion, October 2

The remaining cancellation constructor-owner consumer now uses one bounded
structured proof and shared store-before-capture evidence; its completed use
census also supplies exact owner returns. The
[cancellation owner follow-up](../../benchmarks/precision/audits/cancellation-owner-followup-2026-10-02.md)
records actual SSA, cutoff/fresh/flow controls and scoped production evidence.
The [publication allowance](../../benchmarks/precision/audits/concurrency-publication-followup-2026-10-02.md)
and [imported declaration isolation](../../benchmarks/precision/audits/concurrency-declaration-copy-followup-2026-10-02.md)
follow-ups complete the metadata boundaries found during the spill-path review.

A current source inventory of the once-cell APIs finds the remaining production
callers in fixed binding, concurrency spill paths, deferred result guards and
constructor owners passing their request allowance. The old default wrapper
had only test callers and was removed, preserving explicit nil-budget lookup.
The seven scoped children of `gohawk-dho.44.11.5.25` account for that family's
migration and follow-ups. This completion is limited to the recorded consumer
family: the broader transitive heap/type/flow/setup review and eleven unresolved
production FP sites remain open, and the overall consolidation is unproven.

## Lock-state traversal consolidation, October 2

The lock work list now shares one allowance across expansion, keys, detached
collections, phi/guard/cycle queries and nested result-backed branch/termination
queries while retaining its distinct-state cap. Function diagnostics and order
edges remain unpublished when any of that traversal is interrupted. Expansion
and transfer were extracted from the outlying flow file; successor selection
consumes the same state, and obsolete test-only default guard wrappers were
removed in favor of the authoritative `Within` engines.

The [lock-state allowance follow-up](../../benchmarks/precision/audits/lock-state-allowance-followup-2026-10-02.md)
records actual SSA cutoff/fresh controls, six failing counterfactuals and four
retained production lock TPs in pinned XD/goiardi package scopes. No FP removal
or corpus-wide claim is credited. Beads `gohawk-dho.44.11.5.26` covers this
traversal stage; `.27` retains setup, nested completion and final metadata costs.
Heap/type/graph internals remain separate. The eleven recorded production FP
sites, Rune publication issue and wider consolidation goal remain unproven.

## Lock release query ownership, October 2

The five release searches identified during the lock-flow review now share one
request owner in `release_queries.go`, with exact/possible coverage and launch
reasons retained. It charges the function traversal pool before cache access,
retains each question's cap, and records an interrupted question as unavailable
rather than as a release. Expansion and publication consume one interruption
barrier; a late completion cutoff discards earlier findings and order edges.
Scalar/go/defer instructions no longer enter the synchronous release search.

The [lock release follow-up](../../benchmarks/precision/audits/lock-release-queries-followup-2026-10-02.md)
records actual SSA coverage/cutoff/fresh/warm controls, four failing
counterfactuals, the passing local gate and four unchanged pinned production
lock TPs. Beads `.44.11.5.27.1` covers this release family; parent `.27` retains
setup, final metadata, callee ordering and alias/exclusivity boundaries. This
stage earns no production FP correction or whole-query time claim. The wider
consolidation requirements and remaining production findings remain unproven.

## Lock-function setup consolidation, October 2

One bounded setup inventory now supplies direct effects, helper effects,
acquisition eligibility, caller-owned first actions, defers and possible writer
witnesses. State transfer reuses its direct metadata. Helper inference retains
its shared summary cap while drawing from the function pool; interrupted setup
exposes no partial inventory, even after an earlier completed helper.

The [lock setup follow-up](../../benchmarks/precision/audits/lock-setup-inventory-followup-2026-10-02.md)
records actual SSA cutoff/fresh/warm and nested-summary controls, three failing
counterfactuals and four unchanged pinned production lock TPs. Beads
`gohawk-dho.44.11.5.27.2` covers this setup stage. Parent `.27` retains final
return/report/held-contract metadata, callee ordering and identity/alias/
exclusivity internals. This consolidation earns no production FP removal or
whole-query cost guarantee. The eleven recorded unresolved production sites,
Rune publication issue and wider completion requirements remain unproven.

## Lock return-contract consolidation, October 2

Held-success and conditional caller-release evidence now has one query owner in
`return_contracts.go`. The setup inventory supplies returns and branches;
contract metadata, caller coverage, private-lock uses and diagnostic acquisition
metadata share the function allowance. Final reporting reuses cached direct
effects and prepares each candidate's metadata once. A final-stage cutoff
discards all buffered function findings and staged order edges.

The [return-contract follow-up](../../benchmarks/precision/audits/lock-return-contracts-followup-2026-10-02.md)
records actual SSA, cold/fresh/padded controls, two failing counterfactuals,
the passing local gate and four unchanged pinned lock TPs. Beads
`gohawk-dho.44.11.5.27.3` covers these contract and reporting boundaries.
Return retention/merging, returned-owner inference, named-result storage,
identity/heap/type/alias and callee-order/package exclusivity remain distinct
work. No production FP correction or whole-query cost claim is credited.
The broader architecture audit and remaining easy-FP assessment remain open.

## Lock return-retention and callback-origin consolidation, October 2

Return retention, detached masks, possible/definite merges and returned unlock
owners now share the function allowance. Callback handoffs use the same bounded
capability projection of the lifecycle engine. Its wrapper/phi traversal moved
to the shared reaching fold, keeping one origin history per completion request
and the original memo invalidation on revisit. The legacy Boolean helper
delegates to that structured proof. Flow cutoff assertions across traversal,
final metadata and return retention share one test helper.

The [retention follow-up](../../benchmarks/precision/audits/lock-return-retention-followup-2026-10-02.md)
records the controls, repaired request-scope regression and scoped production
evidence for Beads `gohawk-dho.44.11.5.27.4`. Callee ordering, package
caller/exclusivity inventory, named-result storage and transitive identity/heap/
type/alias costs remain open. Rune's map reader passes the gate into a returned
iterator before later use; the existing confinement query does not establish
that guarded publication contract. It remains unresolved, as do the eleven
recorded production FP locations and the wider completion requirements.


## Completion callee resolution

Beads `gohawk-dho.44.11.5.27.5` moves the launch dispatch and exact callback
resolution out of the completion body engine into `completion_callees.go`.
Those queries previously used unbounded reaching and storage searches inside
an otherwise budgeted request. Dispatch, wrapper/phi origins, stable storage
and target materialization now share that request's allowance. Origin order,
all-alternative resolution and direct/OnceFunc/testing/WaitGroup contracts
remain unchanged; a cut supplies no partial target set. An early cutoff now
reports budget exhaustion before claiming any local-body provenance.

Actual SSA controls compare target identities/order at every cold cutoff and
fresh child, distinguish mixed and opaque callbacks, and cover stored or
reassigned function cells. A 41-target fixture isolates origin cutoff and
fresh target resolution; later body coverage has its own larger costs. This
step earns no FP correction and is not a whole-query time/memory bound. The
remaining identity/alias/heap/type, named-result and package inventory reviews
remain open, alongside the eleven recorded production FP sites and Rune.

The final canonical local gate passes. Four assertion-failing counterfactuals
pin origin charging, stable-storage charging, all-origin polarity and discarded
partial results. Clean pinned XD/goiardi scans retain the four reviewed lock
findings with byte-identical JSON; no production FP correction is credited.
Receipts and limitations are in
[the callee resolution follow-up](../../benchmarks/precision/audits/completion-callee-resolution-followup-2026-10-02.md).


## Named-result cell discovery

Beads `gohawk-dho.44.11.5.27.6` makes named-result recognition one all-return
cell census under `ssaflow.ProveNamedResultCellsWithin`; the obsolete
single-cell loop and test-only adapter are removed. Lifecycle discovery reuses one
completed map across captures and literals. Exact direct reads, first-slot
selection, every-SSA-return agreement and capture ordering remain separate
from cleanup coverage. Cutoff publishes neither a partial map nor guard list.

Actual SSA and five behavioral counterfactuals cover mismatched/missing return
reads, duplicate slots, shared allowance, positive prefix publication and
warm capture reuse. Synthetic result cells and recovery returns are preserved.
The initial gate identified the now-unreachable adapter; it is removed rather
than retained for fixtures. No production allowance is increased. This is discovery consolidation, not completion of
all result storage, identity/path/alias/heap/type or package inventory work.
The eleven recorded production FP locations and Rune remain open. See
[the named-result follow-up](../../benchmarks/precision/audits/named-result-census-followup-2026-10-02.md).

Final canonical local validation passes, including ordinary tests, self-dogfood,
lint and deadcode. Scoped pinned XD/goiardi scans retain all four reviewed lock
findings unchanged. Parent/current goiardi transaction scans also remain
byte-identical, including the two open transaction FPs; the other transaction
findings are not relabelled by this replay. No FP removal is credited.


## Storage-owner query and current duplicate scan

Beads `gohawk-dho.44.11.5.27.7` evaluates the successful argument's
`sameValueStorageOwner` once, reusing its returned owner in the mapping.
Exact storage/identity, callback and strict projection keep their priority;
owner mapping still precedes aggregate containment. The existing centrifuge
rationale and actual SSA once-written/reassigned/nested-owner controls remain.
This removes repeated proof work without extending ownership policy or claiming
the transitive identity/heap cost review complete. Graph tools were unavailable.

The same normalized complete-function scanner described above now examines
315 production files and 2,137 bodies under `internal/`, with seven candidate
groups (35-token minimum). Receipt:
`.build/goal-duplicate-functions-current-2026-10-02.json` at `ed3725b` plus the
owner-query change. Each group's source was inspected; normalization still
hides type, literal-identifier and policy differences.

| Current candidate | Disposition |
| --- | --- |
| Resource/obligation state keys | Different obligation state; shared guard key machinery, no generic domain-state merge. |
| Carried-value/captured-Body proof results | A found carry is Proven; guarded Body evidence is Unknown. Reasons and fallback meanings differ. |
| Spawn/resource classifier memo adapters | Different action/reason policies; each caches and traces its own authoritative classifier. |
| Field/global/enclosing-scope stores | Destination and may-alias/derivation policies differ; shared provenance is already underneath them. |
| Spawn/resource budget adapters | Different candidate limits and observers over one shared pool mechanic. |
| Resource-presence/stored-value constructors | Different payloads and evidence reasons; a path-presence proof is not a storage identity proof. |
| HEAD/local-header-only cutoff wrappers | The post-query budget projection is mechanically identical. New Beads `gohawk-dho.44.11.5.28` tracks sharing it while keeping the finders' policies separate. |

This is a refreshed bounded candidate inventory, not proof that differently
structured or partial duplication is absent. The confirmed acquisition wrapper
work, transitive identity/heap/alias/type review and eleven production FP sites
plus Rune remain open. No FP removal is credited by the owner-query change.

The focused existing SSA mapping controls pass in
`.build/goal-owner-once-focused.log`. Canonical `make verify` passes all local
targets in `.build/goal-owner-once-verify.log`, including ordinary tests and
self-dogfood. No new test duplicates the unchanged policy, no production FP
relabel is made, and no full precision replay or local race run is performed.


## Resource acquisition cutoff projection

Beads `gohawk-dho.44.11.5.28` consolidates the mechanically identical HEAD and
local-header-only post-query cutoff wrappers into `resourceProof.within`.
Each authoritative finder still runs first and retains its domain policy.
The projection replaces interrupted evidence with Unknown/budget-exhausted;
available state, reason and provenance pass through unchanged. No classifier,
acquisition policy, evidence allowance or fact schema changes.

Existing actual SSA HTTP allowance controls pass, including cold/fresh queries
and accepted/diagnostic boundaries (`.build/goal-resource-cutoff-focused.log`).
An ignored overlay returning the finder result without the projection fails
both HTTP families on their cutoff assertions, rather than compilation:
`.build/goal-resource-cutoff-overlay/result.log`. No new mirrored test is added.
The refreshed same-scope duplicate scan examines 315 files and 2,138 bodies and
retains exactly the six domain-distinct groups reviewed above; the wrapper
copy is gone (`.build/goal-resource-cutoff-duplicates.json`). This is still a
whole-function candidate review, not an absence claim about partial duplication.
No production FP removal is credited. The broader architecture/identity review
and eleven recorded production FP sites plus Rune remain open.

Canonical `make verify` passes all local targets, including lint, deadcode,
self-dogfood and ordinary tests (77 seconds):
`.build/goal-resource-cutoff-verify.log`. The focused existing controls and
counterfactual above verify the changed boundary; no broader production replay,
full precision-regression audit or local race run is needed for this unchanged
policy projection. The initial owner-query scan's confirmed wrapper work is now
closed; its other six candidate dispositions remain unchanged.

## Current precision reassessment and local pipe correction

The [eleven-site reassessment](../../benchmarks/precision/audits/remaining-eleven-reassessment-2026-10-02.md)
refreshes seven pinned scopes at `707913f`: eight sites visible, three absent
with exhausted evidence, all eleven still semantically unresolved in that
snapshot. The historical labels and replay ledgers remain unchanged.

The [local pipe follow-up](../../benchmarks/precision/audits/process-local-pipes-followup-2026-10-02.md)
corrects rev-dep at the existing unused-command boundary. One private contract
distinguishes local standard IO operations and their results from handing on a
pipe or command owner. Returned-pipe, partial-wait and Kill fixtures remain
diagnostic; two pinned production process TPs remain byte-identical. Actual SSA,
unknown-reason traces, two failing counterfactuals and final canonical validation
support the correction. This leaves ten unresolved production sites plus Rune;
budget-driven silence remains open. It does not complete the broader graph,
alias, type, package-inventory or fact-consumer consolidation reviews.

## Graph containment traversal

Beads `gohawk-dho.44.11.5.27.15` consolidates the identical bounded region
search in whole-build and point-in-time containment. Each query retains its
selected slot map, graph lock and availability gates; the shared private
search retains unknown/stale may-pointees, cycles and the existing depth limit.
Actual SSA tests distinguish later and overwritten storage from history and
pin nested/cyclic owners and both depth boundaries. Three counterfactuals fail
assertions. Final canonical verification and byte-identical resource/goroutine
fixtures and four pinned production lock controls pass.

The [containment review](../../benchmarks/precision/audits/graph-containment-consolidation-2026-10-02.md)
records neighboring query/effect dispositions and source scope. The normalized
complete-body scan still has five distinct-contract groups across 316 files and
2,146 functions; it did not expose this partial traversal duplication. Graph
construction, deferred observation costs, alias/type internals, cycle metadata,
package inventories and other fact consumers remain open. Ten production FP
sites plus Rune remain unresolved; no FP correction is credited by this refactor.

## Deferred-cell observation allowance

Beads `gohawk-dho.44.11.5.27.16` replaces the deferred graph relation's separate
RunDefers/Return censuses with one completed shared instruction census and
threads completion's existing allowance through reachability, occupant unions,
comparison and descendant history. A shortened census or relation is
unavailable, so no exact mapping or path fallback can consume its prefix.
The old default API is removed after wiring its sole production consumer;
nil-budget behavior and graph locks remain unchanged.

Actual SSA exact/cleared/replaced/merged/aggregate/registered-cell controls,
intermediate cutoffs, prefix discard, independent child recovery and lifecycle
mapping cutoff pass. Three counterfactuals fail assertions. Final canonical
validation and byte-identical 444 fixture diagnostics and four pinned production
lock controls pass. See the [deferred-cell follow-up](../../benchmarks/precision/audits/deferred-cell-allowance-followup-2026-10-02.md).
Graph build/replay, pointee internals, alias/type work, cycle metadata, package
inventories and other fact consumers retain separate review scope. Ten production
FP sites plus Rune remain open; this consolidation credits no FP correction or
whole-query cost bound.

## Call-cycle body inventories

Beads `gohawk-dho.44.11.5.27.17` shares immutable owning-package/direct-callee
inventories across reachability roots under the existing cycle-cache lock.
Root queues own copied storage; foreign bodies are rejected before discovery.
Static Go/Defer, generic wrapper/origin edges and cycle polarity remain unchanged.
Summary publication leaves this structural metadata intact.

Actual SSA and concurrent publication controls pass; three counterfactuals fail
assertions. Canonical verification passes all eight targets, and scoped scans
preserve all 444 fixture diagnostics and four pinned production controls
byte-for-byte. The [inventory review](../../benchmarks/precision/audits/call-cycle-inventory-consolidation-2026-10-02.md)
records receipts, retained metadata costs and source scope. Graph build/replay,
pointee/alias/type internals, package inventories and other fact consumers remain
open. The five complete-body candidate groups retain distinct contracts; partial
duplication is not ruled out. Ten production FP sites plus Rune remain unresolved.

## Lock package caller discovery

Beads `gohawk-dho.44.11.5.27.18` shares package instruction discovery between
conditional release and exclusive ownership. Their policies remain separate:
initialization and escaped private function operands belong to conditional
release; the exclusive view retains synchronous static sites outside
initialization. A conditional cutoff drops every conditional caller set while
exclusive discovery continues. No second body-scanning constructor remains.

Actual SSA controls and three assertion-failing counterfactuals pin scope,
escape, caller count and prefix discard. Canonical verification passes all eight
targets; 551 fixture diagnostics, four pinned production controls and the two
SkyWalking FP sites retain byte-identical output. The [caller inventory review](../../benchmarks/precision/audits/lock-caller-inventory-consolidation-2026-10-02.md)
records receipts and the remaining private-function escape question in
`gohawk-dho.44.11.5.27.19`. That follow-up must verify completeness before the
exclusive precondition can be certified. Broader graph/alias/type and fact
consumer reviews, ten production FP sites and Rune remain open.

## Exclusive caller completeness correction

Beads `gohawk-dho.44.11.5.27.19` reproduces and corrects the preceding review's
private-function escape gap. Fresh synchronous calls had allowed an exclusivity
answer despite shared Go/Defer or callback uses. Both preconditions now consume
one complete private caller set; the parallel exclusive view is removed.
Escaped, method, exported, empty and interrupted caller sets remain unknown,
and initialization callers must pass the same exact fresh-argument check.

Actual SSA, caller-count and cutoff controls, a minimized callback-cycle
diagnostic and the existing accepted direct-only initialization fixture pin the
boundary. Two counterfactuals fail assertions. The [completeness review](../../benchmarks/precision/audits/exclusive-caller-completeness-2026-10-02.md)
records canonical validation and twelve successful scoped scans: one expected
lock fixture diagnostic added, none lost, and resource/goroutine fixtures plus
production controls unchanged. The earlier two-view disposition is superseded.
Ten production FP sites and Rune remain unresolved; graph/alias/type and other
fact-consumer reviews still prevent an overall completion claim.

## Region reset and graph core dispositions

Beads `gohawk-dho.44.11.5.27.20` removes repeated allocation reset and
contents/backing deletion from opaque clobbering. One state operation forgets
stored subtrees; allocation/full overwrites separately clear stamps, while
opaque effects retain them. Escaped-loop bailout, prefix boundaries, children
collected before forgetting and may-only history remain unchanged.

The [graph core inventory](../../benchmarks/precision/audits/region-reset-consolidation-2026-10-02.md)
disposes construction/replay/state-join/copy/reset families by their contracts
and records validation scope. Actual SSA and three counterfactuals verify reset,
stamp and sibling boundaries. Scoped scans preserve all 552 fixture diagnostics
and pinned production output. A separate parent-failing nested backing path
probe identifies the next concrete correction, `gohawk-dho.44.11.5.27.21`.
Structural alias/type and remaining summary-consumer review stay open, as do ten
production FP sites and Rune. Independent cost boundaries are documented rather
than claimed as one request-owned time bound.

## Nested backing path correction

Beads `gohawk-dho.44.11.5.27.21` corrects the confirmed nested copy defect:
backingOf returns one canonical relative path, root lookup uses the same loop,
and copy propagation no longer repairs the suffix independently. Unchanged
copied fields retain exact identity while sibling, mutation and opaque-call
cases remain unproved; existing cycle/depth cutoffs remain unknown.

The [backing path review](../../benchmarks/precision/audits/nested-backing-paths-2026-10-02.md)
records four parent-failing SSA cases, ten current controls, full heapmodel
validation, a failing raw-path counterfactual and canonical/scoped receipts.
No production FP removal is credited. Structural alias/type and other
fact-consumer reviews, ten production FP sites and Rune remain open.

## By-value type traversal

Beads `gohawk-dho.44.11.5.27.22` shares the struct/array recursion used by
reference capability and by-value overwrite detection. Leaf predicates retain
their meanings, including underlying-struct identity and zero-length arrays.
Field-based synchronization detection and bounded slot enumeration stay separate.

The [type review](../../benchmarks/precision/audits/byvalue-type-consolidation-2026-10-02.md)
records thirteen compiled-type controls, two assertion-failing counterfactuals,
focused tests, all successful canonical target receipts (lint passes separately
after a process-lock collision) and twelve successful scoped comparisons.
All 552 fixture diagnostics and pinned production output remain byte-identical.
No production FP reduction is credited. The bounded alias/path/type inventory
identifies possible structural identity's independent recursion for follow-up
in `gohawk-dho.44.11.5.27.23`. Other fact-consumer review, ten production FP
sites and Rune still prevent overall completion.

## Possible structural identity traversal

Beads `gohawk-dho.44.11.5.27.23` replaces possible identity's private phi fan-out
and visited set with `ReachingWalk`. One shared Any implementation supports a
direct origin witness before expansion, preserving phi reflexivity. Wrapper
normalization, exact-address stores, field/index matching and the two search
directions retain the possible-identity policy; exact identity stays separate.

The [structural identity review](../../benchmarks/precision/audits/structural-identity-consolidation-2026-10-02.md)
records fifteen parent/current compiled-SSA controls, origin/cutoff tests,
6,067 agreeing differential comparisons in an extended compiled corpus, three
assertion-failing counterfactuals and successful canonical validation. All 552
fixture diagnostics and pinned production output remain byte-identical across
twelve successful scoped scans. No production FP reduction is credited.
`gohawk-dho.44.11.5.27.24` bounds the remaining summary-consumer review to a
candidate source inventory that still requires verification. The overall
architecture/easy-FP goal, ten production sites and Rune remain open.

## Concurrency binding and unused formal view

Beads `gohawk-dho.44.11.5.27.24.1` removes the broker's duplicate imported
declaration copy/binding path and delegates selected calls to the concurrency
engine. Dead-code and caller evidence identify the resulting unused formal
view; its declaration lookup/copy path is removed too. Imported cache stability
is tested through actual bound linear and alternative effects, while result and
lifecycle views keep their existing role.

The [binding review](../../benchmarks/precision/audits/concurrency-call-binding-2026-10-02.md)
records parent/current local/imported controls, intermediate cutoff checks,
three assertion-failing counterfactuals and final canonical validation. All 123
lock/capture fixture diagnostics and pinned production output remain
byte-identical across ten successful scans. No production FP reduction is
credited. The typed location ledger identifies 71 broker references in 28
analyzer files; semantic dispositions remain pending in parent .24. The overall
goal, ten production FP sites and Rune remain open.


## Finite summary-consumer dispositions

Beads `gohawk-dho.44.11.5.27.24.2` moves literal-first unconditional
result-outcome lookup into `summaries.Provider.OutcomeOf`, removing the two
cancellation/resource adapters. Literal knowledge survives unavailable
components; unknown calls, typed-nil boxing and existing allowances retain
their meanings. Guard retention and lifecycle decisions remain analyzer policy.

The [consumer review](../../benchmarks/precision/audits/summary-consumer-review-2026-10-02.md)
disposes all 71 typed references in 28 analyzer files across eight families.
It closes the finite consumer inventory, not every transitive proof or every
build configuration. Graph tools were unavailable; source/type information was
used. Independent cost boundaries remain explicit. No production FP reduction
is credited; ten production FP sites and Rune and the broader completion audit
remain open.

Focused compiled-SSA controls and three assertion-failing counterfactuals pin
the outcome boundary. Final canonical validation passes all eight targets;
four scoped comparisons preserve all 355 affected fixture diagnostics with
empty stderr. The final architecture check passes. This gives parent .24 a
finite disposition without expanding its claim to a global architecture proof.


## Duplicate read-lock reports across flow paths

Beads `gohawk-dho.4.5` corrects duplicate reporting when distinct branch states
reach one write. The proof and per-state tracing stay authoritative; proven
instruction/lock pairs are presented once per function. Unknown states do not
reserve a pair, different locks stay distinct, and the existing incomplete-walk
report barrier remains in place.

The [reporting review](../../benchmarks/precision/audits/readlock-path-reporting-2026-10-02.md)
records a parent-failing convergent fixture and six scoped comparisons. Existing
108 lock-fixture diagnostics remain byte-identical; Skywalking multiplicity
falls from four to two with identical distinct payloads. This is presentation
correction, not an FP removal: ten production sites and Rune remain unresolved.

Final canonical validation passes all eight targets, including ordinary tests
and self-dogfood; final architecture validation passes. No full precision
regression replay or local race run was used.


## Private entry reclamation and shared caller uses

Beads `gohawk-dho.4.6` corrects boxesandglue at the existing reclaim-only
resource boundary. One complete private-use collector now serves lockorder and
an explicitly bounded private entry-chain query. Operands/body/CFG searches use
the caller allowance; interrupted resource queries stay unknown. The entry query
requires unique synchronous, nonescaping, acyclic calls and a nonreferenced
language entry. Other analyzers retain their direct-entry policy; effectful
cleanup and unconditional callee facts remain unchanged.

The [private-entry review](../../benchmarks/precision/audits/private-entry-reclamation-2026-10-02.md)
records compiled SSA, cutoff recovery, depth boundaries, two assertion-failing
counterfactuals and parent/current production receipts. The accepted minimized
file acquisition and boxesandglue `pattern.go:79:14` disappear. All 426 existing
lock/resource fixture reports and two pinned goiardi lock findings remain
byte-identical; repeated/looped/escaped helper acquisitions, compressors and
transactions remain diagnostic. This credits one FP correction, leaving nine
recorded production sites plus Rune. The broader architecture completion audit
remains open; this is not a new corpus census.

Final canonical validation passes all eight targets and final architecture
validation passes. Ten scoped scan receipts exit zero with empty stderr. No
full precision regression replay or local race run was used.


## Closure choices and opaque worker participation

Beads `gohawk-dho.4.7` extends one possible-capture query across phi choices,
shared by the instruction classifier and existing worker census. Positive
capture evidence or interrupted discovery leaves ownership unknown; it never
proves a join, unique target, participant count or unconditional fact.
Conversions and loads retain their boundary.

The [closure-choice review](../../benchmarks/precision/audits/closure-choice-consumers-2026-10-02.md)
records compiled SSA controls, two assertion-failing counterfactuals and six
terminal scoped scan receipts. All 126 existing goroutine fixture findings stay
identical. The new fixture preserves unrelated-consumer and converted-callable
diagnostics. Two pinned Debian producers disappear with an explicit
`signal-consumed-by-worker` decision, rather than budget-driven silence.
This credits two FP corrections, leaving seven recorded production sites plus
Rune. The broader architecture completion audit remains open.

Final canonical validation passes all eight targets and final architecture
validation passes. No full precision replay or local race run was used.


## Read-lock writer release census

Beads `gohawk-dho.23.1` removes per-write/per-deferred-writer function discovery
from `possibleWriterAt`. The authoritative decision now consumes the completed
`lockFunctionSetup.calls` inventory already used by summary and writer setup.
Dominance, temporal reachability, mutex action and alias checks remain local
policy. No truncated inventory can reach the flow; this does not claim that
nested temporal or heap queries share one allowance.

The [writer census review](../../benchmarks/precision/audits/writer-release-census-2026-10-02.md)
records focused fixture validation, actual SSA for the explicit-release boundary,
canonical validation and parent/current fixture plus pinned Skywalking scans.
No production FP correction is credited. Seven recorded production sites plus
Rune and the broader architecture completion audit remain open.


## Deferred process wait allowance and discovery

Beads `gohawk-dho.23.2` identifies a process-specific bypass of the shared
bounded coverage API. Deferred closure waits now use one completed callback
instruction census and `ProveMethodCallCoverageWithin` under the candidate's
child allowance. Successful-Start assumptions, capture-before-argument priority,
independent Boolean guards and Process replacement retain their policies.
The focused `deferred_wait.go` file owns this evidence model; general transfer
and asynchronous handoff remain in `ownership.go`.

The [deferred waiter review](../../benchmarks/precision/audits/process-deferred-wait-allowance-2026-10-02.md)
records compiled SSA exact/conditional/guarded/replaced controls, intermediate
cutoffs, fresh child recovery and two assertion-failing counterfactuals.
Scoped parent/current process fixture and pinned production comparisons preserve
diagnostic payloads. No production FP correction is credited; seven recorded
production sites plus Rune and broader architecture completion remain open.

A current source reassessment of goiardi confirms `UsingDB` is the disjunction
of two mutable configuration fields. Carrying that caller condition into either
SQL helper requires stability through Begin and, for the stream helper, Get and
GetRun. Current private-call and unconditional summary helpers do not supply
that guarantee. This remains a caller-precondition question rather than an easy
name-based cleanup exception; no new production scan or correction is claimed.


## Process callback choices and structured handoff proof

Beads `gohawk-dho.23.3` verifies a process-specific gap using actual SSA:
a chosen capturing closure passed to an opaque runner, or dynamically launched,
was reported even though the existing literal handoff boundary is uncertain.
One shared reaching fold now traverses phi alternatives and requests bounded
possible containment. Conversions and loads stay opaque. The structured handoff
proof retains merged receiver and nonreturning worker policies; normal-return
and anywhere-completion queries now share its candidate allowance.

The [callback-choice review](../../benchmarks/precision/audits/process-callback-choices-2026-10-02.md)
records capture and cutoff controls, three assertion-failing counterfactuals and
eight scoped receipts. All 37 existing process findings and pinned production
outputs stay identical. The new fixture removes two opaque choice findings
while retaining unrelated, bypass and converted-callable diagnostics. Explicit
final decision tracing distinguishes opaque ownership from budget silence.
No recorded production FP correction is credited: seven sites plus Rune remain,
and broader architecture completion is unproven.


## Strict-dominator census consolidation

Beads `gohawk-dho.23.4` consolidates three process pre-Start block/prefix
loops, the external-store scan and the goroutine pre-spawn traversal into
`ssaflow.InstructionsStrictlyDominatingWithin`. It indexes the pivot once,
preserves function block order and excludes the pivot and its later same-block
instructions. Block checks, indexing and yielded visits share the supplied
allowance; the iterator does not decide ownership policy.

Process ownership materializes one completed prefix for its four distinct
policies, discarding any interrupted census before classification. Goroutine
ownership retains early positive witnesses and gives an interrupted negative
search an explicit unknown proof. Existing signal-census direct controls still
cover that query; its large main proof now cuts off at the earlier pre-spawn
stage. The existing cutoff observer projects the authoritative proof and
attributes `pre-spawn-census`, without changing trace-disabled behavior.

The [dominator review](../../benchmarks/precision/audits/strict-dominator-census-2026-10-02.md)
records actual SSA differential/allowance controls, three assertion-failing
counterfactuals, canonical validation and affected scoped parent/current output.
No recorded production FP correction is credited; seven sites plus Rune remain,
and overall architecture completion remains unproven. Watcher, binding, heap,
type and remaining pre/post-Start query costs stay explicitly outside this census.

## Process startup family ownership

Beads `gohawk-dho.23.5` reviews the remaining startup owner path together.
Registered owner arguments and result projections now share the completed
pre-Start census allowance; interrupted discovery publishes neither prefix.
Watcher bodies and possible containment use existing bounded APIs, retaining
literal-closure and source-position selection. Successful-return reachability
has a structured proof: only a completed negative establishes no normal return;
cutoff remains unknown. Startup evidence lives in one focused `prestart.go`;
ordinary post-Start handoff and completion stay in their existing files.

The [startup family review](../../benchmarks/precision/audits/process-startup-family-2026-10-02.md)
records multi-result owner, late/early/unrelated watcher, normal-return and
nonreturning branch SSA controls, all intermediate cuts and fresh recovery.
Three assertion-failing counterfactuals protect metadata charging, containment
charging and unknown reachability polarity. Scoped parent/current outputs and
canonical checks preserve their stated boundaries. The candidate budget comments
now describe request ownership without claiming one bound on independent heap,
type, symbol and other query costs. No production FP removal is credited;
seven recorded sites plus Rune and overall consolidation completion remain open.


## Post-Start process discovery

Beads `gohawk-dho.23.6` consolidates command-use discovery behind one structured
query consumed by the final reporting decision and its trace. Shared bounded
instruction, possible-reachability, reaching-value and stored-value APIs own
traversal. Existing loop-back-edge reachability, scalar and synchronous-pipe
boundaries remain analyzer policy. Returned process-owner discovery uses a
bounded complete body census and the existing returned-ownership proof instead
of charging only selected loads and starting unbounded containment.

The [post-Start review](../../benchmarks/precision/audits/process-poststart-discovery-2026-10-02.md)
records actual SSA cutoff/recovery, distinct handle/data and returned-owner
controls, scoped diagnostic comparisons and final validation. This is no claim
of one bound on every independently owned query. No production FP correction
is credited; seven recorded sites plus Rune and whole-goal completion remain
open.


## Private read-lock storage boundary

Beads `gohawk-dho.23.7` revisits the lock/field family and identifies a separate
reproducible gap: the parent reports fresh unpublished mutation destinations.
One destination selector now supplies both owner matching and existing heap
exclusivity. The proof excludes only a proven local destination, retaining
published/caller and borrowed-container diagnostics. Shared heap exclusivity
separates object selection from exact slot identity and fresh language
allocations from opaque producers. Its focused evidence lives in
`store_exclusivity.go`; standard mutex effects use the existing known-call and
exact summary-invalidation boundaries.

The [private-storage review](../../benchmarks/precision/audits/readlock-private-storage-2026-10-02.md)
records parent-failing accepted forms, shared heap controls, scoped output and
final gates. This removes a reproduced fixture FP class without claiming a
recorded corpus correction. Skywalking head/current writes still lack an exact
field/participant relation; seven production sites plus Rune remain unresolved.


## Deferred contracts and conditional atomic writes

Beads `gohawk-dho.23.8` addresses two reproduced shared heap errors. Exactly
registered deferred calls now use the direct-call contract dispatcher after
registration certainty checks. Atomic compare-and-swap retains old and possible
replacement contents through the existing weak-store mechanics, while Store
and Swap keep definite-write semantics. One store implementation supplies both
forms; atomic API identity and conditionality have one focused owner.

The [deferred-contract review](../../benchmarks/precision/audits/deferred-known-contracts-2026-10-03.md)
records actual SSA, publication, capture and uncertainty controls, assertion
counterfactuals, scoped diagnostics and validation. No recorded production FP
correction is credited; seven production sites plus Rune and overall completion
remain open.


## Builtin execution and snapshot invalidation

Beads `gohawk-dho.23.9` removes the deferred builtin bypass and prevents
asynchronous builtins from publishing synchronous storage evidence. Clear now
uses the existing selected-slot invalidation, preserving sibling and former
pointee storage. A reproduced aggregate-copy gap corrects whole-value cache
invalidation and carries unknown writes into later snapshots. Possible element
writes share ancestor whole-value invalidation. Backing identities
and unknown stamps share one metadata-copy mechanism; contents retain their
separate detachment/history contract.

The [builtin storage review](../../benchmarks/precision/audits/builtin-execution-storage-2026-10-03.md)
records parent failures, compiled SSA controls, counterfactuals, scoped output
and canonical validation. No recorded production FP removal is credited;
seven sites plus Rune and the overall consolidation goal remain open.


## SSA and summary selected-slot updates

Beads `gohawk-dho.23.10` consolidates the duplicated scalar replacement/union
paths in direct SSA stores and imported heap edges. Compiled summary callers
reproduce omitted ancestor cache invalidation for possible field/element writes.
One slot update now owns replacement, possible-content union, history, bounds
and enclosing aggregate invalidation. First wildcard writes also retain
unwritten nil/foreign possibilities rather than proving a value for an untouched
specific element; fresh/caller and known-index controls cover the boundary. Direct SSA exposure and aggregate-copy
policy, and imported escape/result binding policy, retain their own boundaries.

The [summary slot review](../../benchmarks/precision/audits/summary-slot-updates-2026-10-03.md)
records parent failures, exact/may/wildcard and snapshot controls, assertion
counterfactuals, scoped output and canonical validation. No recorded production
FP correction is credited; seven sites plus Rune and the broader consolidation
completion requirements remain open.


## Uncertain aggregate storage invalidation

Beads `gohawk-dho.23.11` reproduces stale exact fields after possible aggregate
writes: the unknown marker did not remove old concrete entries. One selected
stored-slot invalidation now forgets contents/backing, drops enclosing cache
entries and marks unknown. Local summary forgetting consumes that same operation;
foreign epoch and closure policies keep their own boundaries. Definite copies
and zeroing, prior snapshots, untouched siblings and former pointees are preserved.

The [aggregate invalidation review](../../benchmarks/precision/audits/aggregate-slot-invalidation-2026-10-03.md)
records parent SSA failures, seven controls, four assertion counterfactuals,
scoped diagnostics and canonical verification. No recorded production FP removal
is credited; seven sites plus Rune and the broader completion requirements remain
open.

## Producer operation count and source attribution

Beads `gohawk-dho.23.12` corrects a duplicated execution count in the producer
summary adapter. Equivalent branches yield one ordered concurrency operation
with alternate source positions. Expanding those positions into producer
records made the count proof treat one execution as several sends. Parent
actual-SSA fixtures reproduce balanced branch sends and first-send false
alerts. One producer record now retains all source positions, and its one
count proof feeds reporting and tracing at each position.

Balanced one/two-send workers, true excess second sends and distinct competing
workers pin the boundary. A parent overlay fails diagnostic and trace outcome
assertions. Complete all-check fixture payloads remove exactly six false
alerts, add none, and preserve all other findings. Canonical validation passes.
The [scoped audit](../../benchmarks/precision/audits/producer-branch-attribution-2026-10-03.md)
records the binaries and receipts. Seven recorded production sites plus Rune,
the remaining semantic/partial-duplication review and overall consolidation
completion remain open; this correction earns no production FP credit.

## Producer counts across alternative launches

Beads `gohawk-dho.23.13` widens the existing producer-count unknown boundary
when contributing launches do not form a dominance chain. Mutually exclusive
workers cannot be added into one total; even when each can reach a common later
worker, both alternatives need not execute. A latest-launch dominance frontier
checks the complete set through the shared instruction-order helper. Serial
and nested ordered workers retain their diagnostic controls. Trace fixture
assertions now share the source-position expectation helper.

Actual SSA and parent-failing controls reproduce five false alerts. Full
all-check fixture payloads remove precisely those five, preserve every other
finding and add none. The [scoped audit](../../benchmarks/precision/audits/producer-launch-order-2026-10-03.md)
records validation and the conservative loss of coverage for unordered workers.
This does not settle all producer protocol paths or the broader semantic and
partial-duplication review. Seven recorded production FP sites plus Rune remain
open; no production FP credit or full precision replay is claimed.

## Direct fallback producer send ordering

Beads `gohawk-dho.23.14` corrects counts within workers whose concurrency
summary is incomplete. Mutually exclusive direct sends can each reach a later
common send without both executing. Their total previously produced a false
alert at the common send and could inflate the total for a separate worker.

The count stage now returns a structured proof and uses one dominance-frontier
mechanic for launches and per-worker direct sends. Unordered contributions are
unknown at the existing count boundary; normalized complete summary operations
retain their sequence policy. Actual SSA, balanced branch/competition controls,
straight-line/nested diagnostic controls and parent-failing outcome assertions
pin the correction. Full all-check fixture payloads remove exactly three false
alerts, add none and preserve other findings. The
[scoped audit](../../benchmarks/precision/audits/producer-fallback-order-2026-10-03.md)
records validation and the deliberate false negatives for unordered fallback
protocols. Seven recorded production sites plus Rune and broader semantic and
partial-duplication completion remain open; no production FP credit or full
precision replay is claimed.

## Producer receiver count at identity cutoffs

Beads `gohawk-dho.23.15` closes a receiver-count availability leak. Binding a
complete helper summary can consume the allowance before channel identity is
resolved. The parent treated that identity cutoff as a known zero receive count,
which can inflate apparent excess production. The existing helper classifier
now returns `receiver-budget-exhausted` and discards partial counts after either
query stops. Binding orchestration delegates to one focused summary receive
counter rather than growing the top-level helper condition.

Cold/warm one/two-receive SSA controls enumerate allowances through completion;
parent controls fail and current cutoffs retain unknown, zero count and the
stable reason. Complete all-check fixture payloads remain identical at 21
findings. The [scoped audit](../../benchmarks/precision/audits/producer-receiver-budget-2026-10-03.md)
records validation. Non-budget identity ambiguity, indirect receivers, broader
semantic/partial-duplication review and seven recorded production sites plus
Rune remain outside this correction. No production FP credit or full precision
replay is claimed.

## Shared possible closure capture for opaque participants

Beads `gohawk-dho.23.17` removes a producer false alert when a phi selects either
of two callbacks that drain the channel. Incomplete summary classification had
looked only for a direct called closure. The existing process callback-capture
fold now lives in `lifecycle.ProvePossibleClosureCaptureWithin`, with shared
mixed/unrelated/opaque-wrapper and cutoff controls. A positive result proves
possible capture only. Process ownership maps it to unknown Wait participation;
producer classification maps it to unknown receiving, without counting a drain
or proving invocation. Both keep their local diagnostic policy.

Actual SSA and parent-failing diagnostic/trace assertions pin the correction.
Full producer fixture payloads remove exactly one false alert (23 to 22), while
all 40 process findings remain identical and no diagnostics are added. The
[scoped audit](../../benchmarks/precision/audits/producer-callback-capture-2026-10-03.md)
records validation and counterfactual polarity checks. The earlier complete
channel-phi hypothesis in `.23.16` was rejected by the engine before receiver
counting; no behavior was changed for that unsupported case. Seven recorded
production sites plus Rune and broader semantic/partial-duplication completion
remain open; no production FP credit or full precision replay is claimed.

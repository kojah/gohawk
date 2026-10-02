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
| Remaining easy FPs fixed | Frozen batches 62/63 supplied 55 production FP locations. The queue refresh, successful 22-site replay and four subsequent corrections left 18 unresolved production sites. The later opaque registry-owner correction removes two FDio sites, leaving 16 unresolved; the one-time entry-context correction in dho.37 removes one k8ssandra site, leaving 15. The ten-family assessment states the missing evidence for each. Rune callback wait is separately corrected; fresh-lock publication remains open. | No target-resolution-only correction has been demonstrated. The remaining families need policy, identity, state or protocol evidence. The current-helper reassessment found the FDio field-origin gap fixed in dho.31 and the one-time entry policy corrected in dho.37. Further source review must distinguish a newly available bounded fix from a genuinely larger model. |
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
| Possible helper cleanup | Disproven exact completion can still produce unknown through the older may-alias invocation or returned-deferred-cleanup query. | `ownership.go:pairedErrorHelperCleanup` requires an exact resource/error relationship and an anywhere cleanup witness; `classify.go:ambiguousHelperCleanup` requires merged argument provenance and proven cleanup of that argument. Neither establishes cleanup of the exact acquisition. |
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
| `resourcelifetime/captured_cleanup.go` | `opaqueClosureCall` combines possible captured-cell cleanup, narrowly guarded HTTP Body cleanup, aggregate capture, asynchronous invocation and unreadable/retaining callees. `cleanupRegisteredBefore` supplies the same uncertain cell/parent-cleanup boundaries before acquisition. A cleanup witness for a cell proves possible cleanup, never which stored acquisition was released. Deferred by-value arguments and read-only captures remain outside this cell contract. |
| Guarded captured HTTP cleanup | `guardedCapturedBodyCleanup` requires current stable content equal to this acquisition, and both caller/callee exposure checks must pass under one candidate allowance. `guardedBodyCoverage` uses Body-load identity and every-normal-return coverage with a nonnil assumption. Exhaustion cannot make that narrower positive witness succeed. This differs from cancellation's written-once cell/direct-defer ownership proof; a common broad capture traversal would erase the distinction. |
| Selected coverage adapter | `lifecycle/completion_search.go:MethodCallCoverage` delegates every-return coverage to the existing return/action witness and shared obligation walk, while anywhere coverage is an existential instruction witness. Captured-cell callers request anywhere coverage only for unknown classification; the guarded Body proof requests every-return coverage. These polarities remain explicit at their callers. |
| `resourcelifetime/optional_acquisition.go` | `proveOptionalAcquisition` requires one acyclic diamond, exact resource and paired-error phis, nil alternate edges and a repeated equality of the same operands. It pairs phi values with predecessor blocks through the shared adapter. Only the acquired merge successor is selected, and cleanup must target the exact resource phi through the selected transparent wrappers. Generic existential derivation and helper/edge completion remain excluded. |
| SQL parent/context classifier boundaries | `contracts.go:closesStatementDatabase/finishesRowsTransaction/cancelsTransactionContext` require known database/sql symbols and the exact receiver or paired context-constructor cancel. They yield uncertainty about parent-owned/asynchronous cleanup, never synchronous child release. `statementParentIdentity` uses shared storage identity at the loads; its nil budget selects NewStorage's bounded default, not an unbounded search. Resource SQL lifetime and cancellation-owner policy remain separate. |
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

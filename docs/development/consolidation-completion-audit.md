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
| Local observation | `localCallOnlyObserves` first asks `CallEffects.PreservesStorage`, then the cancellation-specific `cancellationUse` memo checks each exact argument/capture binding. Its conservative unavailable/cycle result is unresolved use. | `opaqueCall`, `aggregateOwnerMayEscape`, `wrapsResource` and `possiblyRetainedCallback` distinguish retain, asynchronous exposure and unknown effects at publication sites. A generic read-only test cannot replace cancellation invocation resolution or resource retention. |
| Capture storage | `result_guards.go` accepts a written-once cancel cell used only by directly deferred literals, with store-before-defer dominance. `owner_structs.go` separately restricts a captured cell to one visible owner field. | `captured_cleanup.go` handles current vs deferred cell observations, response Body guards and possible retention by unreadable callees. Broader mutation/exposure is unknown; the cancel cell restrictions must not be weakened by this resource policy. |
| Owner/destination identity | Fresh cancellation owners require visible field/return uses; parent context resolution observes storage at child creation. | `ownership.go:resourceExternalStorageProof` resolves an indirect destination at the store. `classify.go:carriesDirectly/carriesWithin` and Body handoff distinguish identity, projection and observation-time containment. These are separate questions, already supplied by storage/reaching helpers. |
| Wrapper traversal | Broad cancellation references allow ChangeInterface, ChangeType, Convert and MakeInterface for uncertainty; exact invocation does not inherit this broad relation. | `ownership.go:unwrapWrapper` peels only ChangeInterface, ChangeType and MakeInterface. Wrapper chains are bounded and count possible retention only at suitable publication boundaries. The differing Convert policy is deliberate; no universal unwrap is appropriate. |
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

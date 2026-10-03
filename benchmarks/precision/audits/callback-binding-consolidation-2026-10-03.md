# Callback agreement and fresh lock binding consolidation

Beads: `gohawk-dho.23.21.10` and `.23.21.11`. Parent source: `28a1c4c`.

## Shared callback agreement

Field and array-element callback resolution repeated the same observation-time
storage query and agreement with the previously selected exact SSA value.
`stableCallbackContent` now owns that rule. It returns the existing structured
stored-value proof; stable conflicting values yield unknown with
`EvidenceStoredValuesDiffer`, never a selected callback. Geometry remains in
the callers: exact field selection, fixed index selection, dynamic array
coverage, duplicate-index rejection and visible escape checks are unchanged.
Query order, observation instruction and the supplied allowance are unchanged.
The helper belongs to lifecycle callback binding, rather than making heap
storage decide which callbacks satisfy completion policy.

Existing actual-SSA controls cover direct/forwarded/captured, field/element,
dynamic agreed/mixed/hole, replaced, escaped, sliced and wrong-target callbacks.
`completion_callback_budget_test.go` additionally logs field, element and
dynamic SSA, sweeps every allowance through first completion, requires budget
unknown at each cutoff and checks fresh-child recovery without exhausting the
outer pool. Complete proof state, reason, method and provenance are retained.

## Fresh lock identity reuse

Binding already computed `mutexPathInstanceIdentity` for the selected caller
path, then repeated it through `localMutexPathIdentity` for fresh roots.
Instance resolution rejects loop allocations before supplying an identity;
for the same exact fresh root its identity also supplies the local class.
Binding reuses that result. The independent class-only caller retains
`localMutexPathIdentity`; it has no prior instance proof to reuse.

`binding_identity_test.go` logs actual SSA for an ordinary fresh helper argument
and an allocation repeated inside a loop. The ordinary binding keeps its
instance/local class; loop binding supplies neither identity nor class.
Existing constructor, publication, pointer-field, order and release fixtures
retain their policies. This does not infer exclusive ownership or safe
publication, change a fact schema, or move graph costs into the lock request.

## Validation

Full lifecycle and lock packages pass before adding the narrower boundary
controls; those controls also pass. Final `make verify VERIFY_TIMINGS=1` passes
all eight gates: generate 3s, modules 0s, vet 1s, formatting 2s, deadcode 6s,
lint 14s, dogfood 63s and ordinary tests 146s. No threshold or budget is raised.
The reviewed binary matches the canonical build:
`38fd7052a865d7307f1dbba06c6fb7cb745a62e2e1ba76019431c96e083ca580`.
Parent binary:
`f5b7076df7f42251620adb01a7c0020b8bb769614b4dbbadd7fecdda12e63fea`.

## Source review and limits

Graph/index tools were unavailable. Exact-source review retains the previous
scanner exclusions and limitations. The current scan covers 334 production
files and 2,202 declarations, five whole-body groups and 34 partial groups.
The prior callback agreement group is gone from that candidate inventory; its
actual policy was reviewed separately above. Other remaining groups retain
the distinct-contract dispositions recorded in the enum decode review.
These candidate counts are not a proof that differently structured semantic
duplication is absent across the repository.

No full precision replay, local race run or production FP credit is claimed.
Five production sites and parent requirement/evidence reconciliation remain
open. Final scoped output and trace receipts are recorded below.


## Final scoped compatibility

Sixteen parent/current read-only all-check invocations exit zero with empty
stderr. Complete diagnostic JSON matches in every scope: lock fixtures 116,
goroutine fixtures 128, cancellation fixtures 40, resource fixtures 318,
process fixtures 41, Rune 0, Skywalking 0 and stargz 1. Receipts retain package
scopes, clean pinned production revisions and binary hashes. Production modules
are read-only; Rune enables CGO and other scopes disable it. No external
repository tests, applications or generators were run.

Four targeted trace invocations compare `exclusive_owners.go` lock candidates
and callback resource candidates. All 102 lock and 474 resource records per
version preserve complete contents and ordered per-candidate sequences. Both
traced diagnostic payloads also match their corresponding untraced scans.
This establishes the selected boundaries, not every historical trace or cutoff
sequence. Artifacts use `.build/goal-callback-binding-*`.

Final architecture checks pass after the audit documentation. A source-signature
comparison verifies that only the former callback agreement group disappears
from the 35-group partial inventory; all remaining group signatures match the
prior review. This does not expand the scanner's semantic coverage.

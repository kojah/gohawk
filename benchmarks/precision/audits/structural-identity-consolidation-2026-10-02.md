# Structural identity consolidation — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.23`. Parent: `49ac33d`.

## One reaching traversal

The possible-identity fallback repeated phi fan-out and maintained its own
visited set. `StructurallySame` now uses the shared reaching fold for wrappers,
phi alternatives and cycle handling. Its leaf policy retains matching fields,
indexes, loads and exact-address stores. Index identity still gets a separate
bidirectional search, and the two top-level directions retain separate walks.
Target wrapper normalization preserves sibling directional channel conversions;
the existing pinned rationale link stays beside that decision.

The new `ReachingWalk.AnyIncludingOrigin` shares the ordinary Any implementation
and accepts a direct witness before expansion or a revisit, after charging the
visit. This preserves identity of a phi with itself. Without an origin witness,
the fold follows the existing Any policy. Ordinary Any, Every and resolution
keep their contracts. Possible identity stays independent of store order; it
does not promote containment, arbitrary call operands, or an uncertain alias
to exact identity or cleanup evidence. There is no new fact vocabulary, graph
algorithm, budget increase or analyzer policy.

Graph tools were unavailable; the task used source fallback. The reviewed scope
is `ssaflow/value_matching.go`, `value_reaching.go`, structural/exact identity
and derivation adapters in ssaflow/heapmodel, and the compiled boundary controls.
The new primitive owns traversal mechanics; the leaf remains the possible-
identity policy. Other fact consumers and whole-query cost claims are outside
this verification scope.

## Boundary evidence

Fifteen permanent compiled-SSA controls cover direct/unrelated values, interface
boxing, directional channel wrappers, phi alternatives, related/unrelated
cycles, same/distinct fields and indexes, loads, captured-cell stores, aggregate
containment and opaque call results. Both directions and direct reflexivity are
checked. They pass on the parent before the implementation change and on current.
Additional origin tests cover a direct phi witness under one visit, exhausted
allowance, revisits, fallback alternatives and nil. Focused ssaflow, heapmodel
and lifecycle suites pass in `.build/goal-structural-same-focused.log`.

An ignored overlay retains the parent's implementation for differential checks
over parameters, SSA instruction values and operands in each compiled function.
All 2,926 comparisons in the permanent fifteen-function corpus agree; receipt
`.build/goal-structural-same-differential/results.log`. The old implementation
is an ignored review artifact, not a permanent parallel proof or test oracle.
An extended compiled corpus adds directional wrappers in loops, boxed loop
values and captured-cell loop loads. All 6,067 comparisons agree in
`.build/goal-structural-same-differential/extended.log`; it remains an ignored
bounded review aid rather than a completeness claim.

Three ignored counterfactuals remove the origin witness, target-wrapper
normalization or phi alternatives. All fail assertions without compilation
failure in `.build/goal-structural-same-mutants/`. The normalized complete-body
scan covers 319 production files and 2,152 functions, retaining five previously
dispositioned distinct-contract groups. It cannot prove partial duplication
absent; this task removes the specific independent phi/visited-set mechanics.

Canonical validation passes all eight local targets in
`.build/goal-structural-same-verify.log`, including ordinary tests, lint and
self-dogfood. Generated helper documentation includes the new origin fold.
Final architecture checks pass in
`.build/goal-structural-same-architecture-final.log`.

## Scoped compatibility and limits

Parent `.build/goal-byvalue-types-reviewed`, SHA-256
`266f559a0b213594e615fa99235723dc56131a9c4e6fb2e9f7a28e55b0f63263`.
Current `.build/goal-structural-same-reviewed`, SHA-256
`f3d921ca7141ce9e81124ddddb6fe43e64af86421f7ac0efa28f35c6782cb4c1`.

The [comparison ledger](structural-identity-consolidation-2026-10-02.tsv) records
six parent/current scopes. All twelve scans exit zero with empty stderr; each
receipt names the final binary hash in `.build/goal-structural-same-final/`.
All 552 fixture diagnostics, four pinned XD/goiardi production controls and both
duplicated SkyWalking FP sites retain byte-identical JSON. A confirmed current
lock-fixture replay in `.build/goal-structural-same-confirmed-lock/` also exits
zero with empty stderr and the same output. Production scans use readonly
modules, disabled CGO/workspaces, 180-second timeouts and at most two workers.
No full precision-regression replay or local race run is performed.

No production FP correction is claimed. Ten production sites plus Rune and the
broader completion review remain open. Beads `gohawk-dho.44.11.5.27.24` tracks
the remaining summary-consumer review from a finite candidate-path inventory;
selector-name matches still need source verification before completeness claims.

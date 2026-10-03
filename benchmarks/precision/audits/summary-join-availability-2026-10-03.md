# Summary join request availability

Beads: `gohawk-dho.44.13`. Parent source: `760649d`.

The instruction classifier's summary join query had a standalone 1,000-visit
budget and returned only a Boolean after an interrupted request. It now draws
the same child cap from the candidate's observed pool. One structured helper
result returns exact join, no applicable effect, or unknown with the stable
`summary-join-budget-exhausted` reason. The classifier consumes that result
before source-visible helper fallback. Tracked selection, returned-group
completion, ordered effect binding and exact storage identity share the query.

Exact joins still require an already-established completion obligation,
synchronous receive/Wait and exact resource identity. Owner lifecycle calls,
cancellation effects and launched observers do not become joins. Unknown is
attached to the affected instruction rather than to all exits. No budget
constant, fact schema, published guarantee, API name heuristic or diagnostic
family changes. The instruction memo retains the authoritative classification.

Actual SSA tests cover 32 arithmetic steps before a receive with outer limits
0, 1 and 32, then fresh exact completion. A cached summary still cannot bypass
zero candidate allowance. A helper larger than the child cap produces unknown
with a live outer pool; a larger independent engine query completes it and the
next candidate request recovers an exact cached join. Candidate-position and
summary-join trace attribution are asserted. Existing imported/returned waiter,
conditional/different-handle, asynchronous and cancellation controls pass.

The standalone-budget counterfactual compiles and fails the new assertion at
zero outer allowance; it reports an exact join without budget admission.
Receipt: `.build/goal-summary-join-mutant/test.log`. Focused receipt:
`.build/goal-summary-join-focused.log`. Initial lint found a copied test trace
assertion; helper and summary tests now share one candidate-cutoff assertion
with explicit event predicates. No string enum parameter was added.

Six read-only all-check parent/current scans exit zero with empty stderr and
identical complete payloads: goroutine fixtures 128, pinned Openase hook 0,
pinned stargz store 1. Pins, scopes, exits and hashes are in
`.build/goal-summary-join-scans/scans.json`; comparisons are in
`comparison.json`. Openase silence still receives no FP-correction credit.
Reviewed SHA-256:
`765fcd5eec88ead4cadcdb0defe359ea6161e9ecf2a4c723962f4d2ffe143392`.
It matches the canonical binary; these are precommit artifacts.

The normalized production scan retains 336 files, 2,208 declarations, five
full-body and 33 partial groups. All path/name/token candidate signatures match
the parent. The source inventory in the completion ledger distinguishes exact
identity from historical containment and selected-arm evidence from whole-call
coverage. Graph tools remain unavailable; exact source fallback was used.
The remaining guarded/count/caller/lifecycle inventory is still open.

No full precision-regression replay, local race run or production FP credit.
Five reviewed FP sites and broader semantic consolidation remain open.

Final canonical `make verify VERIFY_TIMINGS=1` passes all eight targets:
generation, module verification, vet, formatting, lint, deadcode, self-dogfood
and ordinary tests. Receipt: `.build/goal-summary-join-verify.log`.
Final architecture validation after maintained documentation updates passes;
receipt: `.build/goal-summary-join-architecture.log`.

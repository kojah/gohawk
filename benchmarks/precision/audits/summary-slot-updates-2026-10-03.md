# Shared SSA and summary slot updates

Beads: `gohawk-dho.23.10`. Parent: `2a8760a`.

## Reproduced divergence and correction

`heapSubstitution.apply` and `storeValue` independently replaced or unioned
selected slot contents. Possible summary edges omitted invalidation of cached
enclosing aggregates. Actual compiled callers assigning a whole holder, saving
an earlier copy, then invoking a summarized helper reproduce stale exact
contents for possible field, element and wildcard writes. A definite field
replacement is the retained positive control.

One `storeSlot` in `store_regions_writes.go` now owns selected-slot replacement,
possible-content union, history, bounds and enclosing aggregate invalidation.
Wildcard elements use one weak-element update rather than creating another
storage rule. Additional actual SSA controls reproduce an inherited error:
unknown-index writes to fresh or caller-supplied arrays could establish element
zero as the replacement, even when untouched. A first wildcard update now unions
existing elements and unwritten nil/foreign possibilities before the replacement.
Known-index strong writes remain exact. The gap is recorded in
`.build/goal-summary-writes-wildcard-parent.log`; final controls are in
`store_wildcard_writes_test.go`. Summary Must requires an exact destination before
replacement; even a Must wildcard edge retains selection uncertainty.

SSA foreign-write invalidation, exposure, and aggregate-value copies stay in
their current orchestration. Summary binding, imported escape metadata, result
collection and unknown-target handling stay with substitution. An imported
may edge is not upgraded to definite cleanup or ownership. This consolidates
shared storage mechanics without merging those distinct evidence policies.

## Behavioral evidence

`store_summary_writes_test.go` compiles five callers and applies registered
callee summaries. Possible field/element/wildcard updates retain prior and
replacement possibilities without claiming exact content. Definite field
replacement remains exact. A Must wildcard remains possible. Earlier snapshots
and unchanged sibling fields stay exact. Possible containment of a historical
aggregate object may remain even after a definite replacement, so the control
requires exact replacement and positive possible contents rather than an
unsupported negative may-answer.

Parent failures are in `.build/goal-summary-writes-parent.log`; whole heapmodel
controls and final focused controls are in `focused.log` and `final-controls.log`
under that prefix. `.build/goal-summary-writes-complete-ssa.log` records the final actual IR and
storage dump, including wildcard/fixed-index controls. Whole heapmodel after the
wildcard correction passes in `.build/goal-summary-writes-complete-heap.log`.
Four isolated source overlays fail behavioral assertions: remove enclosing
aggregate invalidation, force possible summary edges definite, and clear the
whole object on a selected replacement, and discard unwritten possibilities
on a wildcard update. All four final-source overlays are terminal exit 1 with assertion failures,
recorded in `.build/goal-summary-writes-complete-mutants/results.json` and logs.
Production source was not temporarily mutated for these counterfactuals.

## Scoped comparison

Parent executable: `.build/goal-builtin-complete-reviewed`, SHA-256
`6a0d5b3bef10c276eaa136642fd9f99a041a90a529bc0f34c737b116272bb9ae`.
Its six successful current receipts are reused as parents with unchanged scopes
and repository pin. The earlier current executable and six comparisons are prototype evidence
before the wildcard correction. Final executable is `.build/goal-summary-writes-complete-reviewed`, SHA-256
`141f09899d6d345235e599f56542f35b5a6fe1ec9863e3f068c206d24b9f2d4d`.
Refreshed receipts are recorded below.

All twelve final parent/current receipts are terminal exit 0 with empty
stderr. Complete merged diagnostic payloads are identical: lock/order/path 117,
private-read-lock 6, resource 318, process 40, goroutine 128, and Skywalking 2.
Skywalking is a clean checkout pinned at
`e83d5925500a7e63dd55c080a9b1542d6cedaefb`, scope `./pkg/tools/buffer`.
The companion TSV records terminal statuses/counts/pin; detailed scopes and
binary receipts live in `.build/goal-summary-writes-complete/scans.json`, with
complete payload equality in `comparison.json`. All checks are enabled. No
candidate application, generator or test execution was performed.

## Validation and remaining scope

Stable final `make verify VERIFY_TIMINGS=1` is terminal exit 0, including
all canonical gates and the full ordinary test suite (86 seconds), recorded in
`.build/goal-summary-writes-complete-verify.log`. Final architecture is terminal
exit 0 in `.build/goal-summary-writes-complete-architecture.log`. No full
precision corpus replay or local race run was performed. No recorded production
FP correction is credited: seven sites plus Rune remain unresolved, and broader
architecture completion remains unproven. Graph MCP tools are unavailable;
discovery used bounded exact-source fallback. The inspected pair demonstrates
this duplication, not an exhaustive absence claim about the repository.

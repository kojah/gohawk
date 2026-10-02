# Cancellation constructor-owner availability

Beads `gohawk-dho.44.11.5.25.7` completes the remaining constructor-owner
once-cell consumer review. Its closure rule previously combined the default
once-store query with a separate default dominance query and raw referrer
censuses. It now uses shared `WrittenOnceCellAtWithin` at closure creation.
Cancel, cell, closure, owner and sibling-field work share one classifier request
child and its candidate-wide pool. A structured owner proof preserves complete
rejection separately from cutoff, discarding all partial holds when unavailable.

The authoritative owner-use census also marks exact owner returns; return
classification reuses it rather than rescanning result vectors. One classifier
retains its own stopping reason without repeating the interrupted search.
A fresh cancellation proof creates a new classifier. Cutoff supplies
`owner-evidence-unavailable` unknown labels; direct cleanup, guarded cleanup and
returning the exact cancel retain their existing precedence. No unavailable
owner evidence becomes lost cleanup or a callee guarantee.

Twelve actual SSA scopes cover direct fields, read-only single and nested
closures, store timing, rewritten cells, reads, extra captures, opaque owners,
two fields, field loads, unrelated captures and plain sibling-field access.
Every cutoff is paired with child/parent/fresh controls. Padded capture bodies,
initialization order and owner-field censuses exceed a child allowance and
recover with a fresh allowance. Flow controls retain the dropped-owner violation
when complete, keep it unknown at cutoff, and honor exact cleanup in both cases.

The source inventory found the old unbounded `WrittenOnceCell` wrapper used
only by tests after migration. It was removed; tests select the unchanged
nil-budget identity query through `WrittenOnceCellWithin`. Its documented
identity-after-store contract remains separate from observation-time evidence.
The scoped once-cell family is now migrated; this does not certify all heap,
type, identity, flow or package-setup costs covered by the broader parent work.
Graph tools were unavailable, so inspection used scoped source fallback.

This is not a demonstrated correction of a recorded production FP. The queue
remains 11 unresolved sites, and frozen audit labels remain unchanged. No full
precision-regression replay or local race test is part of this iteration.

## Validation

Focused owner tests and the shared SSA/lifecycle/concurrency plus cancellation
package suites pass (`.build/goal-cancel-owner-focused-final.log` and
`.build/goal-cancel-owner-packages.log`). Three source overlays fail at the
intended boundaries: removing initialization timing accepts `beforeStore`,
unbounding owner and sibling-field censuses completes the padded field scan,
and removing cutoff invalidation retains a complete rejection at allowance zero.
All exit 1, with no checkout edits. Receipts:
`.build/goal-cancel-owner-{timing,fields,discard}-counterfactual.log`.

The immutable binary `.build/goal-cancel-owner-reviewed` implements parent
`f224de0` plus these production changes; SHA-256:
`fa000fc9e57d70041a127cf8001e43912f568203fdf1e8fa44fea5288758dadd`.
At clean Openase pin `e530faf137e764337d5beaaf68af3be159eb17aa`, the direct
`-enable-all -json ./internal/orchestrator` run uses `CGO_ENABLED=0`,
`GOFLAGS=-mod=readonly`, and `GOWORK=off`, with cancellation tracing to a separate
file. It exits 3 with empty stderr and byte-identical JSON to the publication
parent control. Both reviewed cancellation TPs remain at `runtime_launcher.go:414:22`
and `runtime_process_lifecycle_slice.go:192:22`. All 8,459 trace events parse;
no owner-unavailable label occurs in this scoped production control. Cutoff
labeling is covered by the focused classifier and flow tests instead. Receipts:
`.build/goal-cancel-owner-openase.{json,err,trace.jsonl}`. No candidate tests,
generators or applications ran.

The final canonical local gate passes (`make verify VERIFY_TIMINGS=1`,
`.build/goal-cancel-owner-verify-reviewed.log`): generation, module verification,
vet, formatting, deadcode, lint, local dogfood and ordinary tests. The initial
gate passed ordinary tests and dogfood but rejected one long test line; that
line was split before the successful final gate.

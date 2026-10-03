# Process deferred binding and family reconciliation

Beads: `gohawk-dho.23.21.13` and `.13.1`. Parent: `3c27597e`.

Deferred closure discovery previously built default capture/argument slices,
then charged each selected binding in analyzer-local loops. It now uses
`ClosureBindingPairsWithin` and `CallBindingsWithin` directly under the live
query allowance. Capture-first ordering and per-visit charges remain intact.
Explicit availability checks stop argument fallback and final disproof after
iterator cutoff. There is no new wait guarantee, budget or fact schema.

The actual SSA regression has 65 arguments and one unrelated lexical capture.
Its cutoff allowance completes the body/capture census, then admits only 32
arguments; the query returns unknown while the parent pool stays available.
A fresh child proves the supplied-command Wait. Parent and current probes
both complete at 86 visits. This is eager-allocation removal and shared query
mechanics, with no claimed behavioral counterfactual or FP correction.
The initial test incorrectly expected a new cutoff at 86; it failed and was
corrected after measuring both implementations. Existing exact, conditional,
guarded and replaced wait controls remain.

Source fallback reviewed startup registration/caller/aggregate selection,
successful merges, immediate Process guards, deferred Wait, possible callback
handoff, lower-level handles and returned owners. Their evidence polarity,
observation time and final disposition remain distinct as recorded in the
maintained consolidation inventory. Graph tools were unavailable. Independent
heap/type/branch costs are not claimed to share a whole-candidate time bound.

Full process package tests pass (15.115s); the corrected mid-argument test also
passes. Receipts: `.build/goal-deferred-binding-focused.log` and
`.build/goal-deferred-binding-metadata-final.log`.
Final canonical validation and final architecture checks use
`.build/goal-deferred-binding-verify-completed.log` and
`.build/goal-deferred-binding-architecture.log`.

Twelve read-only all-check scans exit zero with empty stderr. Complete
parent/current payloads agree for process fixtures (41), entry fixtures (2),
pinned Ferro (1), rev-dep (0), of-watchdog (1) and diff (1): 46 diagnostics.
Both reviewed process TP controls persist. Ferro's receiver-state model gap
remains; budget silence receives no correction credit. Pins, scopes, exits and
hashes: `.build/goal-deferred-binding-scans/scans.json`; complete comparisons:
`.build/goal-deferred-binding-scans/comparison.json`.

Reviewed binary SHA-256:
`266f42e0b2cec606009d32c6ee0a0675125aae6ef3f42a0c575e2b5eb21e04b2`.
It matches the canonical binary. The normalized scanner covers 340 production
files and 2,212 declarations; its five full-body and 33 partial candidate
signatures are unchanged from the parent. This supplies bounded inventory
support rather than semantic duplication absence for the full repository.

No full precision replay or local race run. This closes the finite process
family review after final publication; broader consolidation and five reviewed
production FP sites remain open.

# Process startup family consolidation

Beads `gohawk-dho.23.5`; parent source `71c31a8`.

## Evidence ownership

`prestart.go` owns startup discovery, wrapper cleanup/watcher evidence and
successful-Start return queries. Post-Start handoff and completion stay in their
existing files. Registered-owner instruction, argument and returned projection
visits share the completed strict-dominator census allowance. An interrupted
inventory publishes neither instructions nor owners. The static-callee and
possible-alias selection and tuple projection policy are retained.

Later watchers use existing bounded instruction and containment queries. Only
literal closures launched after Start in source position are candidates, as
before. Capturing a wrapper provides possible supervision, not an exact Wait
or unconditional cleanup guarantee. Unknown containment remains unknown.

Successful-Start reachability uses a structured normal-return proof. A proven
return disproves no-return; a completed absence of normal return proves it.
Unknown retains its state and reason. The reporting boundary suppresses an
interrupted candidate and traces its budget reason rather than inventing a
no-return guarantee. Existing success-branch selection and termination policy
are retained. No new fact, public option or diagnostic message is added.

The candidate pool bounds its participating requests. Independent heap, type,
symbol and other query internals keep their own costs; this does not establish
a whole-program runtime bound or a performance percentage.

## Controls and validation

Actual compiled SSA controls exercise a factory call with two returned
projections (three owner candidates), every intermediate census cutoff, empty
publication on cutoff and fresh child recovery. Watcher controls distinguish
late capture, an earlier launch and an unrelated worker, and require extra
containment work to consume allowance. Returning, looping and panicking success
branches pin return polarity; every interrupted query retains unknown and
permits a fresh successful query without exhausting the parent pool.

Three isolated source-overlay counterfactuals fail behavioral assertions:
uncharged owner metadata, unbounded watcher containment, and unknown
reachability treated as no-return. Their receipts precede two mechanical
switch/assertion style fixes; the final compiled SSA controls pass again.

The [ledger](process-startup-family-2026-10-02.tsv) records completed scoped
parent/current all-check scans and full diagnostic payload comparisons. Parent
receipts are reused from the completed earlier scan; current receipts are
refreshed from the final immutable binary. Process/choice fixtures retain 40
findings. Pinned clean Ferro `./mcp` retains one unrelated finding; its recorded
receiver-state target remains historically budget-silent and receives no FP
credit. Pinned clean rev-dep `./internal/telemetry` remains silent.

Parent SHA-256:
`f4aaa34926a705750e7a908497b3697f715f748a45cd6c99934275677986d940`.
Final SHA-256:
`a8ae71c4f8849f4ae2a4723e38cbab40432d2d020f4b21fbe500e72c924212a3`.

Receipts: `.build/goal-process-prestart-family-focused.log`,
`.build/goal-process-prestart-family-controls-complete.log`,
`.build/goal-process-prestart-family-mutants/results.json`,
`.build/goal-process-prestart-family-complete/scans.json`,
`.build/goal-process-prestart-family-verify-complete.log` and
`.build/goal-process-prestart-family-architecture-complete.log`.
Initial canonical runs caught mechanical lint requirements for the assertion,
tagged switch and explicit unknown case. Final canonical validation passes all
eight targets; the final architecture check passes.
Graph tools were unavailable; exact source fallback and compiled SSA controls
supply the evidence. No full precision replay or local race run was used.

No recorded production FP removal is credited. Seven production sites plus
Rune and the broader architecture completion audit remain open.

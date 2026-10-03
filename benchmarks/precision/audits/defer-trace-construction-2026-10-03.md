# Disabled defer lifetime trace construction

Bead: `gohawk-dho.23.29`. Parent: `184f449`.

Exact-source review of defer-loop obligation discovery, classification and final
flow found four trace presentation sites constructing detail maps and formatting
SSA values before the tracer could decline them. Candidate presentation,
iterator transitions, classified instruction evidence and argument evidence now
check the candidate probe before constructing those steps. Final decision
presentation already had that guard.

Alias, containment, acquisition and lifetime queries remain outside trace-only
guards. Classification order, state transitions, proof reasons and diagnostics
retain their existing semantics. This is not a precision correction or a measured
wall-clock speedup. No new regression test mirrors the guards: existing analyzer
trace fixtures cover accepted, unknown and rejected backedges, while complete
parent/current enabled trace comparison checks the actual event contents.

The all-check fixture scope is `deferinloop iteratorowner` under the analyzer's
GOPATH testdata tree, with CGO disabled and modules/workspaces disabled. Parent
and current complete merged diagnostic payloads preserve 21 findings. Final
traced invocations are compared with both untraced payloads and with each other.
Immutable binary and terminal scan receipts are stored in
`.build/goal-defer-trace-scans/`. Canonical validation and final architecture
results are recorded in the Bead. Final `make verify VERIFY_TIMINGS=1` passes
all eight gates. The reviewed and canonical binaries share SHA-256
`3a56b5986cb8815ce37bdd13553ce1019798a6d931b08bdc99624a7fb2abf23a`.
Both untraced scope receipts and both traced receipts exit zero with empty
stderr. All 1,280 trace events, including dependency candidates, are identical
as a multiset; ordered event sequences within every candidate also agree.
Global inter-candidate ordering differs and is not treated as a proof sequence.
Both complete traced merged diagnostic payloads match their untraced versions.

The bounded source review distinguishes this loop proof from return-obligation
flow: it asks whether an acquired value stays live to a dominating backedge;
normal returns run its deferred cleanup. Its response-owner adapter and iterator
uncertainty remain analyzer policy. Shared traversal, identity, ownership and
feasibility queries are already used. Transitive helper implementations and
other analyzer families remain outside this review; no exhaustive semantic
absence claim is made. Graph and coverage tools were unavailable, so the review
used exact source fallback. The five production residual sites remain unresolved.
No full precision replay or local race run is part of this iteration.

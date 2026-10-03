# Process pre-Start decision consolidation

Bead: `gohawk-dho.23.28`. Parent: `2a2df09`.

The pre-Start Boolean dispatcher in the analyzer entry point is replaced by
`proveProcessStart`, beside its existing startup evidence queries. The ordered
helper-origin, caller-storage, aggregate-element, registration and return-path
policies retain their existing semantics. One structured decision drives entry
suppression and the final trace adapter also used after Start. A local obligation
continues into flow analysis without emitting a premature final decision.

Possible external ownership and registration are unknown, not Wait guarantees.
Only a proven absence of successful normal returns yields an accepted pre-Start
decision. Exhausted and unavailable discovery remain unknown. Existing rationale
comments and source links move with the policy.

Compiled SSA controls cover helper and caller origins, aggregate elements,
deferred registration, external receiver storage, local obligations, nonreturning
success paths, zero allowance and unavailable input. The integrated trace test
requires exactly one final candidate-associated decision for caller ownership,
deferred registration and a nonreturning success path. Restoring the parent entry
dispatcher through a source overlay fails all three missing-decision assertions.

Scoped all-check comparisons cover process/closure-choice fixtures, program-entry
fixtures, pinned Ferro MCP and pinned Openase hooks. The complete diagnostic
payloads preserve 41, 2, 1 and 0 findings respectively. Traced fixture JSON also
agrees with untraced JSON. Openase silence and Ferro's remaining finding receive
no correction credit. The five recorded residual production sites stay open.

Final `make verify VERIFY_TIMINGS=1` passes all eight gates. The canonical
binary and frozen reviewed binary have SHA-256
`af6c231d8f7c2ca5bfb78e045ef5ea939c8d64bde6365741ec6d116ad6efe87d`.
Eight terminal scope receipts have exit zero and empty stderr; parent SHA-256 is
`d6084bacb7aa9b06104540fe115dfd1c50feea941768e542575329bc3339b16d`.
The final fixture trace has exactly one decision for each of its 153 candidate
positions, including unknown registration and accepted nonreturning success.
Receipts and complete payload comparisons live in
`.build/goal-process-prestart-scans/`; the final traced JSON is also byte-identical
to its untraced payload. Final architecture validation after these documentation
updates is recorded in the Bead. This scoped refactor does
not establish absence of differently structured duplication in other analyzers
or transitive engines. No full precision replay or local race run is used.

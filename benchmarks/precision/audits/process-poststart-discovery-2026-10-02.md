# Post-Start process discovery consolidation

Beads `gohawk-dho.23.6`; parent source `45a0eb2`.

## Owned evidence

`command_use.go` owns the final command-use boundary. The reporting decision
consumes one structured proof instead of an unbounded Boolean absence query;
tracing consumes that same decision. Instruction, possible-reachability,
closure-binding, operand, reaching-value and stored-value visits share one
candidate child allowance through existing shared APIs. Interrupted discovery
remains unknown with `command-use-budget-exhausted`, rather than proving either
a handoff or unused-command intent.

The original possible-reachability relation includes loop back edges. This
policy is retained through `InstructionMayFollowWithin`; the shared runtime-value
use-after census stops at back edges and is not a semantic replacement here.
Possible use of a handle retains the missing-wait question; no use leaves
intent unknown. Scalar PID/name reads, Start's own error, and synchronous
command-pipe IO retain their distinct boundaries. Wrapper selection and the
single reaching-walk cycle guard remain unchanged.

`returned_process_owner.go` owns possible returned lower-level handle evidence.
The shared bounded body census charges unrelated instructions as well as
selected loads. Aggregate containment uses the existing
`ProveReturnedOwnershipWithin` with that allowance. A possible owner or cutoff
maps to unknown reaping, never an exact Wait. Call argument visits also charge
one child allowance while completion/binding requests retain their own children.
Independent heap, symbol and type internals keep their costs. Other return
allowance and flow queries are not claimed bounded by this change.

## Verification

Compiled SSA controls distinguish direct, captured and aggregate uses from
unused launches, Start-error-only returns and scalar PID consumption. A
back-edge control pins the original structural reachability. Returned handles,
PID-only aggregates and unrelated owners pin containment polarity. Every child
cutoff is unknown; fresh queries recover while retaining the parent pool. The
shared test query harness explicitly rejects a query bypassing zero allowance.
A direct call-argument control verifies cutoff and fresh recovery for scalar
arguments. The final decision trace asserts phase, stable reason, unknown
outcome and candidate association; fresh discovery yields the ordinary
unowned-return decision.

Three isolated source-overlay counterfactuals fail assertions: cutoff mapped
to unused, uncharged returned-body census and replacement by the back-edge-
excluding use-after census. Production source is not modified by these runs.

The [ledger](process-poststart-discovery-2026-10-02.tsv) records six terminal
parent/current all-check scans with empty stderr. Complete diagnostic payloads
are identical: process/choice fixtures retain 40 findings, clean pinned Ferro
`./mcp` retains its one unrelated finding, and clean pinned rev-dep
`./internal/telemetry` remains silent. The unresolved Ferro receiver-state target
remains historically budget-silent and receives no correction credit. Completed
parent receipts are reused; all current receipts use the immutable reviewed
binary.

Parent SHA-256:
`a8ae71c4f8849f4ae2a4723e38cbab40432d2d020f4b21fbe500e72c924212a3`.
Current SHA-256:
`1b68d2f2af3677175d2a963b0e10d32ff283c0aca63570fe82f28dad09c02d7a`.
The traced fixture scan has empty stderr and byte-identical diagnostic JSON;
its decisions cover accepted, rejected and unknown outcomes. The new cutoff
reason is exercised directly in the compiled-SSA decision trace control.

Receipts: `.build/goal-process-poststart-focused.log`,
`.build/goal-process-poststart-controls-final.log`,
`.build/goal-process-poststart-mutants/results.json`,
`.build/goal-process-poststart-scoped/scans.json`,
`.build/goal-process-poststart-fixture-traced.json`,
`.build/goal-process-poststart-fixture.trace.jsonl`,
`.build/goal-process-poststart-verify-final.log` and
`.build/goal-process-poststart-architecture-final.log`.
Initial canonical validation found one overlong trace assertion, corrected
before the final gate. Final canonical validation passes all eight targets;
the final architecture check passes.
Graph tools were unavailable; source fallback and actual compiled SSA provide
the evidence. No full precision replay or local race run was used.

No recorded production FP correction is credited. Seven production sites plus
Rune and whole-objective completion remain open.

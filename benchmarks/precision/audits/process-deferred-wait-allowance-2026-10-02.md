# Deferred process wait discovery and allowance

Beads `gohawk-dho.23.2`.

## Corrected ownership and responsibility

`deferredClosureWaitsForCommand` searched a callback body separately for each
mapped command, then `guardedDeferredWait` rediscovered stores and loads.
Its coverage requests used the unbounded `MethodCallCoverage` adapter despite
an existing bounded shared API and a command-candidate allowance.

One focused deferred-wait search now collects callable instructions, stores
and loads once, before interpreting bindings. An interrupted census publishes
no exact completion or absence claim. Binding visits, selected witnesses and
`ProveMethodCallCoverageWithin` draw from the command candidate's child budget;
cutoff is unknown. Capture-before-argument priority and the existing
successful-Start non-nil Process assumption are unchanged. Defensive Process
guards remain uncertain where separate loads cannot prove exact identity;
independent Boolean guards and Process replacement remain diagnostic.
No fact guarantees or unconditional Start result claims are added.

`deferred_wait.go` owns this particular evidence model and its assumption,
while `ownership.go` retains ordinary transfer and asynchronous handoff.
Heap identity, type and positional binding metadata keep their existing
independent cost boundaries. The change does not prove a global time bound.

## Validation and scope

`deferred_wait_test.go` compiles actual SSA for exact, conditional, guarded and
replaced Process values. Each intermediate child cutoff must be unknown;
a fresh child under the same nonexhausted parent must recover its expected
answer. Exhausted classifier and direct coverage controls verify both entry
points. Two counterfactuals fail assertions: discarding the candidate allowance
proves the exact waiter despite cutoff, and bypassing bounded coverage returns
called-completion on an exhausted coverage request.

Focused suite and control receipts are `.build/goal-process-deferred-focused.log`,
`.build/goal-process-deferred-controls-final.log` and
`.build/goal-process-deferred-mutants/results.json`. The existing fixtures in
`processownership.go` and `guarded_merge.go` retain accepted defensive guards
and diagnostic independent flags or Process replacement. Counterfactual
receipts precede a receiver-style correction that does not alter their semantics.

The [ledger](process-deferred-wait-allowance-2026-10-02.tsv) records six terminal
scoped parent/current all-check scans with empty stderr. Completed parent
receipts are reused; three current scopes were refreshed against the final
immutable binary after the receiver-style correction. All 37 process fixture
findings retain identical payloads. Clean pinned Ferro
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4` `./mcp` retains one unrelated
finding; its previously budget-silent target remains unresolved. Clean pinned
rev-dep `8a2fdb0927e2fc9b2a5b178c94f55d1887659152` `./internal/telemetry`
remains silent under its existing pipe boundary.

Parent binary SHA-256:
`7b3a353a1faccabd2c8837c5eddbd5c100259e89efaa3f5c6456e9bcde9612d6`.
Final binary SHA-256:
`67fef5820e87ebf0692fadc8d41456ddda537e42f6d4d77af4d91eabb734ec23`.
Final canonical `make verify VERIFY_TIMINGS=1` passes all eight targets
(`.build/goal-process-deferred-verify-final.log`); final architecture validation
passes (`.build/goal-process-deferred-architecture-final.log`). Initial canonical
validation found mixed receiver style on the search type; this was corrected.
No full precision replay or local race run was used.
Graph tools were unavailable; exact source and compiled SSA supply scoped evidence.

No production FP correction is credited. Seven recorded production FP sites
plus Rune remain unresolved; this is not a completed architecture audit.
A source-only goiardi reassessment confirms mutable caller configuration needs
stability across intervening calls. The existing private-use inventories and
unconditional fact model do not supply that caller-context guarantee; no new
scan or correction is claimed for those sites.

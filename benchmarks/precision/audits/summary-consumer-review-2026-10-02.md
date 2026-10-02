# Bounded summary-consumer review

Beads: `gohawk-dho.44.11.5.27.24` and child `.24.2`.

## Scope and evidence

The refreshed [typed ledger](summary-consumer-locations-2026-10-02.tsv) contains
71 declaration-resolved broker uses in 28 analyzer files. The locator uses
`go/packages` type information for the current production `./internal/...`
build and excludes the broker itself. This covers eight analyzer families,
including package-level selections. Tests, fixtures, other build configurations
and source outside this scope are not covered. Graph tools are unavailable;
source/type-information fallback supplies the evidence.

The enclosing consumer functions were read, with narrow follow-up reads for
producer send/receive normalization and exact completion/retention consumers.
The original 38-function source extraction is retained locally at
`.build/goal-summary-consumer-functions.json`; its two outcome adapters have
since been replaced by broker calls in the existing result-guard functions.
This review disposes the finite consumer inventory; it is not a complete review
of every transitively reachable proof or the entire analyzer catalog.

## Dispositions

| Family | Reviewed boundary and disposition |
| --- | --- |
| Setup, all eight families | Requirements and prerequisites come from the same fixed selection. Provider acquisition remains setup; domain evidence stays selected independently. |
| concurrentcapture | Complete call-bound lock effects feed possible guard policy. Missing/incomplete effects retain the opaque fallback. Imported binding delegates to the engine, as reviewed in child `.24.1`. |
| goroutineownership | Exact completion requires proven mapped targets. Retention, canceled-context and opaque-owner evidence suppress uncertainty rather than inventing a join. Successors and termination serve the existing single flow query. |
| lockorder | Complete effects are bound before setup publication; interrupted setup is discarded. Successor feasibility and nonreturn evidence use the walk allowance and do not establish synchronization themselves. |
| producerlifecycle | Complete worker effects normalize sends and receiver helpers together. Unknown callbacks and asynchronous receivers decline the finite receive count. Ordered attribution, direct-body fallback and producer-count policy stay local. |
| cancellationownership | Lifecycle evidence discovers obligations; one classifier/flow decides their outcome. Literal/result lookup moves to `Provider.OutcomeOf`; guard retention and exact cancel identity stay local. |
| deferinloop | Lifecycle evidence establishes an obligation; local state transitions decide live backedges. Result feasibility removes only proven edges, not opaque cleanup uncertainty. |
| processownership | Lifecycle candidates and exact command identity feed one ownership flow. Result successors affect reachability; returned aggregates and opaque handoffs remain ownership policy. |
| resourcelifetime | Error implications use exact arguments; unchanged-argument forwarding preserves typed receiver identity; returned views cannot establish cleanup. Feasibility/nonreturn and shared outcome lookup remain separate from acquisition/release policy. |

The outcome lookup was the remaining mechanical copy identified by these reads.
It first recognizes literals/constructions, then queries an unconditional result
guarantee. Nil providers and unavailable components preserve literal knowledge;
unknown calls and interrupted summary queries stay unknown. A boxed typed-nil
pointer stays a nonnil interface. No arguments strengthen a callee guarantee.

The independent inference, publication and consumer allowances retain their
existing contracts. This work introduces no whole-query time bound, new fact
payload, reporting policy or production FP credit. Ten production FP sites and
Rune remain unresolved; the broader consolidation goal remains open.

## Validation

Focused summaries/resource/cancellation tests passed. Actual compiled SSA
controls exercise true/false/nil/non-nil results, typed-nil boxing, opaque and
argument-dependent results, unavailable components and an exhausted allowance.
Three counterfactual overlays fail assertions: removing literal evidence,
losing boxing, and treating unknown calls as false.

Four affected-fixture parent/current scans pass with empty stderr, preserving
all 318 resource and 37 cancellation diagnostics byte-for-byte. The
[scan ledger](summary-consumer-review-2026-10-02.tsv) records exits, binary hashes
and receipt paths. Scope is the affected fixtures and canonical self-dogfood;
this change does not repeat unaffected pinned lock scopes or claim a production
precision improvement.

Final `make verify VERIFY_TIMINGS=1` passes all eight canonical targets,
including ordinary tests and self-dogfood. Final architecture validation passes.

Local receipts: `.build/goal-outcome-broker-focused.log`,
`.build/goal-outcome-broker-mutants/results.json`,
`.build/goal-outcome-broker-scoped/scans.json`,
`.build/goal-outcome-broker-verify.log` and
`.build/goal-outcome-broker-architecture-final.log`.
No full precision-regression replay or local race run is used.

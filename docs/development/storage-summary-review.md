# Storage and summary decision review

This source-based review follows `gohawk-dho.21` and is tracked by
`gohawk-dho.22`. Its bounded inventory is the decision points listed below in
`heapmodel`, `summaries`, and the three domain passes. Graph tools were
unavailable; targeted source reads and searches supplied the evidence. It is
not an exhaustive certification of every helper in those packages.

## Consolidated decisions

| Decision owner | Shared mechanics and retained policy |
| --- | --- |
| `Storage.projectionUsesPreserveStorage` in `store_projection.go` | Root and selected-address projection checks share referrer enumeration, budget charging, observation-window filtering, outward-wrapper recursion and rejection of repeated values. The driver keeps depth-first order. Root policy permits field/index selections and nil comparisons; address policy permits exact pointer loads. Each policy still checks calls through `CallEffects`. |
| `slotBeneath` in `store_regions.go` | Reaching writes, stable cell contents and stable field contents use the existing delimiter-aware path containment predicate. Ancestor writes initialize a selected location; descendant writes partially replace it; siblings are disjoint. Storage paths retain their leading slash and field/index encoding, while heap-summary paths retain their separate vocabulary. The helper compares path structure only; callers retain root and exactness checks. |
| `capturedTargetUses` in `lifecyclefacts/evidence.go` | Retention and unreadable-callee queries share capture binding, possible target provenance and enumeration of the free variable plus its pointer loads. Uses stay grouped per capture, and the iterator stops at the caller's first witness. The consuming queries retain different semantics: a visible escaping store can retain without an unreadable call. |
| `regionGraph.view` in `store_regions_transfer.go` | Index selection, slicing and selected aggregate snapshots share recorded-window lookup and the full-array fallback. Existing offsets, lengths and capacities are returned unchanged; an unrecorded slice remains unknown and an empty array remains known. The consumers retain index bounds, slice-bound validation and snapshot size/staleness checks. This later consolidation is tracked by `gohawk-dho.34`. |

The [projection tests](../../internal/heapmodel/store_projection_test.go)
cover unchanged roots/slots, mutation, retention, later escapes, ambiguous
roots, unrelated owners, and converted roots/slot addresses. Converted cases
check that the built SSA actually retains a `ChangeType` wrapper. The
[capture evidence tests](../../internal/passes/lifecyclefacts/capture_evidence_test.go)
compare visible retention, opaque handoff, read-only use, unrelated captures,
derived captures and multiple captures. Both sets pass against the parent
implementation and the consolidated implementation.

Existing [storage snapshot tests](../../internal/heapmodel/store_model_test.go),
[field stability tests](../../internal/heapmodel/store_field_stability_test.go)
and [give-up observer tests](../../internal/heapmodel/store_observer_test.go)
retain aggregate-copy, ancestor/sibling-write, observation and budget checks.
No fact schema or audited production FP count changes in this consolidation.

## Distinct decisions retained

| Decision owner | Why its contract remains separate |
| --- | --- |
| `Storage.Content`, `ContentFromWrites`, `StableContent`, `StableFieldContent` | All use the reaching-write engine. Content reads a snapshot and may use an exact graph fallback; ContentFromWrites deliberately avoids graph construction. StableContent additionally rejects later mutation, excluding the observation's own effects. StableFieldContent includes that observation and permits disjoint sibling uses. Merging these policies would change when a stored callback or worker reference is exact. |
| `resolveHeapCall`, `bindHeapCaptures` | Heap substitution preserves concrete generic instantiations, receiver/argument arity and capture contents at the instruction. Captures must be read-only and nested pointer projections remain unsupported. Dispatch resolution supplies a target, not an effect guarantee. |
| `heapSummaryOf`, `applyHeapSummary` | The registry owns availability, generation and on-demand projection. Application owns call-cycle cuts, substitution and truncation. Escapes are applied to the incoming state before edges overwrite slots. Neither is a second storage-content proof. |
| `Provider`, `Function.Results`, `Function.Lifecycle`, `Function.Concurrency` | Setup selection controls ordinary prerequisite scheduling. Availability belongs to each domain; an available result may still be Unknown, lifecycle facts may omit relationships, and concurrency declarations require their own completeness. The broker does not merge these meanings or infer unrequested knowledge. |
| `Provider.ResultOf`, `pairedNilness`, `ArgumentReturnedUnchanged` | They share result-source mapping and selected declaration access, but ask different questions: unconditional outcome, a case whose premise holds on the path, and exact returned-argument identity. Unknown and exhausted evidence do not prune successors. Returned identity does not discharge ownership. |
| `storedResultQuery` | Consumes ContentFromWrites at the load's execution point, retaining result-value semantics such as nonnil interface boxing. Identity-only unwrapping would corrupt nilness guarantees. |
| `LifecycleEvidence.Prove`, `selectedMaskProof`, `factArgumentMatches`, `factOwnsArgument` | One local/imported proof path owns completion/transfer decisions. Exact masked arguments, containment, immutable captures and strict projections have different binding requirements. A possible alias is uncertainty rather than completion, and an abandoned local search cannot become a disproof from an imported mask. |
| `concurrencyfacts.Engine.bind`, `bindCapturedRoot` | Ordered effects bind reference identities and embedded field paths. A forwarded free variable can remain symbolic; an actual captured cell needs StableContent. Field-root binding also selects a canonical caller load. This is distinct from heap capture substitution and lifecycle retention. |
| `ReturnsParameterUnchanged` | Result relationships and lifecycle returned-view inference already delegate exact unchanged-return proofs to the same lifecycle query. No second identity proof is introduced in the broker. |

The result-facts package description now includes its result cases and normal
termination guarantees. Misplaced lifecycle comments now accompany the
functions they describe. These are documentation corrections to existing
contracts, not new guarantees.

Further reviews must preserve the state, order, budget, availability and proof
polarity behind these distinctions. An absence of an imported relationship
does not prove no effect; a possible owner does not prove cleanup. The
[remaining FP assessment](../../benchmarks/precision/audits/remaining-fp-assessment-2026-10-01.md)
continues to track the 18 unresolved production locations independently.

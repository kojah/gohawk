# Graph containment consolidation — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.15`. Parent: `a6b7aca`.

## Shared traversal and retained policies

Whole-build `regionGraph.contains` and observed `containsAt` repeated the same
bounded region search. Both now select their slot map and delegate to private
`containsThroughSlots` in `store_regions_containment.go`. The whole-build query
selects history; the observed query selects the state before its instruction.
Each retains its graph lock, points-to gates and existing availability answer.
An unavailable observation remains unknown; absent tracked values in an
available observation remain a known negative.

The shared search preserves the existing eight-level limit, region cycle guard,
exact target-slot membership and possible containment through unknown pointees.
Stale pointees retain may-polarity. No budget, graph construction, summary schema,
ownership policy, cleanup guarantee or public behavior changes. This traversal
walks abstract regions and slot maps; it does not reproduce SSA reaching-value
traversal owned by ssaflow. Moving the concern reduces the mixed query file from
530 to 437 lines; it remains an existing outlier, with alias, identity and
observed storage query responsibilities unchanged.

Graph tools were unavailable. Source fallback inspected query.go,
store_regions_query.go, store_regions_content.go, store_regions_cache.go,
store_alias.go, store_derivation.go, alias_decision.go, lifecycle's
store_ownership.go, resource ownership consumers and lifecyclefacts heap claims.
The containment query bodies were read completely, along with their direct
callers. Selected graph replay/state and effect/cycle bodies were also read;
this does not certify every transitive engine or fact consumer.

| Neighboring query | Distinct contract retained |
| --- | --- |
| slotsMayAlias/everHeld | Placeholder and relative-path alias recursion; may identity, not owner containment. |
| mustSame/singleSlot | One non-stale exact slot; unknown/wildcard sets cannot prove identity. |
| everContainedUnlocked | One object's strict descendant history, used by deferred-cell relation; not the transitive region search. |
| contentWhenDeferredRun | Union of readable reachable deferred-execution points or returns; separate observation selection and costs. |
| storedPath/valueAtPath | Exact selected content and bounded static paths, not possible reachability. |
| exclusiveAt/publishedAfterUnlocked | Exact object exclusivity and later publication; independent escape-state interpretation. |
| escape/clobber | State mutation, field-prefix reach and read-only closure policy; do not merge with a read-only depth-limited containment query. |

Graph construction/waiting, alias/type internals, deferred observation allowance,
cycle metadata, package inventories and other fact consumers remain open.
The separate redundant-query leads in Storage.Same/Content were not changed:
graph queries can replay state and summary registration can replace a cached
graph, so source similarity alone is insufficient to delete a retry.

## Regression evidence

New controls compile actual Go to SSA and exercise both public containment
queries before and after later/replaced stores, unrelated values, nested owners,
cycles and both sides of the existing depth limit. A missing observation remains
unknown. These controls pass before the refactor as well as after it; they pin
compatibility rather than introduce a new policy. The existing history widening
control additionally verifies transitive may-containment after a slot widens
to unknown.

Three ignored overlays fail behavioral assertions, rather than compilation:

- Selecting history for ContainsAt reports containment before a later store
  and after replacement (`history-at-counterfactual.log`).
- Extending depth by one reaches a previously excluded nested object
  (`depth-counterfactual.log`).
- Discarding unknown pointees loses possible widened-history containment
  (`unknown-counterfactual.log`).

Receipts are under `.build/goal-graph-containment/`. Focused heapmodel,
lifecycle and lifecyclefacts tests pass in
`.build/goal-graph-containment-focused-final.log`. Canonical
`make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests including
architecture checks. Receipt `.build/goal-graph-containment-verify.log`.
No full precision-regression replay or local race test ran.

The refreshed complete-body normalized scan covers 316 production files,
2,146 function bodies and the same five domain-distinct candidate groups.
Neither new containment function appears in those groups. Receipt
`.build/goal-graph-containment-duplicates.json`. The duplicated containment
search was found by source review outside the scanner's complete-body matches;
the scan cannot prove that partial or differently shaped duplication is absent.

## Fixture and production comparison

Immutable binary `.build/goal-graph-containment-reviewed`, SHA-256
`1ba01a029904656316faab3fbcd6398510e1f11b182531ba09d80fdc2f14be11`.
Parent binary `.build/goal-process-local-pipes-reviewed-final`, SHA-256
`38c27d44a3ac00cbea721f53818cb64d23ac4db74c0a39b3cf7256054c1b613e`.
All-check fixture vet runs cover resourcelifetime and goroutineownership with
their local GOPATH stubs. Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab`
uses `./lib/configparser`; goiardi
`937cae400a92d8036b88ae2f65d93506271c292e` uses `./datastore ./indexer`.
Production modules are readonly, CGO/workspaces disabled, timeout 180 seconds
and scan concurrency at most two. Final receipts are under
`.build/goal-graph-containment-final/`, including per-scan exit/hash metadata.

All eight final scans exit zero with empty stderr. Both fixture diagnostic
streams are byte-identical to parent (335,148/116,184 bytes); XD and goiardi
retain byte-identical 838/1,476-byte streams and all four exact reviewed lock
position/check keys. The [comparison ledger](graph-containment-consolidation-2026-10-02.tsv)
records scopes, counts and receipts.

The initial scan coordinator used an incorrect XD checkout alias and failed
before writing combined metadata. Those receipts are retained separately;
the corrected final coordinator records each exit before assembling its index.
No failed or missing scan is counted as retained diagnostics. Ten production
FP sites plus Rune remain unresolved; this mechanical consolidation credits no
production correction or complete architecture claim.

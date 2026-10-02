# Call-cycle inventory consolidation — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.17`. Parent: `a205cd2`.

## Shared structural discovery

Reachability was cached per root, but each new root rescanned bodies already
examined from other roots. `cycleMetadata` now shares each function's owning
package and direct static callees under the existing cycle-cache lock.
Competing readers discover privately and reuse the first published immutable
inventory. A traversal clones its initial queue; appending successors cannot
overwrite the shared slice. Summary registration cannot change structural edges.

Package filtering still precedes discovery of a successor's body. Static
Call/Go/Defer edges retain their order, duplicates and generic wrapper/origin
alternatives. Dynamic calls, interface invokes and builtins contribute no edges.
The generic instantiation wrapper exception and recursive-cycle polarity remain
unchanged. `sameCallCycle` resolves each origin once per query.

Graph tools were unavailable. Source fallback read the complete cycle module
and the direct consumers in `store_heap_apply.go` and
`store_heap_requirements.go`. Both continue to cut recursive summary evidence
through the same authoritative cycle query. No fact schema or guarantee changes.

Like existing reachability caching, this assumes completed immutable SSA bodies.
It retains additional per-function metadata and direct edges alongside transitive
reach sets in the process cache. No eviction or whole-query cost bound is added;
no measured speed or memory improvement is claimed. Graph build/replay,
pointee/alias/type internals, package inventories and other fact consumers remain
open. Ten production FP sites plus Rune remain unresolved.

## Validation

Actual SSA controls cover branching work queues, shared roots, self/mutual
recursion, static Go/Defer versus dynamic calls, foreign packages, generic
wrappers, summary registration and concurrent first publication. Dumps and
focused heapmodel/lifecycle/lifecyclefacts results are in
`.build/goal-cycle-inventory-focused-final.log`.

Three ignored overlays fail assertions rather than compilation: sharing the
mutable queue with the inventory; crossing the package boundary; treating a
wrapper's call to its origin as recursion. Receipts are
`.build/goal-cycle-inventory-mutants/{queue-alias,foreign-body,wrapper-cycle}.log`.
Ordinary concurrent controls pass; no local race run is credited.

Canonical `make verify VERIFY_TIMINGS=1` passes all eight targets, including
ordinary tests and self-dogfood (`.build/goal-cycle-inventory-verify.log`). No
full precision-regression audit ran. The normalized complete-body scan retains
the same five distinct-contract groups across 317 files and 2,147 functions
(`.build/goal-cycle-inventory-duplicates.json`). It cannot prove partial or
differently structured duplication absent.

## Scoped compatibility

Current binary `.build/goal-cycle-inventory-reviewed`, SHA-256
`07c3a4903ddb910b139fd828d593b65480f2f9d5724c53d3105e8ff555eb19f8`.
Parent `.build/goal-deferred-cell-reviewed`, SHA-256
`4d22690ea7e7367027eef82ce2e8876c67ad7b509a3763d2675228167be7324c`.

The [ledger](call-cycle-inventory-consolidation-2026-10-02.tsv) records all-check
resource/goroutine fixture comparisons and pinned XD/goiardi controls. All eight
scans exit zero with empty stderr and per-scan hash/exit metadata in
`.build/goal-cycle-inventory-final/`. All 444 fixture diagnostics retain identical
335,148/116,184-byte JSON. XD's two missing releases and goiardi's two read-lock
writes retain identical keys and 838/1,476-byte JSON. Production modules are
readonly, CGO/workspaces disabled, timeout 180 seconds, concurrency at most two.
No FP correction or overall architecture completion is credited.

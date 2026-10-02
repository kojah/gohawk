# Deferred-cell observation allowance — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.16`. Parent: `bce4417`.

## One observation census and request-owned relation

The finite graph-query inventory identified a completion request bypass:
`deferredCellLocal` called an unbudgeted graph relation after its bounded
storage proof. That relation separately scanned all RunDefers and, when none
existed, all returns, then used default reachability and history queries.

`DeferredCellRelationWithin` now receives the completion request's existing
allowance. Its owning file `store_deferred_cells.go` keeps observation selection
and complete occupant interpretation together. One `InstructionsWithin` census
selects RunDefers or the existing return fallback for test-registered callbacks.
Reachability uses `InstructionMayFollowWithin`; slot unions, occupant comparison
and descendant history visits share that allowance. No unused default adapter
is retained: the sole production consumer submits its budget directly.

An interrupted observation census publishes no points. An interrupted union or
history query publishes no relation, including no known negative. The caller
therefore cannot map a completing prefix or rescue it with targetStoredOnPath.
The enclosing completion memo and final proof retain their existing shared
budget cutoff handling. Graph locks, nil-alternative handling, stale exactness,
possible aggregate containment and nil-budget behavior remain unchanged.

Graph construction/waiting, state replay, pointee lookup/add internals, alias
and type work retain independent costs. This is not a whole-query time bound,
an allowance increase, a new cleanup guarantee or a fact-schema change. The development model and generated helper reference document the shared
allowance; diagnostic compatibility is verified for the scopes below.

Graph tools were unavailable. Source fallback completely read the relation,
deferred observation and history query bodies, their sole completion consumer,
mappedLocals, completion request finalization and completion memo composition.
The preceding containment inventory supplies neighboring query dispositions.
Cycle metadata, package inventories and other fact consumers remain open.

## Regression evidence

Compiled SSA controls cover exact, cleared, replaced, merged, aggregate and
test-registered cells. Real function dumps are logged in
`.build/goal-deferred-cell-focused-final.log`. Every intermediate tested cutoff
returns unknown/unavailable; a fresh child allowance recovers the completed
relation without exhausting its parent. A separate actual SSA function reaches
one RunDefers before the census finishes; the cutoff discards that prefix and
a fresh query recovers all execution points.

The lifecycle mapping control starts with an exhausted request and a readable
cleared-cell graph. It must not return an exact local through the graph bypass;
a fresh allowance in the same search recovers the exact mapping. Focused
heapmodel, lifecycle and lifecyclefacts tests pass.

Three ignored overlays fail assertions rather than compilation:

- Detaching the relation from the request allowance publishes exact/contains
  answers at cutoff and lets the lifecycle mapping bypass its exhausted request.
- Keeping an interrupted census publishes one observation as a complete set.
- Dropping final relation availability publishes exact or known-negative
  answers after history examination is interrupted.

Receipts `.build/goal-deferred-cell/{detached-request,observation-prefix,relation-prefix}-counterfactual.log`.
Canonical `make verify VERIFY_TIMINGS=1` passes all eight targets: generation,
module verification, formatting, vet, deadcode, lint, self-dogfood and ordinary
tests including architecture checks. Receipt `.build/goal-deferred-cell-verify.log`.
No full precision-regression replay or local race test ran.

The refreshed normalized complete-body inventory covers 317 production files,
2,147 function bodies and the same five domain-distinct groups, with no changed
function in a group. Receipt `.build/goal-deferred-cell-duplicates.json`.
This thresholded scan cannot establish absence of partial or differently shaped
duplication. The concrete double census and default query bypass are removed.

## Fixture and production controls

Immutable current binary `.build/goal-deferred-cell-reviewed`, SHA-256
`4d22690ea7e7367027eef82ce2e8876c67ad7b509a3763d2675228167be7324c`.
Parent `.build/goal-graph-containment-reviewed`, SHA-256
`1ba01a029904656316faab3fbcd6398510e1f11b182531ba09d80fdc2f14be11`.
The [comparison ledger](deferred-cell-allowance-followup-2026-10-02.tsv) records
all-check resource/goroutine fixtures and pinned XD/goiardi package scopes.
All eight scans exit zero with empty stderr and per-scan exit/hash metadata in
`.build/goal-deferred-cell-final/`.

The two fixture streams preserve all 444 diagnostics byte-for-byte
(335,148/116,184 bytes). XD's two missing lock releases and goiardi's two
read-lock writes retain their exact reviewed keys and byte-identical 838/1,476
byte JSON. Production modules are readonly, CGO/workspaces disabled, timeout
180 seconds and scan concurrency at most two. No production FP correction is
credited: ten recorded sites plus Rune remain unresolved. Broader architecture
completion remains unproven.

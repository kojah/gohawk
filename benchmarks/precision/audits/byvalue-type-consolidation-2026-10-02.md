# By-value type consolidation — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.22`. Parent: `b5ed3d3`.

## Shared traversal and bounded review

`CanHoldReference` and `holdsByValue` repeated struct-field and array-element
recursion. They now use one private `anyByValueType` traversal with separate
predicates. Pointer/interface/slice/function edges remain leaves; overwrite
matching retains underlying-struct identity, including different named types
with identical underlying structs. Arrays of length zero retain element
traversal. No public API, fact schema, budget or diagnostic policy changes.

Graph tools were unavailable. Source fallback reviewed these specific families:

| Family | Disposition |
| --- | --- |
| Reference capability and by-value overwrite (`query.go`, `store_write_once.go`) | Share struct/array traversal; retain caller-owned predicates. |
| Synchronization primitive containment (`store_write_once.go`) | Retain field-based primitive matching and root/array policy. It is not the same Boolean query. |
| Reference slot enumeration (`store_aggregate_fields.go`) | Already shared by snapshot construction and result projection. Paths, array cuts, depth and slot limits remain distinct. |
| Possible identity (`store_alias.go`, `query.go`, `ssaflow/value_matching.go`) | Boolean MayAlias delegates to one proof. Structural fallback includes possible phi/store alternatives; it cannot replace exact identity. |
| Exact identity (`ssaflow/value_identity.go`, `store_alias.go`) | Every represented value must agree; graph evidence adds exact content. A failed proof is not inequality. |
| Static access paths (`ssaflow/access_paths.go`) | Path/read selection share one implementation. Both-path matching and identity-with-path-fallback retain their different preconditions. |
| Derivation (`store_derivation.go`, `ssaflow/value_derivation.go`) | Shared work-list driver accepts a caller identity callback. Operand contribution is broader than transparent identity. |

This is a task-directed inventory, not a certification of all alias/type
internals or fact consumers. Graph construction, alias-query internals and
type-system operations retain independent costs. The possible-identity fallback
still has its own recursive mechanics; its consolidation requires a separate
policy-preserving review rather than substitution with the exact-identity fold.

## Validation

`byvalue_types_test.go` compiles real Go types and checks thirteen shapes:
root, alias, identical underlying named struct, nested arrays, ordinary and
zero-length arrays, pointers and pointer fields, interfaces, slices, functions,
pointer-recursive types and strings. Heapmodel and lifecycle tests pass in
`.build/goal-byvalue-types-focused.log`.

Ignored overlay counterfactuals stopping at arrays or following pointers both
fail assertions, with no compilation failure. Receipts are
`.build/goal-byvalue-types-mutants/{stop-arrays,follow-pointers}.log`.
The normalized complete-body scan covers 319 production files and 2,150
functions, retaining five previously dispositioned distinct-contract groups.
It cannot prove partial duplication absent.

Canonical validation completes generation, module verification, formatting,
vet, dead-code, self-dogfood and ordinary tests successfully in
`.build/goal-byvalue-types-verify.log`. Its lint target encounters a concurrent
golangci-lint lock and makes the aggregate command exit two. Once the competing
process is gone, `make lint` passes with zero issues in
`.build/goal-byvalue-types-lint-retry.log`; the other passing targets are reused.
Final architecture tests pass in
`.build/goal-byvalue-types-architecture-final.log`.

## Scoped compatibility and limits

Parent `.build/goal-backing-paths-reviewed`, SHA-256
`84d25990402cbe18a9a2cfddadb27240843fa48325873974dd9cb2707b966e3b`.
Current `.build/goal-byvalue-types-reviewed`, SHA-256
`266f559a0b213594e615fa99235723dc56131a9c4e6fb2e9f7a28e55b0f63263`.

The [comparison ledger](byvalue-type-consolidation-2026-10-02.tsv) records six
parent/current scopes. All twelve scans exit zero with empty stderr; each has
hash and exit metadata in `.build/goal-byvalue-types-final/`. All 552 fixture
diagnostics, four pinned XD/goiardi production controls and both SkyWalking FP
sites (each duplicated) retain byte-identical JSON. Production scans use readonly
modules, disabled CGO/workspaces, 180-second timeouts and at most two workers.
No full precision-regression replay or local race run is performed.

No production FP correction is claimed. Ten production sites plus Rune and the
broader completion review remain open. Beads `gohawk-dho.44.11.5.27.23` tracks
the concrete possible-identity traversal review identified above; other fact
consumers remain outside this task's verification scope.

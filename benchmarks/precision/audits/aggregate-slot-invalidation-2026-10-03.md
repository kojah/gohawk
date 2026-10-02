# Uncertain aggregate storage invalidation

Beads: `gohawk-dho.23.11`. Parent: `9ac9e3f`.

## Reproduced boundary and shared correction

Actual compiled SSA controls reproduce stale exact fields after whole and
nested aggregate writes through mixed destinations. `storeAggregate` marks a
possible write unknown, but the previous concrete fields remain in state and
are read before the stamp. An unknown stamp alone therefore does not invalidate
an older exact field. A definite nested replacement is the parent positive
control.

Possible or undescribed aggregate replacements now forget selected concrete
contents and backing copies, drop cached enclosing aggregates, and stamp the
selected storage unknown. `forgetStoredSlot` owns that mechanical operation
and also serves local summary invalidation. The foreign epoch and closure
policies remain in substitution; this does not silently impose a local storage
contract on foreign objects. No recursive clobber of former pointees is used.

Definite aggregate copies retain their existing source-copy path, and definite
zero values clear the selected storage without acquiring unknown-write
semantics. Earlier snapshots and untouched sibling storage remain intact.
Uncertain aggregates stay unknown rather than inventing a fieldwise union or
claiming that every possible destination was overwritten.

## Behavioral controls

`store_aggregate_writes_test.go` compiles seven controls: possible whole and
nested destination writes, mixed source values, definite whole and nested
replacement, definite zero, and former-pointee preservation. Snapshot and
untouched-sibling checks accompany the relevant writes. Parent failure and
actual SSA/heap evidence are in `.build/goal-aggregate-writes-parent.log`.
Whole heapmodel tests pass in `.build/goal-aggregate-writes-final-controls.log`.

Four final-source overlays fail behavioral assertions: stamp without removing
concrete entries, force possible writes definite, recursively clobber former
pointees, and mark definite zero unknown. All are terminal exit 1 with assertion
failures, recorded in `.build/goal-aggregate-writes-mutants/results.json` and
individual logs. Production source was not temporarily mutated for these
counterfactuals.

## Scoped comparisons

Parent executable: `.build/goal-summary-writes-complete-reviewed`, SHA-256
`141f09899d6d345235e599f56542f35b5a6fe1ec9863e3f068c206d24b9f2d4d`.
Its six successful current receipts from the previous review are reused as
parents with unchanged scopes and repository pin. Current executable:
`.build/goal-aggregate-writes-reviewed`, SHA-256
`0a8048ec0356b4b195935b04f3b713179423f65d3f90c7970929b76dab430c77`.

All twelve final parent/current receipts are terminal exit 0 with empty
stderr. Complete merged diagnostic payloads are identical: lock/order/path 117,
private-read-lock 6, resource 318, process 40, goroutine 128, and Skywalking 2.
Skywalking is a clean checkout pinned at
`e83d5925500a7e63dd55c080a9b1542d6cedaefb`, scope `./pkg/tools/buffer`.
The companion TSV records terminal statuses/counts/pin; detailed scopes and
binary receipts live in `.build/goal-aggregate-writes/scans.json`, with complete
payload equality in `comparison.json`. All checks are enabled. No candidate
application, generator or test execution was performed.

## Validation and remaining work

Stable `make verify VERIFY_TIMINGS=1` is terminal exit 0, including all canonical
gates and the full ordinary test suite (89 seconds), recorded in
`.build/goal-aggregate-writes-verify.log`. The final architecture check passed with terminal exit 0 (`.build/goal-aggregate-writes-architecture-final.log`).
No full
precision corpus replay or local race run is performed. No recorded production
FP correction is credited: seven production sites plus Rune and broader
architecture consolidation remain open. Graph MCP tools are unavailable;
discovery and verification use bounded exact-source fallback. This correction
verifies the named aggregate and summary storage boundary, not the absence of
other partial duplication or proof gaps.

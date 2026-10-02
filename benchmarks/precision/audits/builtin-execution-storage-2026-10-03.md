# Builtin execution and storage invalidation review

Beads: `gohawk-dho.23.9`. Parent: `2532d36`.

## Reproduction and correction

Compiled SSA controls reproduce eight parent failures: deferred copy, direct
and deferred clear, async copy and clear, conditional copy and clear, and
repeated deferred copy. The parent skips builtins before defer registration
checks and applies builtins synchronously before considering launched calls.
Clear also has no storage effect. These paths preserve stale exact contents
or incorrectly retain exclusive storage after asynchronous use.

Exactly registered deferred builtins now consume the ordinary known-call
dispatcher at RunDefers, using arguments evaluated at registration. Conditional
or repeated registration remains uncertain. Async builtin calls take the
existing unresolved asynchronous exposure path. Synchronous clear forgets the
selected collection through the existing heap-summary invalidation mechanics,
without exposing it or recursively modifying its former pointees. Copy keeps
its existing possible-element writes, now also at deferred execution.

Deletion was reviewed separately: map lookup already carries an opaque
possibility and map contents are possible retained history. This change does
not infer exact deletion or cleanup from it. Clear establishes no exact zero
value or slice extent. No lifecycle completion or cleanup facts are invented.
The language contracts are [defer execution](https://go.dev/ref/spec#Defer_statements),
[copy](https://go.dev/ref/spec#Appending_to_and_copying_slices), and
[clear](https://go.dev/ref/spec#Clear).

## Aggregate and responsibility consolidation

A whole-aggregate control then found stale evidence after selected invalidation.
`forgetSlots` now drops cached whole values above the selected slot. Possible element stores also drop cached whole values above their destination,
so copy/append/map effects cannot restore a stale aggregate. Subtree
copies also carry unknown-write stamps, preventing an older backing copy from
restoring a field that was invalidated before the new snapshot. Earlier
snapshots, untouched sibling fields and former pointee storage stay intact.

Backing identities and stamps share one slot-metadata rebasing helper. Contents
retain their detached pointee-set copy and history recording. This avoids the
repeated rebasing loop identified by canonical lint while keeping each state's
copy contract explicit. Builtin effects have one focused owner in
`store_builtin_calls.go`; call/defer orchestration no longer contains that
separate language-contract implementation. The existing effects-file outlier
shrinks from 455 to 418 lines and remains cohesive around uncertain call effects.
No new exported query or fact schema is added.

## Behavioral controls and counterfactuals

`store_builtin_calls_test.go` covers direct/deferred/async and uncertain
registration, captured slice arguments, selected sibling preservation, former
pointee preservation, and before/after aggregate snapshots for clear and copy.
The copy-aggregate intermediate failure is recorded separately in
`.build/goal-builtin-copy-aggregate-parent.log`; it prompted the final common
weak-store correction rather than an exception in copy. Parent reproduction
is `.build/goal-builtin-parent-controls.log`; current heap controls and actual
SSA are `.build/goal-builtin-complete-heap.log` and
`.build/goal-builtin-complete-ssa.log`.

Seven final-source overlays target missing clear effects, skipped deferred
builtins, async synchronous treatment, retained whole-aggregate values, lost
unknown stamps, recursive clear invalidation, and retained aggregates after
possible element writes. All seven final-source overlays are terminal exit 1
with behavioral assertion failures, recorded in
`.build/goal-builtin-complete-mutants/results.json` and individual logs. An earlier prototype overlay
for missing stamps initially failed compilation due to an unused local; its
corrected assertion failure is retained separately and is not used as final
source evidence. Production source was never temporarily mutated for these
counterfactuals.

## Scoped comparison

Parent binary: `.build/goal-deferred-contract-reviewed`, SHA-256
`9cbfe607f32479a1d3dfc9a8df9819a2d8c8a92ae7fa0a59d0fe96045df74d20`.
Its six successful current receipts from the previous review are reused as
parents with identical scopes/pin. Final current binary is
`.build/goal-builtin-complete-reviewed`, SHA-256
`6a0d5b3bef10c276eaa136642fd9f99a041a90a529bc0f34c737b116272bb9ae`.
The two earlier prototype executables remain separate and are not final
comparison evidence after the weak-element correction.

All twelve final parent/current receipts are terminal exit 0 with empty
stderr. Complete merged diagnostic payloads are identical: lock/order/path 117,
private-read-lock 6, resource 318, process 40, goroutine 128, and Skywalking 2.
Skywalking is a clean checkout pinned at
`e83d5925500a7e63dd55c080a9b1542d6cedaefb`, scope `./pkg/tools/buffer`.
The companion TSV records terminal statuses/counts/pin; detailed scope and
binary receipts live in `.build/goal-builtin-complete/scans.json`, with complete
payload equality in `comparison.json`. All checks are enabled. This is not a
full precision replay; candidate tests, applications and generators were not
executed.

## Validation and outstanding scope

Initial canonical lint identified duplicate rebasing loops and an unconditional
one-return test loop. The shared metadata helper and explicit compiled return
assertion correct those issues. Stable final `make verify VERIFY_TIMINGS=1`
is terminal exit 0, including all canonical gates and the full ordinary suite
(117 seconds). Final architecture is terminal exit 0. Receipts are
`.build/goal-builtin-complete-verify.log` and
`.build/goal-builtin-complete-architecture.log`. Whole heapmodel controls pass
in `.build/goal-builtin-complete-heap.log`. No local race or full precision
corpus run was performed.

No recorded production FP correction is credited. Seven production sites plus
Rune and the broader consolidation requirements remain open. Graph MCP tools
are unavailable; discovery and verification used bounded exact-source fallback.

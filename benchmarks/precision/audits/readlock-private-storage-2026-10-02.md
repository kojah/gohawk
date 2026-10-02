# Read-lock writes into private storage

Beads `gohawk-dho.23.7`; parent source `e63563c`.

## Reassessment and correction

The two pinned Skywalking cursor writes still need an exact field/participant
relation. An unlocked cursor assignment elsewhere does not prove confinement,
and a shared receiver does not establish that every field has the same guard.
No framework, cursor-name or fresh-lock publication exemption is added.

A separate, reproducible precision gap is bounded: the parent reports five
mutations into private storage in `privateread`. Two scalar writes are local or
before publication; three container mutations use fresh local backing storage.
One destination selector now supplies both owner matching and the existing heap
exclusivity query. A proven local destination excludes the diagnostic with
accepted reason `private-write-storage`. A fresh wrapper containing a borrowed
map or slice does not qualify; its backing storage belongs to the caller.

## Shared evidence

The parent actual SSA/heap dump shows standard `RWMutex.RLock` escaping its
receiver because no body/summary is available. The graph's known-call boundary
now models exact direct synchronous `sync.Mutex` and `sync.RWMutex` locking,
unlocking and try methods as nonretaining mutation of the mutex receiver.
It reuses summary substitution's exact slot invalidation, preserving sibling
storage and existing exposure. It does not infer lock success or completion.
Launched calls, interface dispatch, `RLocker`, project-defined lookalikes and
unknown receiver selections retain conservative effects. The contract follows
the standard implementations in `sync/mutex.go` and `sync/rwmutex.go`, inspected
in the installed Go toolchain and linked at the decision point.

`store_exclusivity.go` owns whole-object exclusivity. One non-stale object may
have an unknown selected index while exact content identity remains unavailable.
`singleSlot` retains its exact selection boundary. Fresh language `make` maps,
slices and channels can be local despite opaque element contents; opaque calls,
unknown/mixed/stale identities and any recorded exposure remain unavailable.
The exposure test applies to every region kind, including caller collections
whose individual elements escaped. The graph's general allocation/content model
is unchanged; no new storage or guard inference engine is introduced.

This query is also consumed by lock caller/acquisition exclusivity and indirect
resource-store classification. Those consumers keep their policies. The generated
helper reference follows the moved exported type. Heap graph, symbol and type
query costs retain their independently owned boundaries; no performance
percentage or whole-program bound is claimed.

## Controls and validation

The parent fixture run fails with five unexpected diagnostics. The current
fixture retains all six published-owner, caller-owner and borrowed map/slice
reports while removing those five false alerts. Heap controls cover all nine
standard method identities, async/adapter/lookalike calls, prior publication,
fresh/borrowed/opaque/mixed maps and slices, private/published/async/borrowed
channels, loop maps and element exposure for both fresh and caller slices.
Actual SSA and heap dumps ground the receiver and destination identities.

Six isolated overlay counterfactuals fail assertions: removing the private
exclusion; treating a fresh wrapper as private backing storage; invalidating the
whole wrapper at a mutex call; treating an async mutex call as synchronous;
requiring exact element identity for object exclusivity; and ignoring selected
storage exposure. The final controls and mutants use the final source.

The [ledger](readlock-private-storage-2026-10-02.tsv) records terminal scoped
parent/current all-check results. Comparisons merge package JSON objects before
comparing complete payloads, because independent package output order can vary.
Existing lock/order/path, resource, process and goroutine fixtures are compared;
private fixture removals are checked individually. Clean pinned Skywalking
`e83d5925500a7e63dd55c080a9b1542d6cedaefb` `./pkg/tools/buffer` remains in the
comparison. An initially misspelled goroutine fixture scope failed loading and
is superseded by the corrected `closurechoices` receipts; it is not evidence
of silence. Initial prototype receipts are superseded by the final binary after
the selected-exposure tightening.

Parent binary SHA-256:
`1b68d2f2af3677175d2a963b0e10d32ff283c0aca63570fe82f28dad09c02d7a`.
Final binary SHA-256:
`b33072d7dae095c919081b92999e2d2299aa603364b21ebc49be377d48fa87b9`.

Receipts: `.build/goal-readlock-private-parent-control.log`,
`.build/goal-readlock-private-parent-heap.txt`,
`.build/goal-readlock-private-current-heap.txt`,
`.build/goal-readlock-private-final-controls.log`,
`.build/goal-readlock-private-final-mutants/results.json`,
`.build/goal-readlock-private-final/scans.json`,
`.build/goal-readlock-private-final-traced.json`,
`.build/goal-readlock-private-final.trace.jsonl`,
`.build/goal-readlock-private-verify-final.log` and
`.build/goal-readlock-private-architecture-final.log`.
Final scoped comparisons retain complete payloads for 117 existing lock/order/
path findings, 318 resource findings, 40 process findings, 128 goroutine findings
and both Skywalking sites. The private fixture changes from 11 findings to six,
removing exactly the five accepted private forms. The traced invocation has
empty stderr and identical merged diagnostic JSON. Its 3,643 events include
accepted, rejected and unknown decisions, with exactly five candidate-attributed
accepted `private-write-storage` decisions. Final canonical validation passes all
eight targets; final architecture passes.
Graph MCP tools were unavailable; exact source and compiled SSA supply the
evidence. No full precision replay or local race run was used.

This corrects a reproduced fixture FP class, not a recorded production audit
site. Seven recorded sites plus Rune and the broader completion audit remain
open.

# Lock-state traversal allowance

Beads `gohawk-dho.44.11.5.26` consolidates lock work-list expansion under one
function traversal allowance. This earns no production FP correction. The
bounded batches 62/63 production queue still has eleven unresolved locations;
Rune's fresh-lock publication issue remains separate.

## Evidence boundary

The previous walk limited distinct states to 4,096 but ran its keys, copies,
phi selection and guard/cycle queries outside that limit. Successor result
inference and termination inference also owned separate query allowances.

`flow_walk.go` now retains that state cap and shares a 200,000-step traversal
allowance across queued visits, state metadata, instruction/effect visits,
phi selection, guard/cycle queries, successor selection and result-backed
termination. `state_copy.go` detaches the mutable lock collections before
transfer. The successor query consumes the same state rather than receiving
separate copies of its collection arguments. One expansion/transfer path owns
both direct operations and summarized helper effects.

Cutoff makes the whole function unavailable. Buffered diagnostics and order
edges remain private until expansion finishes. Incomplete keys cannot enter
the work list, and an interrupted cycle query cannot prove a computed Boolean
stable. A fresh query can retry using the same evidence engines.

The existing cycle query is exposed as `ssaflow.BlockInCycleWithin`; no second
CFG walker was added. After migration, `PathGuards.Key` and `Extend` had only
test callers. They were removed and those tests use the same `Within` engines
with nil allowances. The generated shared-helper inventory follows those API
changes.

This is not a whole-query time bound. Prewalk effect/caller/defer setup,
helper completion, alias/exclusivity searches, final contract metadata and
order publication retain independent costs. Sorting, string formatting and
type/graph internals are not bounded wall time by per-entry charging. Beads
`gohawk-dho.44.11.5.27` owns the next setup/completion boundary review.

## Focused controls

Actual SSA tests cover a read-lock write and order edge before a padded tail,
all cutoffs through traversal completion, child versus parent exhaustion,
fresh retries, stable branch cleanup, detached state mutation, phi refresh,
acyclic versus repeated computed conditions, and cold/warm termination inference.
A padded result helper isolates nested summary spending from ordinary branch
setup. Existing lock fixtures retain the default semantics.

Ignored source overlays fail behaviorally when they bypass:

| Counterfactual | Failure |
| --- | --- |
| State key allowance | A partial key is exposed. |
| State copy allowance | A partial copy is exposed. |
| Successor result allowance | One successor is returned without caller exhaustion. |
| Termination result allowance | The interrupted caller is reported complete. |
| Diagnostic publication barrier | A read-lock-write diagnostic escapes at cutoff 11. |
| Order publication barrier | An order edge escapes at cutoff 21. |

Receipts are `.build/goal-lock-state-overlays/*-final.log`. These are intended
test failures, not compiler errors. No candidate tests, generators or
applications were run in the production checkouts.

## Scoped production receipts

The immutable executable `.build/goal-lock-state-reviewed-final` implements
parent `679af13` plus this production change, SHA-256
`99744c3f5959837f5fd179409f0ce0474b375bf0a06dd18119ffaa029944eed4`.
The parent control is `.build/goal-cancel-owner-reviewed`, SHA-256
`fa000fc9e57d70041a127cf8001e43912f568203fdf1e8fa44fea5288758dadd`.
Both checkouts were clean at the recorded pins. Scans use
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, a 180-second timeout,
and `go vet -vettool=/absolute/binary -enable-all -json` on these scopes:

| Repository and pin | Scope | Retained reviewed reports |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Missing releases at `configparser.go:115:4` and `121:3`. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Read-lock writes at `datastore.go:542:5` and `file_index.go:708:4`. |

Each scan exits zero with empty stderr. Corrected JSON is byte-identical to
the parent: 838 bytes for XD and 1,476 for goiardi. Receipts are
`.build/goal-lock-state-{xd,goiardi}-{parent,reviewed-final}.json` and `.err`.
The corrected goiardi scan also enables lock tracing to a separate file;
its JSON equality proves tracing preserves these diagnostic controls; all
13,743 emitted JSONL events decode successfully.

The local gate `make verify VERIFY_TIMINGS=1` passes, receipt
`.build/goal-lock-state-verify-reviewed.log`. The focused documentation gate
also passes (`.build/goal-lock-state-docs.log`). No local race test or full
precision-regression corpus runs. This scoped stage does not prove that the
whole architecture is consolidated or that remaining easy FPs are exhausted.

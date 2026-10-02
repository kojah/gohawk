# Lock release query consolidation

Beads `gohawk-dho.44.11.5.27.1` consolidates the five release searches found in
the lock-flow setup review: synchronous exact completion, synchronous possible
completion, spawned exact completion, registered possible defer completion and
possible defer completion registered before acquisition. No production FP
removal is credited. The eleven recorded unresolved production locations and
Rune's fresh-lock publication issue remain open.

## One request owner

`release_queries.go` owns these requests, their launch/coverage policy and
availability. All five reuse the existing lifecycle completion engine with
`Unlock`/`RUnlock`; there is no second callee walker. Each question retains its
250,000-step cap and charges the same function pool as state traversal.
Dispatch is charged before the evidence cache is consulted. Identity snapshots,
candidate/defer/value visits and pre-acquisition dominance charge that pool too.
Synchronous release searches accept only synchronous call instructions; scalar
instructions, `go` and `defer` have no synchronous completion proof to request.

A cutoff at the function or question boundary records unavailable evidence.
It never counts as an unlock. The work list uses one interruption barrier for
expansion and publication, including a question's own cap when the function
pool still has capacity. Interrupted functions publish neither their buffered
diagnostics nor their staged order edges. Fresh queries can reuse the evidence
engine because interrupted completion answers are not retained as callee facts.

Exact and possible completion retain their separate coverage and outer launch
reasons. An opaque callback remains unknown; a helper that conditionally
unlocks can supply possible release but cannot supply all-return release.
The existing comments and pinned examples moved with their policy.

Prewalk summaries and caller/defer inventories, final return/contract metadata,
callee ordering and alias/exclusivity internals remain distinct work in parent
`gohawk-dho.44.11.5.27`. Type/heap graph costs are separately recorded. This
stage is not a whole-query wall-time guarantee or a completed architecture audit.
Graph MCP tools were unavailable; discovery and verification used scoped source
reads and searches of the lock analyzer and existing lifecycle APIs.

## Controls

`release_queries_test.go` uses actual SSA for exact versus possible coverage,
all three launch forms, opaque callbacks, every cold cutoff through completion,
fresh retries on the same evidence, warm-cache zero allowance, an independent
question cutoff and a late release-search cutoff after findings/order edges.
The late-flow control leaves result summaries unavailable to isolate release
inference from termination-result inference. Existing analyzer fixtures pass.

Four ignored source overlays fail behaviorally:

| Counterfactual | Failure |
| --- | --- |
| Detach completion from the function pool | The cut flow completes and publishes a read-lock-write finding plus an order edge. |
| Drop the unavailable completion marker | A locally capped question fails to mark the function interrupted while its pool remains usable. |
| Treat budget exhaustion as settled release | The unknown completion passes the release projection. |
| Skip dispatch charging before the cache | A warm cleanup proof bypasses a zero allowance. |

The receipts are `.build/goal-lock-release-overlays/{pool,availability,settled,dispatch}.log`.
Each is an intended test failure, not a build failure.

## Validation and scoped replay

`make verify VERIFY_TIMINGS=1` passes, including ordinary tests, formatting,
vet, lint, deadcode and self-dogfood. The final receipt is
`.build/goal-lock-release-verify-final.log`; the focused release receipt is
`.build/goal-lock-release-focused-final.log`. No local race test or full
precision-regression corpus runs.

The immutable `.build/goal-lock-release-reviewed-final` implements parent
`241397f` plus this production change; SHA-256
`8a2bd36bed4140b107ec66078e6fdb534f3a0179316316970de5fa054a1cebf1`.
The parent binary and receipts are recorded by the
[lock-state follow-up](lock-state-allowance-followup-2026-10-02.md).
The same clean pinned checkouts use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`,
`GOWORK=off`, a 180-second timeout and `go vet -vettool=/absolute/binary
-enable-all -json` on these scopes:

| Repository and pin | Scope | Reviewed controls retained |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Missing releases at `configparser.go:115:4` and `121:3`. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Read-lock writes at `datastore.go:542:5` and `file_index.go:708:4`. |

Both scans exit zero with empty stderr. Diagnostic JSON remains byte-identical
to the parent: 838 bytes for XD, 1,476 for goiardi. Receipts are
`.build/goal-lock-release-{xd,goiardi}-final.json` and `.err`. The goiardi scan
also enables lock tracing to a separate file: all 29,936 JSONL events decode,
and tracing leaves diagnostic JSON unchanged. Candidate tests, generators and
applications were not executed in these production checkouts.

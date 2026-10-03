# Caller exclusivity selection and cache ownership

Bead: `gohawk-dho.44.11.5.27.28`. Parent source: `cfa72cc`.

## Source evidence and decision

The shared caller census already handles private-function completeness,
initialization, caller limits and escaped/asynchronous uses. Its exclusivity
consumer rescanned every caller and cached a Boolean independently of the lock
request. The baseline actual-SSA test shows the old API returns positive for two
fresh callers without consuming an enclosing zero allowance. That API had no
allowance parameter; the baseline is evidence of the missing request connection,
not an injected cutoff inside heap graph execution.

Parameter selection now returns one structured exclusivity proof, charging
admission and each caller visit. Completed unknown and positive answers may be
cached; cutoff never enters the cache. Cache hits require live admission and
fresh children can retry an interrupted query. Acquisition selection shares the
same allowance; held-state transfer stops order recording at cutoff, and the
existing outer walk discards staged findings and edges.

The accepted-exclusivity trace consumes the returned proof. Its parameter and
caller details are built only when tracing is enabled. Complete policy remains
unchanged: every caller must supply an unescaped fresh local, while shared,
exported, method, escaped callback and incomplete caller evidence remains
unknown. Locals published later keep their existing acceptance; locals never
published retain the existing ordering policy.

## Controls

`exclusive_budget_test.go` logs actual SSA for fresh, shared and callback
patterns. Caller and acquisition queries sweep allowances through first
completion, reject interrupted cache entries, retain completed proofs, and
recover with fresh children without exhausting the outer pool. Warm cache
controls require live admission but avoid rescanning callers. Existing
`exclusive_callers_test.go` preserves shared direct/Go/Defer, initialization,
method and caller-count boundaries. Existing `exclusive_owners.go` accepted and
diagnostic fixtures preserve the publication/order policy.

The initial canonical run failed on nested flow blocks while tests passed.
Flattening the new-acquisition branch passes lint. A second canonical run passed
before trace allocation was cleaned up. Final-source
`make verify VERIFY_TIMINGS=1` passes all eight gates: generate 4s, modules
1s, vet 3s, formatting 5s, deadcode 12s, lint 14s, dogfood 36s and tests 69s.
No lint threshold was raised.

## Scope and limits

`heapmodel.ExclusiveAt` owns cached graph construction, state replay, object
selection and publication reachability. A charged caller visit is not a
transitive bound on these graph-internal queries. That separation is explicit;
this change does not invent a second graph budget or alter heap semantics.
Graph tools were unavailable; review uses exact source and actual SSA.

No full precision replay, local race run, FP credit, parent closure or overall
goal completion is claimed. Five production sites and broader semantic
consolidation remain open.

## Final diagnostic and trace compatibility

Ten final-source parent/current read-only all-check invocations exit zero with
empty stderr. Complete diagnostic JSON matches: lock fixtures 116 findings,
goroutine fixtures 128, Rune 0, Skywalking 0 and stargz 1. Production scopes
use clean pinned checkouts and read-only modules; Rune enables CGO. No external
repository tests, applications or generators were run. Final receipts retain
pins, package scopes and hashes under `.build/goal-exclusive-final-scans`.

Two targeted traced lock-fixture scans select `exclusive_owners.go`; all 102
records preserve complete contents and per-candidate order. Traced diagnostic
payloads equal untraced ones. This covers the existing exclusivity boundary;
it is not a comparison of every historical trace or every cutoff event.

Reviewed/canonical SHA-256:
`85a1bf572e5f0ce210f5428f1dabaf4b79f940cd43a11ed4907582c20528ba90`.
Parent SHA-256:
`fc6724eeb2c47d47a507453f29b38dd2700e3cc8e45f4402ea2fb6fca9ca6828`.
The earlier pre-trace-cleanup scan also preserved all five payloads, but final
compatibility claims use the final frozen binary above.

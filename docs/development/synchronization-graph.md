---
title: Synchronization graph
description: The compositional event model for bounded cross-goroutine proofs.
sidebar:
  order: 7
---

The synchronization graph is a shared model of synchronization **events** and
their ordering, not a whole-program schedule or a deadlock verdict. An analyzer
owns the proof and reporting policy for its check. A graph cycle is a candidate
that needs a separate feasibility argument before it can support a diagnostic.

## Current foundation

`concurrencyfacts` already composes complete, bounded ordered effects for
channels, `sync.Mutex`, `sync.WaitGroup`, and bounded context cancellation. `summaries.Provider` selects that
component and binds a helper's symbolic parameters to exact caller objects.
Complete empty effects are distinct from an unavailable summary. Imported facts
carry parameter-relative effects, never process-local SSA values or source
positions.

## Cancellation signals

An exact `context.WithCancel` or `WithCancelCause` call with a proven
`Background`/`TODO` parent identifies a fresh cancellation signal. The returned
context, its cancel function, and receives from its `Done()` channel share that
identity. Ordinary channels remain distinct from cancellation signals. Stable
closure captures and helper arguments use the existing storage and binding
machinery; mutation or opaque identity leaves the model unknown.

`Cancel` is a request event, **not** `Close` and **not** a worker join.
`SyncGraph.Cancellations` relates each request to receives from the exact signal
in that graph variant. These relationships are separate from ordering edges:
they do not establish which cancellation enabled a receive, whether a select
chooses it, or whether the worker completes. The
[context contract](https://pkg.go.dev/context#Context) permits asynchronous
Done closure, and [CancelFunc](https://pkg.go.dev/context#CancelFunc) does not
wait for work to stop. No cancellation relationship enters cycle detection.

Helper summaries carry `CancellationInputs` as explicit binding requirements,
including when a helper merely calls `Done()` and discards the result. A custom
`Context` method or an arbitrary function converted to `CancelFunc` can hide
other effects. Consequently a symbolic requirement is not a complete summary
for analyzer consumption. It may compose and publish parameter-relative facts,
but only an exact binding discharges it. Both linear graph construction and
select expansion enforce this boundary. Cross-package forwarding preserves
these requirements, ordered requests, receives, and child launches.

The first implementation deliberately leaves parent cancellation propagation,
deadlines/timeouts, `WithoutCancel`, `WithValue`, `AfterFunc`, factory-returned
contexts, nested selects, and looped workers unknown. In particular, it does
not infer a missing cancel or a worker leak from absence of a modeled request.
Those need their own obligation proofs. Cancellation arms remain alternatives;
this change does not introduce a new diagnostic or expand deadlock feasibility
proofs.

## Selects and child goroutines

One acyclic worker `select` can yield a bounded set of complete continuations.
`syncgraph.Expand` builds a separate linear graph for each outcome, up to eight
graphs; a consumer must prove its property on every one. The ordinary
`FromSummary` contract remains incomplete for a choice, so linear consumers
cannot accidentally treat its arms as simultaneous events. Only the channel
dependency-cycle check currently consumes these alternatives. Launches hidden
inside helpers, nested selects, unrelated branch conditions, and cross-package
select continuations remain unknown. Complete straight-line helper launches
can compose into a caller, including through parameter-relative imported
facts. A launch within a worker remains unknown.

The experimental lock-and-join and channel/lock-cycle proofs consume a
parent/children fragment through `internal/syncgraph`. The fragment has
`SyncEvent` nodes and separate program-order, spawn-order, and proven blocking
dependency edges. A root summary tracks up to four statically known
children, each with its own identity, spawn point, and ordered effects. Both
checks require a fresh local mutex and channel, and a complete sequence in
which every child able to signal the waited-for channel must first acquire the
mutex held by the parent. A child that could release that mutex or signal
without acquiring it makes the proof inconclusive. The channel/lock check
additionally requires a statically unbuffered channel. A fifth launch,
divergent effects, loops, launches outside the lock-to-wait interval, and
opaque calls remain inconclusive. A helper call counts as a launch only when
its complete summary proves an exact child template.

## Intended graph contract

The graph's event types are `SyncGraph` and `SyncEvent`. An event retains its
operation kind, bound resource identity, goroutine identity, and source/call-site
provenance. Completeness belongs to the graph, inherited from the summary;
an incomplete summary yields no usable event nodes. Edges distinguish program
order from blocking dependencies. Branch alternatives and loop multiplicity
must not be silently flattened into unconditional order.

An event is an operation milestone, not separate start and completion nodes.
A blocking edge says what is required to complete an operation; two sides of
an unbuffered channel handshake may complete together. The initial graph is
therefore a bounded dependency model, not a general happens-before graph.

Construct function fragments once, bind them at calls through the existing
summary broker, and inspect only fragments relevant to a candidate. A package
fact publishes only complete, exportable guarantees. It must never interpret
missing effects as proof of no effect. An unbounded launch, dynamic dispatch,
escaped channel or mutex, unresolved alias, and budget exhaustion remain
unknown unless a narrower structural proof independently accounts for them.

## Shared semantic queries

`syncgraph.NewQuery` snapshots one complete graph variant. Incomplete summaries,
unexpanded choices, malformed event identities, and unknown launch points yield
unknown answers, never a proof that a participant or effect is absent.

- `Before` establishes program/spawn order, conditional on reaching the later
  event. It does not interpret consumer-added blocking dependencies as strict
  execution order, or a missing ordering path as proof of concurrency.
- `FirstSignalAfterAcquire` identifies a goroutine's first send or close on an
  exact channel and a preceding exact mutex acquisition. It rejects prefixes
  that might release another goroutine's lock. It proves neither that the
  worker completes nor that the mutex remains held at the signal. The
  lock-and-join and channel/lock checks use this shared evidence and retain
  their own capacity, parent-lock, signal-kind, and participant policies.
- `CancellationObservations` enumerates receives on the cancel request's exact
  Done identity within that variant. Matching an observation is neither an
  ordering edge nor a join. An exhaustive list of modeled observations is not
  proof that all external participants have been modeled.

Results carry a structured proof with a stable reason and, where applicable,
event witnesses. Query construction is linear in event count; `Before` and
cancellation lookup are constant-time in the current root/children model, and
the signal-prefix query scans one bounded goroutine sequence. Cancellation
observation slices are immutable and shared rather than copied per request.

`LockRegion` remains the ordered-prefix lock-state query. `Expand` keeps select
alternatives separate; consumers still need a proof for every returned variant.
The query layer does not infer missing effects, solve arbitrary channel partner
matching, or establish a deadlock by itself.

## Rollout boundary

The two exact lock/signal proofs share the graph and one bounded root-summary
query. Their first stage filters for likely candidates, so ordinary functions
do not pay for graph construction. The root summary copies at most four child
sequences and 32 events total; graph construction and cycle detection are
linear in those bounded events and edges. A graph cycle alone cannot justify a
diagnostic: each consumer first proves the exact identity, ordering, and
unavoidable blocking semantics of its two dependency edges. Candidate
enumeration and each proof must stay bounded. The initial model has no SMT
solver or path-enumeration requirement. If those are ever added, they are
optional refinements and cannot upgrade an incomplete model to a proven
diagnostic.

## Initial cost check

With only `lockorder` selected, two before and two updated runs on
`golang.org/x/tools@v0.49.0` (`./go/...`, 109 target packages and 493 timed
packages including dependencies) took 9.81–10.04 versus 10.12–11.26 seconds
wall time. Summed `lockorder` time in the x/tools packages was
1.335–1.416 versus 1.320–1.379 seconds, with about 408–410 MiB allocated on
either side. The larger wall-time spread came mostly from `net/http`; these
few runs do not establish a stable regression or measure peak memory. The
graph is constructed only after the cheap local candidate filter finds a
launch, a local channel, a mutex acquisition, and a receive. This historical
cost check predates multi-child support; recheck it before broadening further.

## Kubernetes and Moby coverage snapshot

On 2026-09-24, analyzer commit `17a1d9d` (after cancellation modeling, before
the query-layer refactor) ran the four experimental dependency checks against
Kubernetes `e72c2715ade37738aa5c029e8de5285cbe1c9441` and Moby
`3f673306102e01c16b5e0ab2343588bfab2fc4e7`. Root-module package inventories
contained 1,472 and 347 packages respectively; test source was included.
No selected check reported a diagnostic. Every traced candidate surviving the
initial filters stopped at incomplete protocol evidence, before cycle proving.

| Candidate decision | Moby | Kubernetes |
| --- | ---: | ---: |
| Channel dependency: fresh-channel filter | 344 | 430 |
| Channel dependency: launch filter | 54 | 104 |
| Channel dependency: incomplete summary | 45 | 27 |
| Lock-and-join: incomplete summary | 5 | 5 |
| Channel/lock cycle: incomplete summary | 5 | 5 |
| WaitGroup/lock cycle: incomplete summary | 2 | 7 |

Counts exclude vendor/staging dependencies and deduplicate repeated test
variants by check, source candidate, reason, and outcome. The two lock/signal
checks inspect the same five sites in each repository; these are not ten
distinct bugs. For channel summaries, control flow accounted for 24 Moby and
14 Kubernetes cutoffs; the remainder were unavailable bodies, effects, or loads.

Actual SSA inspection confirmed representative boundaries:

- [Moby's health monitor](https://github.com/moby/moby/blob/3f673306102e01c16b5e0ab2343588bfab2fc4e7/daemon/health.go#L256)
  combines loops, nested selects, a buffered result channel, and an interface
  probe. Cancellation is followed by a separate result receive. The first
  cutoff is control flow, not cycle feasibility.
- [Kubernetes monitor startup](https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/controller/garbagecollector/graph_builder.go#L295)
  waits on a receiver-owned startup channel under an RWMutex, then launches a
  dynamic set of monitors. Beyond the control-flow cutoff, a proof would still
  need the external sender's identity and dependencies. This is a modeling
  target, not evidence of a deadlock.

Kubernetes completed in 4m23.50s. Moby took 2m35.01s but had CGO-disabled
test-loading failures in btrfs, quota, and volume/local, so its scan is partial.
Parallelism differed and another validation overlapped the Moby run; these are
not before/after performance measurements. Temporary clones were deleted;
local traces, timings, SSA dumps, and a detailed report were retained under
`.build/sync-recall-2026-09-24`.

The next coverage work should improve resource-specific effect slices,
bounded alternatives, and cross-method participant/receiver identity while
preserving opaque effects as unknown. This sample supplies no evidence that
SMT feasibility solving is the immediate bottleneck. Shared queries organize
available evidence; they cannot reconstruct effects discarded by a summary.

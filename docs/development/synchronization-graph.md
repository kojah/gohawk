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
channels, `sync.Mutex`, `sync.RWMutex`, `sync.WaitGroup`, and bounded context cancellation. `summaries.Provider` selects that
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
contexts, nested selects, and open-ended worker loops unknown. In particular, it does
not infer a missing cancel or a worker leak from absence of a modeled request.
Those need their own obligation proofs. Cancellation arms remain alternatives;
this change does not introduce a new diagnostic or expand deadlock feasibility
proofs.

## Selects and child goroutines

One acyclic worker `select` can yield a bounded set of complete continuations.
`syncmodel.Expand` builds a separate linear graph for each outcome, up to eight
graphs; a consumer must prove its property on every one. The ordinary
`FromSummary` contract remains incomplete for a choice, so linear consumers
cannot accidentally treat its arms as simultaneous events. The lock-and-join,
channel/lock, WaitGroup/lock, and channel dependency checks consume bounded
alternatives. Ordinary acyclic branches can retain up to eight separate paths,
including an empty escape path. Branch correlations are over-approximated,
not solved; all paths must establish the same reported parent wait. Worker
alternatives survive local helper launch/forwarding calls. Nested selects,
independent branches within a select continuation, and cross-package
alternative publication remain unknown. Complete straight-line helper launches
can compose into a caller, including through parameter-relative imported
facts. A launch within a worker remains unknown.

The experimental lock-and-join and channel/lock-cycle proofs consume a
parent/children fragment through `internal/syncmodel`. The fragment has
`SyncEvent` nodes and separate program-order, spawn-order, and proven blocking
dependency edges. A root summary tracks up to four statically known
children, each with its own identity, spawn point, and ordered effects. Both
checks require a fresh local mutex and channel, and a complete sequence in
which every child able to signal the waited-for channel must first acquire the
mutex held by the parent. A child that could release that mutex or signal
without acquiring it makes the proof inconclusive. The channel/lock check
additionally requires a statically unbuffered channel. A fifth launch,
unproven alternatives, unbounded loops, launches outside the lock-to-wait interval, and
opaque calls remain inconclusive. A helper call counts as a launch only when
its complete summary proves an exact child template.

`RWMutex` read and exclusive acquisitions remain distinct. An exclusive holder
blocks both modes; a read holder blocks a writer, not another reader. This does
not model queued-writer recursive reading. The generic exclusive `LockRegion`
query declines read-mode effects instead of crediting them as a write guard.

Embedded locks beneath a fresh receiver can participate in these proofs.
Declaration templates carry an exact parameter-relative field through methods
that do not select it themselves; bound queries materialize that field to an
existing caller address. This supports helper-launched receiver workers without
inventing SSA values. It does not infer an arbitrary Start/Stop call ordering,
enumerate callers, or close the participant set of externally owned receivers.
The receive may itself be inside a visible helper. The caller must still
establish the acquisition before the launch and preserve it through that wait.

Channel fields on a fresh receiver can also bind across visible launch and
wait methods. `heapmodel.Storage.StableFieldContent` requires one exact value
and no subsequent replacement, including through helpers or asynchronous
workers. Its field-specific effect query can ignore writes to a disjoint sibling
field without claiming the whole helper is pure. Whole-owner publication,
opaque consumers, recursive effects, and mutable channel slots remain unknown.
These local loaded-field relations are not yet exported across packages.

## Shared loop and dispatch evidence

`ssaflow.ProveCountedLoop` recognizes an exact zero-based unit-step loop with a
literal bound and one body block. It returns control-flow evidence with an enum
reason, not synchronization policy or a termination guarantee. The concurrency
consumer permits at most four iterations, rejects counter-dependent effects,
iteration-local allocations and deferred effects, and charges repeated work to
the same budget. Each expanded launch remains a distinct child. The WaitGroup
proof permits registration interleaved with launches only when every child is
already counted and the parent already holds its lock.

`ssaflow.ResolveInterfaceDispatch` resolves a concrete interface box (or
agreeing receiver alternatives) to its actual method. Concurrency composition
then applies the usual method contract or complete summary with the unboxed
receiver in argument position zero. Merely finding a matching method name or
one possible implementation is insufficient. Different receivers, unresolved
interface parameters, and mutable interface storage remain unknown.

This is not yet arbitrary resource-specific protocol extraction: an opaque call
still cannot be erased just because no synchronization effect was recorded.
Field-stability evidence describes a slot, not whether a helper can block or
return normally. General receive/select loop contracts, different concrete
dispatch alternatives, and externally owned participant sets remain future work.

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

`syncmodel.NewQuery` snapshots one complete graph variant. Incomplete summaries,
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
- `Scope` projects a complete graph onto selected resources, using exact fresh
  paths or positive heap disjointness evidence. Every child is retained and its
  launch prefix is remapped. Uncertain aliases and condition waits prevent the
  projection; it never removes a possible partner or unlock by guessing. Lock
  cycle checks use this to tolerate unrelated synchronization operations.

Scoping is currently **projection after complete extraction**, not recovery of
a partial summary. An opaque helper, loop, or unsupported instruction still
prevents the root proof. Resource-specific extraction through those boundaries
requires a separate no-interference/participant proof and remains follow-up
work; storage preservation alone does not prove a call cannot synchronize.

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
do not pay for graph construction. Each alternative has at most four child
sequences and 32 events; graph construction and cycle detection are
linear in those bounded events and edges. A graph cycle alone cannot justify a
diagnostic: each consumer first proves the exact identity, ordering, and
unavoidable blocking semantics of its two dependency edges. Candidate
enumeration and each proof must stay bounded. The initial model has no SMT
solver. Acyclic path alternatives are capped at eight and share the summary
work budget; exceeding either bound yields unknown. Neither alternative
enumeration nor graph projection upgrades missing effects to proven absence.
Publication uses a separate linear-summary cache: it does not enumerate
alternatives that its fact format cannot export. Richer alternatives are
computed on demand for consumers and cannot reuse a linear cutoff as if it
were a complete answer. Candidate lock/wait pairs are enumerated only within
the bounded event set, with a separate resource projection per pair.

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

### Bounded-model refinement rerun

The same pins and four checks were rerun with the bounded branch, receiver-field,
RWMutex-mode, and resource-projection changes. The frozen intermediate binary
had SHA-256 `40484687a8d8c45fdcdedd641e1255f10cd6cd0bda8c32812a603a2d9fc482ca`;
artifacts remain in `.build/sync-refinement-validated-2026-09-24`.
Subsequent conservative guards reject nested launches in selected workers,
select arms, and deferred helpers; those guards were covered by regression
tests, not this whole-repository scan.

Neither repository produced a selected diagnostic. Channel dependency cutoffs
remained unchanged. Allowing the initial lock/join filter to consider a receive
hidden in a helper increased candidates from 5 to 22 in Moby and from 5 to 37
in Kubernetes, for each of the two lock/signal checks. All still stopped at
incomplete summaries. WaitGroup/lock candidates remained 2 and 7. These counts
use the same root-module and test-variant deduplication rules above; increased
candidates are not increased bug detection.

Kubernetes completed in 4m43.24s with Go 1.26.0 and package parallelism 4.
Moby took 1m48.09s with Go 1.26.6 and parallelism 2, retaining the same three
CGO-disabled test-loading failures. Other validation overlapped these runs,
so the timings do not establish a performance change. The remaining coverage
bottleneck is still obtaining complete resource-specific effects through
loops, opaque calls, and external participants, not graph cycle search.

### Shared-evidence extension rerun

The exact-loop, concrete-dispatch, and stable receiver-channel work committed
as `3f9f59f` was scanned against the same pins and profile. The frozen binary
SHA-256 was `42cac0c18c048cad750b784c994456713544f85048a074212e3cff8f47ff7ea3`;
local artifacts are in `.build/sync-upstream-2026-09-24`. It predates the
defensive rejection of a storage observation that is itself a field write;
the synchronization consumers observe calls, not stores.

Both repositories again produced zero selected diagnostics. Candidate totals
were unchanged: 45/27 channel-summary candidates, 22/37 candidates for each
lock/signal check, and 2/7 WaitGroup candidates (Moby/Kubernetes). All still
stopped at incomplete evidence. Cutoff reasons moved between control flow,
unavailable bodies, and effects; that change in the first observed blocker is
not evidence of improved bug detection.

Moby completed its partial scan in 4m46.75s with the same three CGO-disabled
test-loading errors. Kubernetes completed successfully in 7m32.51s. Compilation
and the normal/race suites overlapped these runs heavily; these wall times
are not controlled before/after performance measurements. The temporary
checkouts were removed after completion; traces and timings were retained.

Validation passed `make verify`, the focused shared-model race tests, the
affected concurrency analyzer race tests, and the three reviewed uTLS labels
(two false positives absent, one true positive retained). The full race suite
was stopped during the unchanged resource suites and is **incomplete**, not a
passing gate. Remaining work is tracked under `gohawk-o44.1` through
`gohawk-o44.4`: demand-driven protocol extraction, receive/select loop contracts,
complete differing interface targets, and constructor/cross-method participants.

### Exact cutoff attribution rerun

The provenance-only change in `fb2107c` retained all four checks' root-module
candidate decision counts at the same pins. The frozen binary SHA-256 was
`c0ae6054e958d30c116475006766056f6a2cf695c4611c38a47a8d58155acceb`;
artifacts are in `.build/sync-cutoffs-2026-09-24`. It includes the code in that
commit before a rationale-comment addition. Traces include test source;
the standalone CLI's test-diagnostic inclusion flag was not enabled. All
surviving candidates still stopped before a dependency proof, so this run does
not establish any new bug coverage.

For the 72 channel-dependency summary candidates, the retained rejecting
instructions were:

| Instruction | Moby + Kubernetes |
| --- | ---: |
| Call | 23 |
| Conditional branch in unsupported loop/branch shape | 21 |
| Map allocation | 10 |
| Recovery return | 8 |
| Load | 3 |
| Slice | 3 |
| Index address | 2 |
| Field address | 1 |
| Type change | 1 |

This refines the earlier 29 control-flow cutoffs into 21 unsupported
loop/branch shapes and eight recovery boundaries. It does not turn the first
failure into an exhaustive explanation of the function. Counts filter on the
candidate's root-module path (excluding vendor and Kubernetes staging), select
`channelsafety/dependency-cycle` and `protocol-cutoff`, and deduplicate by
check, candidate, reason, and outcome before grouping by `instruction-kind`.
The leaf may itself be in a dependency.

For example, Moby's `NewLogFile` candidate at `logfile.go:138` stops inside
`openFile`, at `file_unix.go:8` calling `os.OpenFile`, with the caller at
`logfile.go:121` preserved. Its `containerManager.Run` candidate at
`containerbackend.go:81` stops in the launched closure at line 50 loading its
captured receiver, with the launch at line 49 preserved. These are concrete
investigation sites, not diagnosed deadlocks or proof that the omitted work
is irrelevant.

Moby took 3m59.98s and retained the same three test-loading errors. Its standalone
CLI nevertheless returned zero with empty diagnostic JSON; `gohawk-uqq` tracks
that error-status defect, and the scan is **partial**, not clean. Kubernetes
completed in 6m48.74s. Validation overlapped both scans, so these are not
controlled performance comparisons. The cutoff change passed `make verify`;
the race-enabled lock recursion timing test exceeded its deadline under load
but passed in an isolated four-core rerun without changing its threshold.

### Detached recovery and root results rerun

Commit `77eff78` treats a function's detached recovery block as dead once
every deferred call is a proven release. It also lets a root summary return
references and read package variables. The same pins and four checks were
rerun with a binary of that change (SHA-256
`c1d2e69fb9ee7455e82f1320175145eadde53e11b598dd5b9ec173f46bcfcedd`); traces
are in `.build/sync-controlflow-2026-09-24`.

Neither repository produced a diagnostic, and no candidate reached a cycle
decision. Control flow is no longer the most common first blocker for the
lock/signal checks. The table counts raw decision events per lock/signal
check, without the deduplication used above:

| Lock-and-join first blocker | Moby before | Moby after | Kubernetes before | Kubernetes after |
| --- | ---: | ---: | ---: | ---: |
| Control flow | 63 | 24 | 79 | 26 |
| Load | 31 | 44 | 16 | 36 |
| Effect | 23 | 44 | 18 | 41 |
| Unavailable body | 5 | 12 | 6 | 18 |

The blocked summaries now stop later, mostly at receiver-field loads and
opaque calls. Most of these roots are methods whose mutex is receiver state,
which the freshness requirement rejects even with a complete summary. This
change moves the first blocker; it is not evidence of new bug detection.
Moby's scan had no test-loading errors this time. Both scans ran on warm build
caches, so their wall times are not comparable with the runs above.

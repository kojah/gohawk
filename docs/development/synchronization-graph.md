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
channels, `sync.Mutex`, and `sync.WaitGroup`. `summaries.Provider` selects that
component and binds a helper's symbolic parameters to exact caller objects.
Complete empty effects are distinct from an unavailable summary. Imported facts
carry parameter-relative effects, never process-local SSA values or source
positions.

One acyclic worker `select` can yield a bounded set of complete continuations.
`syncgraph.Expand` builds a separate linear graph for each outcome, up to eight
graphs; a consumer must prove its property on every one. The ordinary
`FromSummary` contract remains incomplete for a choice, so linear consumers
cannot accidentally treat its arms as simultaneous events. Only the channel
dependency-cycle check currently consumes these alternatives. Launches hidden
inside helpers, nested selects, unrelated branch conditions, and cross-package
select continuations remain unknown.

The experimental lock-and-join and channel/lock-cycle proofs consume a
parent/children fragment through `internal/syncgraph`. The fragment has
`SyncEvent` nodes and separate program-order, spawn-order, and proven blocking
dependency edges. A root summary tracks up to four statically launched
children, each with its own identity, spawn point, and ordered effects. Both
checks require a fresh local mutex and channel, and a complete sequence in
which every child able to signal the waited-for channel must first acquire the
mutex held by the parent. A child that could release that mutex or signal
without acquiring it makes the proof inconclusive. The channel/lock check
additionally requires a statically unbuffered channel. A fifth launch,
divergent effects, loops, launches outside the lock-to-wait interval, and
opaque calls remain inconclusive.

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

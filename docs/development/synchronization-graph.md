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

The experimental lock-and-join proof is the first consumer of a parent/worker
fragment. It requires one launch, a fresh local mutex and channel, and a
complete sequence in which the parent holds the mutex while waiting for the
worker's signal, while the worker must acquire the same mutex before signalling.
The proof declines external participants, divergent effects, and opaque calls.
It does not claim to detect general deadlock cycles.

## Intended graph contract

When the ordered-effect model grows beyond one sequence, call its graph and
event types `SyncGraph` and `SyncEvent`. An event will retain its operation
kind, exact or symbolic resource identity, goroutine launch context, source
provenance, and completeness status. Edges will distinguish program order from
blocking dependencies. Branch alternatives and loop multiplicity must not be
silently flattened into unconditional order.

Construct function fragments once, bind them at calls through the existing
summary broker, and inspect only fragments relevant to a candidate. A package
fact publishes only complete, exportable guarantees. It must never interpret
missing effects as proof of no effect. A second launch, dynamic dispatch,
escaped channel or mutex, unresolved alias, and budget exhaustion remain
unknown unless a narrower structural proof independently accounts for them.

## Rollout boundary

Start with exact lock-and-join and other small obligation proofs; reuse their
event and identity machinery across checks before adding graph-wide candidate
search. Keep candidate enumeration and each proof under explicit budgets. The
initial model has no SMT solver or path-enumeration requirement. If those are
ever added, they are optional refinements and cannot upgrade an incomplete
model to a proven diagnostic.

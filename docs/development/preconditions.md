---
title: Preconditions
description: What a summarized function requires of the objects it is handed, and how a caller's state is checked against it.
---

A heap summary describes what a function does to the objects a caller can
name: what it stores where, what escapes, what it releases. That is a
postcondition. It says nothing about what the function needed to be true
on entry, so a caller that hands a summarized helper a file it has already
closed, or a response whose body it has set to nil, gets no answer from
the summary at all. Preconditions are the other half: the footprint the
callee reads, and the facts it relies on along it.

The model follows Pulse, Infer's current engine, and not the biabduction
engine before it. Pulse does not infer a weakest precondition. It records
the cells a function accessed as its precondition footprint, together with
the facts learned about each on the paths that matter, and applying a
summary unifies the caller's heap against that footprint. A caller state
that contradicts the footprint is a finding when the contradiction is a
witness, and unknown otherwise. gohawk's summary is one postcondition
joined over paths, without disjuncts, so a precondition fact is exported
only when every path that reaches a normal return establishes it. That is
the same `every` polarity the escape and release effects already carry.

## The footprint

Every placeholder region in the graph stands for a slot the function read
before it wrote it: the content of a parameter's field, a global, a slot
beneath a result the function was handed. The `Reads` list is that
footprint, projected onto the roots a caller can name. Nothing is inferred
about a slot the function never read.

## Requirements

A requirement is a fact about one footprint slot that held on every path
to a normal return. The first family is the one the leak check acts on
today:

- `requires P0 method Read every`: on every path, a method named `Read`
  is called with the object at `P0` as its receiver, directly or through a
  summarized callee. The name is the callee's own; whether that method
  releases the argument is decided by the caller, from the resource
  contract of the value it passed.

The second family was consumed by the `nilargument` analyzer, which has been
removed. Heap summaries still record it, but no analyzer reads it today:

- `requires P0/field:1 non-nil every`: the slot's content is dereferenced,
  or a method is invoked through an interface it fills, on every path. A
  caller whose graph says the slot certainly holds nil there has a
  witness. Only pointer-typed slots are judged, because an interface
  filled from a nil pointer is not a nil interface. This must be a
  requirement on the *incoming* slot: if the callee may write that slot or
  an ancestor before dereferencing it, the requirement is omitted. The
  summary does not encode write/dereference order, so even a write that
  actually comes later conservatively loses this precondition.

"Not released" needs no family of its own: the method family names what
the helper calls, and the caller's invalidation table says which of those
fail after release, so `rows.Next` through a helper after `rows.Close`
is reported once `Next` is in the rows table.

The third precondition is exclusivity, consumed by lockorder's
contradictory-order check. It is not a summary requirement but a
two-sided proof over the graph: an acquisition orders nothing when the
locked object is unescaped at the acquisition and is published only
afterwards on that path, or is a parameter that every caller in the
package hands in as a fresh unescaped local and the function is
unexported. Initializing a job under the registry lock before publishing
it is then not the reverse of the steady-state order. A purely local
object that is never published is deliberately still ordered, as the
existing fixtures require, and an acquisition inside a callee is never
discharged from the call, because callee acquisitions are summarized per
lock class rather than per argument.

A requirement is never exported for a slot the projection truncated, for a
path deeper than the projection bound, or from a function whose graph was
unavailable.

Projection proves candidates in stable summary order. It publishes at most
16 requirements, considers at most 64 candidates, and spends at most 1,000
expanded path states on each. A cut candidate contributes no requirement;
missing requirements are unknown, not evidence that the helper makes no use
of its arguments.

## The footprint at a cut

The footprint also sharpens what a caller forgets. A call the graph cannot
follow can write only what it can reach: what it was handed, what escaped
before it, globals, and objects other code created. The projection cuts
only those roots, and the caller forgets only beneath the arguments that
match them; a parameter the callee never let out keeps what the caller
knew about it, and what the callee stored into it is applied as usual.
This is the structural contract applied across a call: a parameter and an
unknown callee are connected only when the function's own flow connects
them.

## Applying a requirement

At a call site the substitution already resolves each summary slot to the
caller's slots. A requirement is checked against the caller's own proof,
never against the graph alone:

- For `method M`, `resourcelifetime` treats a call whose callee requires
  the resource's cleanup method on the exact argument, on every path, as
  the release of that resource.

A `use-after-release` check once also read `method M` requirements as uses of
a released resource; it was retired on 2026-09-27, as recorded in the
resourcelifetime design note.

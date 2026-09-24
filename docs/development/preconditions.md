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
to a normal return. The first family is the one the use-after-release
check can act on today:

- `requires P0 method Read every`: on every path, a method named `Read`
  is called with the object at `P0` as its receiver, directly or through a
  summarized callee. The name is the callee's own; whether that method is
  an invalidating operation is decided by the caller, from the concrete
  type of the argument it passed, against the same documented table the
  direct check uses. `io.Copy` requires `Read` of its source, and a caller
  that passes an `*os.File` it has closed has used the file after `Close`.

The second family is built and consumed by the `nilargument` analyzer:

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
caller's slots. A requirement is checked against the caller's own proof of
the contradicting state, never against the graph alone:

- For `method M`, the use-after-release check treats the call as an
  operation `M` on the argument. Its existing proof then applies
  unchanged: a direct release of the exact resource must dominate the
  call, nothing between may have reset or replaced the resource, and `M`
  must be in the resource type's invalidation table. The call is reported
  as the use, with the helper named.

The check therefore gains reach, not a new proof: the same three things
that make a direct `f.Read()` after `f.Close()` a finding make
`io.Copy(dst, f)` one.

## What could be wrong, and the accepted forms that pin it

- The helper reads the resource on some path only. Requirement is not
  `every`; nothing is reported. Fixture: a helper that reads only when a
  flag is set.
- The helper's method is not an invalidating one for the caller's type.
  `rows.Err()` after `rows.Close()` through a helper stays silent, because
  `Err` is not in the table. Fixture: a helper that calls `Err`.
- The helper receives a different value that merely derives from the
  resource, or a wrapper holding it. The argument must be the exact
  released resource under the storage identity proof. Fixture: a helper
  handed a struct holding the file.
- The release does not dominate the call, or is deferred. Unchanged
  proof; fixtures already exist for the direct form and are mirrored.
- The helper reassigns or reopens the resource before using it. The
  requirement is on the slot as read; a helper that stores into `P0`
  before reading it does not read the caller's object, and the
  projection's placeholder is not created for a slot written first.
  Fixture: a helper that replaces the file it was handed and then reads.
- The helper's body is unavailable or its graph was cut. No requirement
  is exported; the call stays an opaque use. Fixture: a bodiless callee.
- The method is invoked through an interface whose dynamic receiver is
  not the argument. Only an invoke whose receiver's pointees are exactly
  the parameter's object counts. Fixture: a helper that calls `Read` on a
  reader it built around the argument.

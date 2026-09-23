---
title: Local points-to model
description: The per-function region graph that answers identity, containment, and storage questions.
---

`ssaflow` answers "could these two values be the same object?", "what does
this cell hold here?", and "is this value stored somewhere inside that one?"
from one structure: a bounded, flow-sensitive points-to graph built once per
function. The graph replaces the separate walks that used to answer each of
those questions from the SSA value graph alone, so a copy, a merge, or a
captured cell that one walk understood and another did not is understood
once.

## Abstract locations

Every pointer-valued expression is mapped to a set of *slots*, each a region
and an access path beneath it (`field:0/index:1`). A region is one abstract
object:

- a **site** is a local allocation, one region per `Alloc` instruction;
- an **external** is the object a parameter, free variable, or global refers
  to, which existed before the function ran;
- an **opaque** object is produced by an instruction the graph cannot see
  through, such as a call result;
- a **placeholder** stands for the content of a slot the function never
  wrote, identified by the slot and the region's write version, so two loads
  of the same untouched slot are the same object and a load after an
  intervening write is not;
- a **snapshot** is an aggregate value, a struct or array copied at one
  point, whose sub-slots were copied from the source then;
- a **closure** holds its bindings.

Two more slots close the lattice: `nil`, and `unknown`, which may be
anything and never supports a must-answer.

## What the graph tracks

The state at each program point records the known contents of slots, which
sites have escaped, which slots were clobbered by an effect the graph could
not follow, and the write version of every non-site region. A store through
a single unescaped local slot is a strong update. A store through anything
else is a strong update of that slot and an invalidation of every other
non-site slot with the same last step, because two objects the function did
not allocate may be one. A call invalidates every non-site region and every
escaped site, and consults the local call-effect proof to keep an unescaped
site whose address the callee only reads.

Loops are handled by the fixpoint, with one rule that keeps must-answers
honest: an entry carried around a back edge whose object was created inside
the loop is marked *stale*, because the pointer then denotes an earlier
iteration's object. A stale entry supports may-answers only.

## Answers

Every answer keeps the *structural* contract the analyzers were built on:
two objects are the same only when the function's own flow connects them.
Two parameters, or two call results, are distinct objects even though at
run time they might be one; a diagnostic about one is not a claim about
the other. What the graph adds is disjointness where the value walk could
only say "no connection found", and exactness through copies, joins, and
captured cells where the walk gave up.

- `may alias`: some slot of one value may be some slot of the other: the
  same object at overlapping paths, `unknown` on either side, or a
  placeholder whose slot ever held the other object. Two different fields
  of one object, two objects the flow never connects, and an unescaped
  site against anything it was never stored into are disjoint.
- `must same`: both sets are one identical slot, not stale and not unknown.
- `content at`: the union of the slot contents in the state replayed to the
  observation; a must-answer needs one non-stale entry.
- `derives from` and `contains` keep their walks over the value graph and
  visible callees, and take their identity steps from `may alias`, so a
  copy or a join the graph resolves is followed there too.

A may-answer is deliberately weaker inside a loop than outside it: a stale
entry counts, so `true` there means "possibly, in some iteration". Only
`must same` filters stale entries. A `false` from `may alias` is the one
answer that can move a consumer from silence to a report, so it carries a
reason, `disjoint-paths`, `disjoint-objects`, or `unescaped-local`, and
`gohawk ssa -regions` prints every value's pointees so an answer can be
checked against the graph that gave it.

Exhaustion of the build budget, or a fixpoint that does not settle, makes
the whole graph unavailable and every answer unknown. Nothing here guesses.

## Heap summaries

A function's graph is projected onto what a caller can name: parameters,
results, globals, and captured variables, each with a bounded set of paths
beneath it. Every internal object collapses to `fresh`, numbered within the
summary so two fresh objects with one origin stay two objects, or to `nil`
or `unknown`. The summary lists where each named slot may point at exit,
with `must` when every return agrees on one non-stale target and the whole
build stored nothing else there; how each named object escaped, with
`every` when it did so on every return; which slots the function read
before writing; and where the projection was cut, including every root
when a call the graph could not resolve may have written anything. The
lifecycle pass adds the release effects its every-return proofs
established and exports the summary in its fact.

Applying a summary at a call site is substitution. The callee's parameter
becomes the argument's slots, a global the same variable looked up in the
program, a result the call's own value, and `fresh` a new object owned by
the call. Content the summary could not describe becomes a foreign object
the caller never named, not "anything". A truncated root is forgotten
beneath. Nothing else the caller holds is touched, which is the gain: a
call to a summarized function no longer makes the graph forget every
object the caller did not allocate. The lifecycle pass registers every
callee summary it imports before building any graph of its package, so the
substitution is available wherever the graph is. Closures with captured
variables, goroutine launches, and deferred calls keep the conservative
treatment.

## Boundaries

Intraprocedural only: callees contribute through the existing call-effect
proof and lifecycle summaries, never through a merged heap. No context
sensitivity, no interface dispatch, no channel or map contents beyond a
single may-slot. Dynamic indexes collapse to one element slot per array.

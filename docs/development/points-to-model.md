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

- `may alias`: the slot sets intersect, or either contains `unknown`, or
  both contain objects the function did not allocate and cannot tell apart.
  Two different fields of one object, or an unescaped site and anything it
  was never stored into, are disjoint.
- `must same`: both sets are one identical slot, not stale and not unknown.
- `content at`: the union of the slot contents in the state replayed to the
  observation; a must-answer needs one non-stale entry.
- `contains`: the value's objects are reachable from the owner's objects
  through slot contents, snapshots, or placeholders, at any point.

Exhaustion of the build budget, or a fixpoint that does not settle, makes
the whole graph unavailable and every answer unknown. Nothing here guesses.

## Boundaries

Intraprocedural only: callees contribute through the existing call-effect
proof and lifecycle summaries, never through a merged heap. No context
sensitivity, no interface dispatch, no channel or map contents beyond a
single may-slot. Dynamic indexes collapse to one element slot per array.

---
title: Local points-to model
description: The per-function region graph that answers identity, containment, and storage questions.
---

`heapmodel` answers "could these two values be the same object?", "what does
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
Selecting a field or element through `nil` faults, so a nil base contributes
no slot to the selection rather than `unknown`. Because `unknown` absorbs a
set, widening it would erase the other bases' real slots: every element of a
slice appended from a nil start would alias every object in the function.

## What the graph tracks

The state at each program point records the known contents of slots, which
sites have escaped, which slots were clobbered by an effect the graph could
not follow, and the write version of every non-site region. A store through
a single unescaped local slot is a strong update. A store through anything
else is a strong update of that slot and an invalidation of every other
non-site slot with the same last step, because two objects the function did
not allocate may be one. A call the graph cannot follow invalidates what it
can reach: what it was handed, what escaped before it by a call, a
goroutine, a global, or a send, every global, and every object some callee
created. A parameter or local this function never let out is beyond its
reach under the structural contract, and what the function wrote into it
stands; a store into an object already within reach hands on that reach.
A resolved call consults the local call-effect proof to keep an unescaped
site whose address the callee only reads.

Loops are handled by the fixpoint, with one rule that keeps must-answers
honest: an entry carried around a back edge whose object was created inside
the loop is marked *stale*, because the pointer then denotes an earlier
iteration's object. A stale entry supports may-answers only.

A dynamically indexed struct copy preserves bounded reference fields when its
array or slice window is known and has at most 16 elements. Each copied field
contains the union of destinations from every possible element; an unwritten,
nil, opaque, or stale alternative cannot become a must-answer. Nested struct
fields use the same depth and slot limits as summary projection. Pointers stay
leaves and nested arrays remain unknown. A snapshot remains clobbered outside
its materialized fields. Dynamic writes beneath the source's elements retain
unknown evidence until descendant wildcard writes have a complete content
model; fixed writes and replaced pointers are read at the copy point.

`ExclusiveAt` accepts different field addresses only when every non-stale
alternative belongs to the same unescaped object. Exact identity queries still
require one exact slot. This distinguishes caller-owned and local destination
tables without conflating their local holders with the destination. Mixed
objects, nil, opaque pointers, unknown or oversized windows, and dynamic
replacement stay unknown. `store_exclusivity_test.go` covers these
boundaries, including slice offsets and nested fields.

The motivating [ferro statement storage](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99)
uses copied structs containing addresses of receiver-owned statement fields.
The graph supplies destination evidence; the resource classifier owns whether
a store transfers or opaquely consumes an obligation.

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

Graph containment has one private region search in
`store_regions_containment.go`. `Contains` selects the whole-build slot history;
`ContainsAt` selects the state immediately before its observation. Both hold
the graph lock and follow the same depth-limited search, including cycles,
stale may-pointees and unknown contents. Later or replaced storage remains in
history but cannot become current containment. A missing observation is
unavailable rather than a known negative. No search result establishes cleanup
or exact identity. Real SSA controls in `store_regions_test.go`
pin later/replaced stores, unrelated objects, nested and cyclic containment,
and both sides of the existing depth limit; the history widening test retains
possible containment after an overfull slot becomes unknown.

Deferred captured-cell mapping uses `DeferredCellRelationWithin`. One completed
shared instruction census selects reachable RunDefers, or returns when a
test-registered callback has no deferred execution points. Selection,
reachability, occupant unions and descendant history checks spend the completion
request's allowance. An interrupted census or relation publishes no occupant
prefix and is unavailable to the mapping fallback. Nil alternatives retain the
existing exact-cell policy; stale or unrelated occupants prevent an exact claim.
Graph construction, state replay, pointee lookup and type/alias internals still
have independent costs. The deferred-cell tests cover exact, cleared, replaced,
merged, aggregate and test-registered cells, cutoff at intermediate allowances,
discarded observation prefixes and fresh recovery.

A may-answer is deliberately weaker inside a loop than outside it: a stale
entry counts, so `true` there means "possibly, in some iteration". Only
`must same` filters stale entries. A `false` from `may alias` is the one
answer that can move a consumer from silence to a report, so it carries a
reason, `disjoint-paths`, `disjoint-objects`, or `unescaped-local`, and
`gohawk dump heap` prints every value's pointees so an answer can be
checked against the graph that gave it.

Exhaustion of the build budget, or a fixpoint that does not settle, makes
the whole graph unavailable and every answer unknown. A slot recorded as
holding more than a bounded number of objects, which a buffer appended to
at every site of a large function does, is widened to `unknown` instead:
may-answers about it say yes, must-answers say no, and the graph stays
small where it would otherwise multiply those sets through every summary
application and join. The dump prints each widening. Nothing here guesses.

## Determinism under concurrent analyzers

The graph cache and the summary registry are shared by every analyzer that
runs on a package, and those analyzers run concurrently. A result must not
depend on which of them asks first. A graph lookup that finds another
analyzer's build in progress waits for it rather than answering from an
unavailable placeholder, and a summary requested while another analyzer is
projecting the same callee is projected privately rather than treated as a
call-cycle cut. Both used to give weaker answers whose presence depended on
scheduling: a resourcelifetime false positive in batch 63 (Polaris), and a
goroutineownership report in grpc-go that appeared in some runs of the same
binary and not others. A build never needs its own function's graph, and a
graph applies no summary from its own call cycle, so neither wait can be
reached from the goroutine doing the work.

Call-cycle discovery shares one immutable direct-callee inventory per built
SSA function across reachability roots. Static Go/Defer calls and generic
wrapper/origin alternatives retain their existing edges; interface invokes,
dynamic function values and builtins add none. The package boundary is checked
before discovering a callee's body. Each traversal owns a copy of its initial
queue, so appending successors cannot overwrite a published inventory. Summary
registration changes heap evidence but not this structural metadata. Concurrent
readers may discover privately and then reuse the first published inventory
under the existing cycle-cache lock. Like cached reachability, the inventory
assumes completed SSA bodies; it adds one direct-edge record per visited
function, separate from the transitive reach sets. The call-cycle tests cover
branching queues, shared roots, foreign packages, recursion, generic wrappers,
summary registration and concurrent publication.

Failed publication and eviction share `removeIndexedEntryLocked`: mark the
entry stale and unlink only the cache slot still naming that exact entry.

A finishing old build cannot delete its replacement. Dependency cleanup stays
with eviction; completion notification stays with publication under the cache
lock. Removing a running build does not wake its waiters before the build
finishes. A consulted summary generation that changed during construction
rejects publication before the graph enters the dependency index.
`store_regions_test.go` covers replacement preservation, stale completion
notification, changed-summary rejection and concurrent publication/lookup.

Allocation reset and full overwrites share `clearSubtree`; that operation clears
stored contents, backing snapshots and clobber stamps beneath the exact target.
Opaque clobbering shares only `regionState.forgetStoredSubtree`, after collecting
reachable children, and retains its effect stamps. Removing those stamps would
turn uncertain unwritten contents into known zero values. The escaped-loop
allocation bailout still precedes clearing, and resetting a site does not erase
the graph's may-only history or another site's state.

Backing snapshot lookup returns one canonical relative path below the nearest
stored prefix. A nested prefix's separating slash is removed at lookup, so
reading an unchanged copied field names the same source slot as a direct read.
Root lookup uses the same traversal; copy propagation consumes that normalized
path without repairing it again. Source stamps still distinguish later writes,
opaque clobbers and sibling fields; cycle and depth cutoffs remain unknown.
`store_writes_test.go` pins nested/deeper/repeated copies and snapshots
taken before a source change beside mutation and opaque-call controls.

## By-value type queries

`CanHoldReference` and `holdsByValue` share `anyByValueType`: visit the current
type, struct fields and array elements, stopping at reference edges. Their
predicates remain separate. Reference capability accepts pointer, interface,
slice and function leaves; overwrite capability matches the owner's underlying
struct identity. Distinct named declarations with identical underlying structs
retain the existing conservative overwrite answer. Zero-length arrays retain
element traversal. Compiled-type controls in `query_reference_test.go` cover these
boundaries, aliases and recursive types whose recursion crosses a pointer.

`containsPrimitive` retains its field-based synchronization predicate rather
than becoming a root-matching query. `walkStructReferences` enumerates paths
under depth/slot limits and cuts at arrays; it is a different contract from a
Boolean type-capability query. This consolidation adds neither a type-walk
budget nor a cross-package guarantee.

## Heap summaries

An instantiation of a generic function is usually a synthetic wrapper that
converts its arguments and calls the generic body. That call is not
recursion, so the wrapper applies the body's summary: `Must[*os.File]`
returns its argument because `Must` does.

A function's graph is projected onto what a caller can name: parameters,
results, globals, and captured variables, each with a bounded set of paths
beneath it. Every internal object collapses to `fresh`, numbered within the
summary so two fresh objects with one origin stay two objects, or to `nil`
or `unknown`. The summary lists where each named slot may point at exit,
with `must` when every return agrees on one non-stale target and the whole
build stored nothing else there; how each named object escaped, with
`every` when it did so on every return; which slots the function read
before writing; and where the projection was cut: a root whose slots
exceeded the bound, and, when a call the graph could not follow ran, the
roots that call could reach, which are the globals and the objects the
function let out. A parameter the function never handed on or published
is not cut, so a helper that stores its second argument into its first
and then logs through an interface still tells its caller about the
store. The lifecycle pass adds the release effects its every-return proofs
established and exports the summary in its fact.

Returned struct snapshots also project untouched reference fields through the
graph's existing lazy content query. A scalar edit in a value receiver keeps
the other fields related to the input; extracting a nested struct keeps its
parameter-relative field paths. Replacing a reference field instead publishes
the replacement. This fills a summary gap isolated by the
[returned-logger assessment](../../benchmarks/precision/audits/returned-logger-assessment-2026-10-01.md),
without adding struct-copy traversal to an analyzer. It does not establish
ownership for a constructor that only possibly retains its argument.

This projection follows by-value struct fields within `SummaryPaths` and
`SummarySlots`, never through pointers. Arrays and exhausted bounds produce a
cut. A lazy placeholder stamped by an opaque write becomes unknown because
the summary cannot name that later version of the caller's slot. The existing
graph can also lose an earlier copy across an opaque call; projection keeps
that uncertainty. `store_heap_summary_test.go` pins preserved and replaced fields,
nested extraction, snapshots, opaque writes, and both bounds.

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

Escapes are recorded per slot. The address of a field handed to a callee
escapes what the field holds and everything beneath it, never the object
above it, so locking a receiver's embedded mutex does not store the
receiver. Applying a summary escapes what the arguments held as they were
handed in, before the callee's truncation forgets any of it. A map key is
stored as surely as a map value.

`gohawk dump facts` prints every summary as `heap …` lines beneath the mask
claims, for the package's own functions and the callees it imports, and
`gohawk dump heap` prints every local function's registered summary and graph as
the analysis saw it, private helpers included, with an `applied` line for
every call a summary was applied at and an `escaped` line for the first
instruction that escaped each slot in each way. `gohawk dump heap -bare` skips
the lifecycle pass, so dependencies' summaries are missing while callees with
bodies in the loaded packages are still projected on demand, which is what a
unit test sees.

A summary also carries requirements, the precondition half: the methods
the function calls on the object at a named slot on every normal return,
which a caller checks against what it has already done to that object.
See [Preconditions](preconditions.md).

## Boundaries

Intraprocedural only: callees contribute through the existing call-effect
proof and lifecycle summaries, never through a merged heap. No context
sensitivity, no interface dispatch, no channel or map contents beyond a
single may-slot. Dynamic indexes collapse to one element slot per array.

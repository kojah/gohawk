---
title: Inferred facts
description: Modular function summaries, their fact publication, and the guarantees each component can establish.
sidebar:
  order: 3
---

Function summaries are the consumer-facing model. Analysis facts publish
supported summary components across package boundaries; they are not a separate
kind of knowledge. `internal/analysis/summaries` brokers access to independently computed
result, lifecycle, and concurrency components. Each domain keeps its inference
and publication in `resultfacts`, `lifecyclefacts`, or `concurrencyfacts`.

### Binary publication

Domain-owned publication types wrap the existing Go summaries in an opaque
`internal/analysis/factcodec` envelope. The envelope uses deterministic CBOR, not JSON,
and hides nested summary schemas from gob's per-stream type descriptors.
Published values and their reachable slices, maps, and pointers are immutable;
copied envelopes share a concurrency-safe encoding cache. Decoding constructs
an independent value and replaces the receiver only on success.

The private payload starts with a format/version header. Unsupported versions,
oversized payloads (over 16 MiB), excessive nesting or collections, duplicate
keys, trailing data, and unknown fields are rejected. Existing domain version
and semantic validation still apply; serialization cannot turn missing or
incompatible evidence into an empty complete summary. A rebuilt analysis tool
invalidates the Go analysis cache; no legacy JSON decoder is retained.

Protobuf comparisons live in the separate `tools/codecbench` module. Its schemas
and generated types are experimental and are not production dependencies.

## Selecting knowledge before analysis

An analyzer declares one immutable selection and uses it both for its ordinary
`analysis.Analyzer.Requires` list and its provider:

```go
var knowledge = summaries.Select(summaries.Requirements{
    Results: true,
    Lifecycle: true,
})
// At analyzer setup: Requires: knowledge.Requires()
// During Run:
provider := knowledge.Provider(pass)
result, availability := provider.ForFunction(fn).Results(budget)
```

There is no additional scheduler and no request-time expansion of dependencies.
Selecting results and lifecycle does not schedule concurrency inference.
Availability distinguishes `NotRequested`, `Unavailable`, and `Available`.
An available result component may still say `Unknown` for every result.
Concurrency's complete ordered sequence remains its own domain contract, not
a claim that the whole function is understood.

Result and lifecycle views expose uninstantiated declaration guarantees. The provider's
lifecycle-evidence and concurrency-at-call adapters retain the existing exact
argument, capture, and result binding machinery. A formal parameter mask is not
already a guarantee about an arbitrary caller value. Private lifecycle bodies
without a published declaration summary use the existing local evidence path.

`ConcurrencyAtCall` delegates both local and imported calls to the concurrency
engine's `AtCall`. The broker does not copy and bind a formal declaration first:
the domain owns imported lookup, exact argument substitution, field
materialization and cutoff reasons in one call path. The unused formal concurrency
view and its declaration-copy adapter are removed. The imported/local two-mutex
controls in `internal/analysis/summaries/concurrency_binding_test.go` pin effect order,
caller values, call-site provenance, unavailable declarations and interrupted
binding; an available component does not make an incomplete sequence complete.

Every catalog analyzer consuming lifecycle or concurrency summaries obtains
them through the broker. The existing domain evidence engines are retained.
`resourcelifetime` additionally requests result guarantees and uses them at its
existing feasible-edge decision. Other analyzers request results only when
they have a concrete use for that additional knowledge.

`TestAnalyzersUseSummaryBroker` rejects direct domain prerequisites, fact-backed
lookup functions, constructors, and fabricated or asserted engine values in
analyzers, using declaration identity rather than import spelling. Domain types,
proof vocabulary, and explicitly reviewed structure-only helpers remain usable.
Raw object-fact import/export remains inside the owning domain pass; the broker
cannot bypass that boundary either.

Returned-view field inference distinguishes complete receiver-method summaries
from unavailable ones. Visible private bodies reuse receiver-field inference
rather than requiring an exported fact. An unavailable method cannot supply
negative release evidence; the field-based claim is withheld. A valid package marker can still
supply a known empty summary. Type-only view rules do not consume that field
census. See `internal/analysis/passes/lifecyclefacts/returned_views.go` and its focused
unavailable-method regression.

## Unconditional result guarantees

The result component records `AlwaysNil`, `AlwaysNonNil`, `AlwaysTrue`,
`AlwaysFalse`, or `Unknown` separately for each result. Guarantees require
agreement across normal returns, including recovery returns, and a return
witness. Absence of a return does not prove termination or any result property.
The initial proof conservatively includes all SSA return blocks; it does not
solve arbitrary path conditions.

Literal results, fresh allocations, interface boxing, agreeing phi alternatives,
and direct forwarding calls are supported locally and across packages. Stable
local loads and field projections reuse `heapmodel.Storage.ContentFromWrites` at the
load's execution point, including a saved snapshot read before a later write.
This projects existing storage evidence rather than building a second heap
analysis. The opportunistic query declines unknown writes without requesting
a whole-function points-to graph for every opaque load. A boxed typed-nil
pointer is a nonnil interface: the result proof
preserves boxing instead of using identity-only unwrapping. Unresolved loads
remain unknown; neither a mutable package sentinel's initializer nor a named
result before deferred modification proves the value returned later. There are
no name-based `errors.New` contracts.
Recursion and exhausted searches produce unknown evidence and do not poison
the summary cache. Queries share a 2,000-step budget in the initial consumer;
export also has that per-function budget and a 16-result limit.

Nilness does not imply ownership. A conditional cleanup guarantee does not
imply that its Boolean result is always true. Unknown never removes a branch.
Existing literal/integer branch evidence remains in place where the result
component does not replace its semantics.

`Guarantee.Outcome` projects an already-obtained unconditional guarantee into
the flow outcome vocabulary. Unknown and invalid guarantees remain unconstrained.
The conversion performs no query or binding and establishes no cleanup.
`Provider.OutcomeOf` first recognizes literal/construction outcomes, then queries
`ResultOf` and projects its guarantee. Cancellation and resource result guards
share this lookup; guard retention and completion remain analyzer policy. Nil
providers and unavailable components still recognize literals, while call results
remain unknown. Boxing stays observable, and the caller supplies the existing
summary-query allowance. Arguments never strengthen an unconditional guarantee.

## Result cases

Beside the unconditional guarantees, the result component proves result
cases, "result R has outcome O on every normal return where condition C
holds", and exports them with the fact. C is an `calls.CallCondition`, the
same condition type summary cases of cleanup use, here naming a parameter's
nilness or a paired error result's outcome:

| case | condition | who uses it |
|---|---|---|
| a Boolean result is false | the exact parameter is nil | `resourcelifetime` treats `if failed(err)` as an acquisition-error guard, for local, captured, and imported predicates alike |
| a nilable result is non-nil | the paired error result is nil | the summaries provider prunes `result == nil` below the success arm of that error's check |
| a nilable result is nil | the paired error result is non-nil | the provider prunes `result != nil` below the failure arm |

A parameter case walks the body under the assumed nilness and requires the
expected literal, or a nil comparison of the exact formal decided by the
assumption, on every reachable normal return. A paired case checks every
normal return and lets a return that forwards both positions of one call
inherit that callee's case. Both need a return witness on the assumed side;
a missing case says nothing about the opposite implication. The symmetric
"true when the parameter is non-nil" case had no consumer and is not proved.

Conditional inference shares its summary allowance with queued block visits,
assumption value folds, bound-condition decoding and successor pruning. Direct
forwarded result-slot decoding charges one dispatch before requesting a callee
summary; it performs no wrapper or referrer search. Cutoff cannot retain a case
from an interrupted walk, even if an earlier return supplied its witness.
The summary memo discards interrupted answers and permits a fresh query.

Unconditional result folds attach that same allowance at
`storedResultQuery.resolve`, including wrapper, phi and revisit work before
leaf inference. Storage recursion reuses the reaching walk and its cycle
history; it does not start a separate fold. This preserves the explicit
transparent forms and typed-nil boxing policy. Exhaustion cannot retain a
result guarantee or publish its interrupted summary.

A returned parameter is not a case: the result is the exact parameter, under
the same static type, on every normal return. `resourcelifetime` resolves a
cleanup receiver through such a call, so `wrap(file).Close()` settles `file`
and a later close of `file` is not required; `lifecyclefacts` uses the
same mechanic to keep an unchanged return from becoming a view. It is exact
storage identity: a wrapper, an interface conversion, or a value chosen
between the parameter and something else is not the parameter.

Returned-parameter inference uses `lifecycle.ProveReturnedParameterWithin`:
a positive normal-return witness precedes the shared obligation coverage walk,
and one storage query compares exact same-type identities at its returns.
Reachability, coverage and storage queries spend the result query's allowance.
Cutoff remains unknown; the summary memo discards interrupted answers and the
writer cannot publish their identity relations. A fresh query can retry.
Graph construction and type-system internals retain independent costs.

Every analyzer that walks an obligation to its returns consumes these through
`summaries.Provider.FeasibleSuccessors`, either directly or through the
obligation walk's successor hook, so a branch a callee's result rules out is
pruned once rather than per analyzer.

## Never returns

The result component also records whether a function never returns
normally: every path from its entry ends in a terminating call, a panic, or
a loop that never exits. The terminating calls are the documented catalog,
`os.Exit`, `log.Fatal`, `runtime.Goexit`, testing's `FailNow` family, plus
any callee whose own summary carries the claim, so a project's `fatal(err)`
wrapper is recognised across packages. A body with a recover block makes no
claim, because a deferred recover returns normally from a panic the entry
never reaches; a function that may exit does not carry it either.

The walks consume it through `summaries.Provider.Terminates`, the terminator
hook of `path.InstructionTerminatesWith` and `NormalReturnReachableWith`:
a path that calls such a function ends there, exactly as it ends at
`os.Exit`, so an early return behind the call is not reached.

Normal-return reachability uses `path.ProveNormalReturnWithin` with the
result query's allowance. Its structured proof distinguishes a reachable return,
a completed search finding none, and unavailable evidence. Queue/instruction
visits and termination queries share the allowance; interrupted callbacks or
deferred-registration scans cannot establish no-return. Only a completed
negative proof sets the declaration's termination guarantee. Recover entries
retain the existing no-claim boundary, even for a recognized terminating defer.
Function-summary memoization discards cut answers and dependent cache entries;
fact publication requires an available summary. Cutoff controls verify that a
fresh larger query can recover without having published a partial guarantee.

## The shape of a fact

A lifecycle fact's original parameter masks describe how a named callee uses
its inputs. They have these boundaries (the conditional and returned-handle
components described later additionally express specific relationships):

- Each mask describes **one parameter at a time**.
- Discharge masks require **every normal return**; retention masks deliberately
  over-approximate possible escapes, as described below.
- It is computed **once per function**, not once per call site.
- It is attached to a callee the analyzer can **name directly**, not one
  reached through an interface.

The declaration below is regenerated from
`internal/analysis/passes/lifecyclefacts/fact.go` by `go generate ./...`, so the mask
list is always the one the code has; do not edit it by hand.

<!-- gohawk:generated-fact-fields:start -->
```go
// Fact is the compact cross-package ownership summary exported for a
// function. Its claims are grouped by polarity, so every use site states
// which kind it reads: a Must claim holds on every normal return and a
// consumer may settle on it, while a May claim over-approximates and a
// consumer may only treat it as unknown, never as settled. This package is
// internal analysis infrastructure, not a public extension API.
type Fact struct {
	Must	MustClaims
	May	MayClaims
	// Discharges are the exact cleanup claims, unconditional and conditional
	// in one list: which method is called, on which parameter, at which
	// access path beneath it, on every normal return of the case Condition
	// names. A discharge with an empty condition is a Must claim; any other
	// is a summary case, a positive guarantee a caller selects only when it
	// can check the condition, so it never widens an unconditional claim and
	// a missing case does not establish no effect. An empty path means the
	// parameter itself (MethodMask); InvokeMethod means calling a function
	// parameter at all, and SynchronousInvokeMethod calling it in the same
	// goroutine before returning; a field or element path lets a caller match
	// the resource it stored there rather than any resource the argument
	// contains. See conditional.go for the cases.
	Discharges	[]Discharge
	// Heap is the projection of the function's points-to graph onto what a
	// caller can name: where each parameter, result, and global slot may
	// point at exit, how each object escaped or was released, what was
	// read, and where the projection was cut. See heap.go.
	Heap	*heapmodel.HeapSummary
	// ReturnedCleanup relates an invoked callback result to an exact factory
	// parameter or sibling result. Merely returning the callback does not clean up.
	ReturnedCleanup	*ReturnedCleanupSummary
	// signature is the summarized function's signature. It is attached
	// whenever a fact is produced or read for a known function and is never
	// serialized. It supplies the type gates the heap projection cannot see
	// when the transfer claims are read from Heap; see heap.go.
	signature	*types.Signature
}

// MustClaims hold on every normal return of the function. The transfer
// claims of the same polarity, ReturnedOwner, Stored, and ReceiverStore, are
// read from Heap rather than stored; see heap.go.
type MustClaims struct {
	// ReturnedView narrows ReturnedOwner: the parameter is stored in the
	// returned struct, but no method of that type releases the field, so the
	// caller keeps the obligation. See returned_views.go.
	ReturnedView	ParameterMask
	// OwnedFields and ReleasedFields are indexed by struct field, not
	// parameter; see fields.go for the constructor and method summaries.
	OwnedFields	FieldMask
	ReleasedFields	FieldMask
	// OwnedResults is indexed by result position: the function hands back a
	// fresh resource it acquired itself, and the caller owes its cleanup.
	// See owned_results.go for the freshness the proof requires.
	OwnedResults	ResultMask
	// RetainingResults is indexed by result position: the function hands
	// back a wrapper that holds a fresh resource it acquired, and the caller
	// must keep, hand over, or return that wrapper. See retaining_results.go.
	RetainingResults	ResultMask
}

// MayClaims over-approximate what the function might do. A set bit never
// proves an effect happened, and a clear bit never proves it did not. The
// retention claims of the same polarity, Retained and Kept, are read from
// Heap rather than stored; see heap.go.
type MayClaims struct {
	// LoopReleased marks parameters whose derived values the callee releases
	// inside a loop, as a variadic close helper does to each of its files.
	// Which element an iteration releases is decided by iteration, so a
	// consumer treats the call as unknown, never as settled.
	LoopReleased ParameterMask
}
```
<!-- gohawk:generated-fact-fields:end -->

A `ParameterMask` is a bitset over the first 64 parameter positions; the
receiver is position 0. `OwnedFields` and `ReleasedFields` are indexed by
struct field instead and describe constructors and methods of a type.

A discharge verb on a struct or array parameter passed by value counts a
cleanup call on one of its fields, such as `j.out.Close()` in
`func finish(j job)`. The builder spills such a parameter into a local cell
before it can select a field, and the summary follows that spill the same way
it follows a field address selected from a pointer parameter, provided the
cell is only ever written whole and still contains that parameter at the
receiver's read. Whole-aggregate replacement does not discharge the original
parameter's field. A saved field value retains its earlier snapshot, including
through later interface wrapping; agreeing branch writes retain the path,
while conflicting writes leave it unknown. Address-only paths require every
whole write to agree because they supply no read snapshot. The shared storage
query proves these identities without constructing a points-to graph.

Deferred cleanup forwarded through a helper uses the same static projection
path retained by local completion. Saving `j.out` before replacing `j`, then
deferring a helper on that saved value, discharges the original `out` field.
Reading it after replacement discharges only the replacement parameter's field.
Conflicting writes export no discharge. A helper on a sibling field never
discharges `out`; the nested completion must name the selected field.
`forwarded_discharge_paths_test.go` pins these exported contracts.

## Three answers to "what happened to my value?"

| mask | guarantee | what it means for the caller |
|---|---|---|
| a discharge (`Close`, `Wait`, … on the parameter) | always | the obligation is finished — a join |
| `ReturnedOwner` | always | the obligation moved onto the result; keep tracking it |
| `ReturnedView` | always | the result is a window onto the parameter; the caller still owns it |
| `Stored` | always | firm evidence the callee keeps it; safe to treat as a transfer |
| `Retained` | maybe | the callee might keep it somewhere; fall back to `unknown` |
| `Kept` | maybe | the callee might keep what it loaded out of the parameter at this path; a caller whose resource sits at another path is unaffected |
| `LoopReleased` | maybe | the callee releases values drawn from it inside a loop; which element is decided by iteration, so fall back to `unknown` |

`ReturnedOwner` and `ReturnedView` look identical in SSA — both store the
parameter in a returned struct. They differ only in whether a method of the
returned type releases that field, which is what `OwnedFields` and
`ReleasedFields` record. That is why the view fact exists alongside the owner
fact rather than being inferred from it.

Fresh `OwnedFields` inference requires a call result of a known concrete
resource type stored in the returned object. An arbitrary custom `Close`
method establishes cleanup capability, not that construction acquired a live
resource. Nested custom owners and interface-only results therefore remain
unknown; they are not recursively assumed to create new obligations.

`OwnedResults` is the direct-result counterpart, indexed by result position:
the function acquired a concrete resource itself and hands it back as a
result, possibly behind an interface the caller can close. The proof is
strict about freshness. The acquired value must reach the return through
nothing but interface conversions, phi merges, and nil comparisons; a call
that has seen it, a store, a captured closure, a registry, or a cleanup that
can run before a return of it all decline the claim. `resourcelifetime`
turns the claim into an acquisition whose cleanup is the result type's, but
only for packages the catalog does not model: the catalog's decision about
a standard-library API, such as leaving `database/sql` statements to their
transaction, is not reopened by an inferred owner.

`RetainingResults` extends `OwnedResults` through wrappers, indexed by result
position: the function acquired a concrete resource and returns a wrapper that
holds it, such as `slog.New(slog.NewTextHandler(file, nil))`. Every step from
the resource to the result must be a call whose imported summary proves
`ReturnedOwner` for that argument, so a may-hold wrapper such as
`bufio.NewWriter` or a same-package helper does not qualify; the freshness
rules above apply to the resource and to each wrapper. No method of the
wrapper releases the resource, so `resourcelifetime` gives the caller an
obligation without a cleanup method: keeping, handing over, or returning the
wrapper settles it, and dropping it is reported at the call. In the
constructor, returning that wrapper is a handover only when the constructor's
own summary makes the claim; a proven chain without the claim, including every
unexported constructor, is an uncertain boundary.

A constructor storing the acquired value in an already external map or owner
also leaves fresh-result ownership unknown. A returned wrapper can share its
resource with a manager; its cleanup capability does not establish a separate
caller duty. Stores into local scratch collections do not have this effect.

Visible private helpers have no exported object fact. Retention consumers can
reuse the bounded local query instead, requiring a direct global store or a
known retaining call before every normal return. A returned wrapper is counted
at its actual global store, using positive returned-owner and stored-argument
claims; merely constructing or discarding it does not transfer ownership.
Local spills, caller-owned cells, and callback captures do not establish this
boundary. Consumer queries read prerequisite summaries, not another analyzer's
object-fact namespace.

## Discharge paths

Every discharge is one entry in `Discharges`: the method, the parameter, the
access path beneath it, and the condition it holds under. An empty condition
is the unconditional claim, and every mask accessor such as `MethodMask`
reads only those entries; any other condition is a summary case (see
"Summary cases"). An empty path is exact about the parameter itself: `Close`
on `file` means `file.Close()`, and `MethodMask("Close")` reads those
entries. Calling a function parameter is recorded with the method
`InvokeMethod`, and calling it in the same goroutine before returning with
`SynchronousInvokeMethod`. A cleanup of a field or element of the parameter is
not the same claim; its entry carries the access path beneath it, such as
`field:0` for `j.out.Close()` or `index:1` for `files[1].Close()`, including
through the cell a by-value parameter is spilled into. Each path is proved on every normal return on its own.
The path `index:*` (`EachElementPath`) means every element of a slice
parameter: the parameter is used only by `len`, `cap`, and range loops that
call the method on the element each iteration reads, and every normal return
follows such a loop running to completion. A local collection handed whole
to such a helper is released by the call.

A caller is credited only for the resource it stored at that path beneath
its argument, resolved from the caller's own stores; a helper that closes
the other field or the other element leaves the obligation open where,
before paths, any resource the argument contained was credited. One
exception keeps an older shape exact: a resource that is itself an owner,
such as an `http.Response`, is released by a helper closing its
resource-typed field. A deferred literal or deferred helper is exported at
the path the completion proof names, so one closing the body projected from
a response claims the body's path, and one whose path the proof cannot
name, because different returns settle different fields or the receiver
comes from a call, claims nothing. A bound callback such as `value.Close`
is the parameter itself and sets the mask. The same rule applies inside the
local completion search: a target mapped into a callee by containment
remembers its path, and a receiver that is a proper projection of the local
must be at that path.

`ClaimReleases` includes every parameter with a discharge at any path, for a
consumer that only asks whether the callee releases part of what it was
handed.

## Claims as projection queries

The heap projection is the one encoding of what a function does to the
objects a caller can name, and every transfer claim is a query over it, so
the masks a consumer reads and the summary a caller's graph applies cannot
disagree. The derivability test in the pass records, function by function,
that each claim follows from the projection:

Heap summary version 2 distinguishes a returned address such as `&p.field`
from the pointer value stored in `p.field`. Substitution maps the first to
the caller's field slot and reads the second field's contents; a zero-valued
field can have nil contents without its address being nil. Imported heap
summaries with a different version are unavailable, never reinterpreted.

- `ReturnedOwner`: on every normal return with a non-nil result, some
  result, or a slot beneath one, holds the parameter's object and nothing
  else (a `must` hold).
- `ReceiverStore`: on every normal return, some slot beneath the receiver
  holds the parameter's object and nothing else (a `must` edge).
- `Retained`, loose: the parameter's object escaped in any way, or a global
  or result slot may hold it.
- `Stored`, strict: the object escaped into a field, a global, or a channel,
  or a global's slot may hold it. Handing it to a call or a goroutine, or
  returning it, is not storage.
- `Kept`: the paths beneath a struct-shaped parameter whose content escaped
  or may be held by a global or a result; content the projection could not
  name that is held outside is claimed at the parameter itself.

The projection also carries requirements: `requires P0 method Read every`
says the function calls `Read` on the object it was handed at `P0` on
every normal return, directly or through a summarized callee. A consumer
reads them through `ArgumentMethodsRequired`; the leak check treats a helper
call whose requirement names the resource's cleanup method as its release.
See [Preconditions](preconditions.md).

An escape is recorded per slot, not per object: the address of a field
handed to a callee escapes what that field holds and everything beneath it,
never the object above it. A receiver whose embedded mutex is locked is not
thereby stored. Map keys escape as surely as map values, because a range
hands them back.

## Serialization

Every fact type encodes itself as deterministic CBOR inside the gob stream
go/analysis uses, through `internal/analysis/factcodec`, with a versioned header and
no JSON fallback. Decoding rejects unknown fields and duplicate keys. gob
compiles a decoding engine per type for every stream it opens, and the
analysis test harness round-trips every inherited fact through a fresh
stream, so a summary with many fields cost more to compile than to decode;
a byte slice costs gob nothing. The lifecycle pass also exports no summary for a function proven
to do nothing with its parameters. A `SummarizedPackage` fact on the package
carries the distinction an importer needs: a function of a summarized
package with no summary of its own was proven empty, while a function of a
package without the marker, or one the marker lists as bodiless, is unknown.

## Kept contents

`Retained` is about the parameter itself and deliberately ignores what is
loaded out of it. A caller that hands an aggregate holding its resource to
an imported helper needs the other half: can the resource inside outlive the
call? `Kept` answers that by path, from the projection: each path beneath
the parameter whose content escaped, or that a global or result slot may
hold. It is exact about where: `field:1` for a helper that closes, stores,
or hands on the second file of a pair, and nothing for the first. A
basic-typed load holds no resource and is never claimed. Content the
projection could not name that is held outside is claimed at the parameter
itself, because it may have come from anywhere beneath it.

Only struct-shaped parameters carry the claim, and a parameter that is
`Retained` outright carries none, because retaining the aggregate keeps all
of its contents. A consumer asks about the path where its own resource is
stored and treats a claim at that path, a prefix of it, or a longer path
inside it as possible escape; a parameter with no claim at that path cannot
let the resource escape through that callee, so the caller still owes the
release. Nothing here is an ownership transfer: the strict `Stored` bit is
untouched.

## Some masks must be exact; others may guess

A discharge summary can also follow a bound method passed to a visible helper:
`invoke(resource.Close)` counts when the exact callback is invoked on every
normal return. The summary builder requires one exact receiver capture and a
synchronous call. Conditional invocation, replacement callbacks, another
receiver, and asynchronous launches do not establish this guarantee. This
reuses the local completion proof rather than treating every callback argument
as cleanup.

How sure a mask has to be depends on what reads it, and keeping that straight
is what keeps the model sound:

- A mask that **carries a proof forward** — `ReturnedOwner`, `Stored`, and
  every discharge verb — has to be exact. If it were a guess, a later step
  could build a wrong conclusion on top of it.
- A mask that can only **hide a diagnostic** — `Retained` — is allowed to
  guess on the cautious side. An extra bit can suppress a warning but can never
  invent one. Consumers use it to fall back to `unknown`, never to prove
  ownership.

When you add a mask, decide which of these it is first, and write down which
way consumers are allowed to lean on it.

## Proven, disproven, or unknown

### Possible identity is not a guarantee

`heapmodel.MayAlias` follows possible origins: one matching phi alternative or
a value previously stored in a cell can match. Use that evidence for possible
consumption and conservative escape handling, not to establish a guaranteed
action. `heapmodel.DefinitelySameValue` requires agreement across alternatives
and does not equate separate loads from potentially mutable storage. A failed
definite match means unknown identity, not proven inequality.

Imported exact-argument matching, callback-invocation summaries, and
unchanged-return proofs use `heapmodel.Storage` to resolve local loads before
requiring definite identity. The query requires agreeing reaching writes, checks
address escapes and competing writes, and preserves the time of aggregate
copies and saved reads. `Content` observes before an instruction; `StableContent`
also rejects subsequent mutation outside that instruction, for callback
bindings whose accesses are independently checked. Callback field and element
resolution share one stable-content agreement step: all selected addresses must
yield the same exact SSA callback value at the same observation. Field selection
and array index coverage remain separate. A different or unavailable value
supplies no completion witness. An ambiguous argument receiving
a lifecycle summary is unknown rather than proven cleanup. Access-path
identity describes corresponding storage locations beneath already-matched
roots; it does not by itself establish that their contents are unchanged.

A consumer never reads a fact as a plain yes/no. `LifecycleEvidence.Prove`
returns one of three answers:

- **proven** — the function was summarized and the bit is set.
- **disproven** — the function was summarized and the bit is clear.
- **unknown** — no fact exists (not exported, too many parameters, not
  analyzed, or reached through dynamic dispatch).

Absence is never disproof. A callee with no fact is opaque, and an opaque
consumption suppresses the diagnostic. Any tool that surfaces facts —
including `gohawk dump facts` — must keep the summarized-versus-absent distinction
visible for the same reason.

`LifecycleEvidence.CallEffectsWithin` exposes the separate local call-effect proof
and traces its read, mutation, retention, async, and invocation evidence. A
clear `Retained` bit is not a read-only guarantee. This query requires a visible
body and does not synthesize effects from missing lifecycle-summary bits; see
[Local storage model](storage-model.md).

`CallEffectsWithin` charges the same effect visits to a caller allowance while
retaining the local query cap. A shortened query remains unknown even when
the caller has budget left. A nil budget selects an independent default
allowance; fact encoding and effect polarity do not change. All production
consumers now pass their allowance, so the unused default-only broker method
has been removed.

`ClaimAsynchronouslyExposes` reads positive `HeapEscapedAsync` effects for an
exact parameter object from the existing heap projection. It is a may-claim:
the asynchronous handoff need not happen on every return. A child-field effect,
synchronous opaque call, missing heap, or truncation alone cannot set it.
The claim adds no serialized mask or fact version. Resource lifetime uses it
at the same unknown-ownership boundary as positive local async call effects;
it never proves cleanup, a join, or absence of exposure when the bit is clear.

## What the model can express

- A lifecycle action guaranteed on every normal return of the callee.
- Ownership transfer to the result or to an escaping receiver, and its
  opposite, a returned view.
- Invocation of a func parameter (an `InvokeMethod` discharge), and the stricter guarantee that
  it is invoked in the same goroutine before return (a `SynchronousInvokeMethod`
  discharge, read by `SynchronouslyInvoked`). A helper that forwards its
  callback to a callee in the same package reads that callee's invocation
  claims from the summaries this pass has already computed, or proves them
  from an unexported callee's body, because facts are exported only after the
  whole package is summarized. Invocation inference owns its state within the
  package pass and uses `calls.FunctionSummaries` for recursion, memoization,
  and a shared `SummaryBudget`. Both invocation modes use one instruction
  classifier. Recursive or exhausted queries advertise no invocation guarantee
  and do not cache an incomplete answer; a later query with sufficient budget
  can retry. `callback_facts_test.go` pins forwarding, asynchronous invocation,
  conditional and replacement callbacks, cycles, and budget recovery.
- Cleanup that happens deeper in a chain of exported calls, because one
  summary is allowed to read the summaries of the functions it calls.
- Facts on things other than function parameters: `CleanupFact` attaches a
  cleanup contract to a named type, so the mechanism is not limited to functions.
- A new discharge verb, cheaply: one row in the mask table in `fact.go`
  (name, method, field).

## What it can't express, and why

Each limit traces straight back to one of the four things above.

- **Relationships between values** ("param A flows to result B", "which lock
  this unlocks") — *one parameter at a time*. Questions about how two values
  relate belong to a different tool today, not to more bits. The one
  relation the summary does carry is *where* beneath a parameter a cleanup
  happened, as a discharge path; see below.
- **Conditional or partial cleanup** ("closes only on the error path") —
  *every return*. In the Must claims it collapses to a clear bit, which looks
  the same as never. The fact's cases carry the conditional part a caller
  can select: cleanup on the returns matching a result condition, or on the
  paths a constant Boolean argument allows; see "Summary cases" below.
- **Per-call answers** — *once per function*. There is one fact, however
  differently two call sites use the function. Argument cases are the one
  exception: a call that fixes a guarding Boolean parameter to a constant
  selects the case for that value.
- **Panics and other abnormal exits** — *normal returns only*. A callee that
  closes on every `return` but leaks when it panics still counts as `Closed`.
  A panic-only or non-returning body does not count as invoking, releasing, or
  transferring anything merely because it has no contrary return. Action
  masks require a positive action witness and a normal return; returned-owner
  claims also require a reachable normal return. In particular, an unimplemented
  panicking method cannot become a type's cleanup contract.
  Body-based returned-owner searches share `ssaflow.ReturnsOnlyNilOrErrors`
  for unsuccessful-construction return shapes. Exact builtin error identity
  includes aliases; an unrelated nonnil owner result or a separately declared
  error-like type does not satisfy this exception. The shape alone proves
  neither failure nor cleanup. `returned_owner_errors_test.go` pins the alias
  case and the existing boundary for delegated tuple results.
- **Interface calls** — *named callee only*. A call through an interface has no
  fact and is `unknown`, unless the method name matches a documented cleanup
  contract such as `Close`.
- **Unexported functions, and more than 64 parameters** — a fact is only
  exported for exported functions; package internals are the local proof's job.
- **Counts and ordering** ("adds two", "closes after waiting") — a fact is
  *just bits*.

Most of these limits are the cost of keeping the summary small, cacheable, and
precise; they are deliberate choices in the current model rather than rules
fixed for all time. Only extend the model for a real, reusable pattern whose
values and paths the analyzer can pin down exactly — a single false positive
found while dogfooding is not reason enough.

## A contract on a type, not a function

`CleanupFact` is the one fact attached to a named type rather than to a
function. It exists because only the closing contract can be read from a method
set: `io.Closer` is documented and carries a signature to check, while a bare
`Stop`, `Shutdown`, or `Release` on a project type is a guess about what the
name means.

The release itself is not a guess, and it is visible in the package that
defines the type — a constructor takes ownership of resource fields, and a
method releases them. Those are already proved as `OwnedFields` on the
constructor and `ReleasedFields` on the method, so the contract only joins them
onto the type:

- `Owned` is what the type's constructors took ownership of.
- `Released` is what its methods release together.
- `Methods` names the methods that release anything.

A contract is exported only when `Released` covers `Owned`. A partial release
is not a contract: reading one as complete would discharge an obligation that
still stands. A type whose methods could not be summarized claims nothing,
which is the same answer as a type that releases nothing — absence is not
disproof here either.

The direction matters. This fact is provable because it flows the way facts
flow, from the defining package to its importers. A question that needs
information from a package's *callers*, such as whether a method can run in
more than one goroutine, cannot be a fact at all.

## Where facts live

### Local compositional summaries

A summary describes what a function does. A fact is a mechanism for exporting
evidence between package-analysis runs; the two are not competing models or
unary-versus-binary relations. The lifecycle facts above are themselves one
family of summaries.

For visible SSA bodies, `calls.FunctionSummaries` shares the mechanics of
composing function summaries: memoization, recursion guards, budget handling,
and direct-call parameter and capture bindings. Each instance has one fixed
analysis policy. Its symbolic answers are immutable; binding an answer to a
caller must not change the cached summary. Budget-shortened and recursively
cut computations are not retained as completed answers.

Consumers keep their own evidence models:

- `lockorder` composes positive lock-acquisition witnesses with call-site
  provenance. An unavailable callee contributes no witness, **not** proof that
  it acquires no locks. Its bounded search does not invent ordering edges when
  it runs out of work budget.
- `goroutineownership` helper-use and `cancellationownership` helper-use use
  context-keyed summaries: the target parameter and, where relevant, tracked
  obligation kind distinguish questions about the same helper.
- Lifecycle retention distinguishes target parameters and strict/may-retain
  modes. Storage call-effects preserve observed effects while marking an
  incomplete answer unknown; neither treats missing effects as purity.

These context-keyed consumers use `CallGraphMemo.Summarize`, the same driver
underneath `FunctionSummaries`. Callback-invocation and enclosing-completion
queries also use it. The driver permits each evidence family to preserve
independent positive witnesses after exhaustion without caching an incomplete
answer or silently claiming completeness.

Some completion and delegated-field searches intentionally separate the cache
key from the guarded body: one question can visit several possible callees or
map a value into several parameters. They use `CallGraphMemo.Compose` for the
cached question and `CallGraphMemo.WithFunction` for each callee's scope,
preserving that distinction without pairing raw guard entry and exit.
`CallGraphMemo.Incomplete` records policy-specific truncation, such as revisiting
a callback value; recursion and budget invalidation are automatic. Resource
cleanup and other lifecycle analyzers consume these shared searches; they do not need
an additional analyzer-local summary engine. Target values and callback
environments must remain in their keys—function identity alone is insufficient.

This infrastructure is neither a scheduler nor a model checker. The generic
summary driver does not merge branches or infer conditional effects. Exporting a summary requires a
separate evidence model, serialization, and precision fixtures. In particular,
finite recursion guards alone do not
guarantee cheap analysis: cut answers cannot be cached, so consumers charge
instruction and effect-expansion work to a shared search budget.

### Summary cases

A summary case is a positive cleanup guarantee under a condition a caller can
check. Cases are the discharges with a non-empty condition, in the same
`Discharges` list as the unconditional claims, and each is proved by
`lifecycle.ProveCompletionForCase`.
The condition is an `calls.CallCondition`, the one serializable condition
type every conditional summary uses, and `Matches` is the one rule selecting a
case at a call. A
case names a result condition (a Boolean result true or false, or an error
result nil or non-nil), Boolean parameters fixed to constants, or both, and
records the method or synchronous callback invocation it guarantees on the
exact parameter, with the path beneath it when the cleanup settles a field.
The unconditional discharges and every other Must claim never inherit a
conditional guarantee. Missing cases are
unknown, not absence of effects: a case says what happens under its
condition, never what does not.

#### Result conditions

`lifecycle.ProveCompletionOnEdge` connects a synchronous helper's Boolean or
nil-error result to cleanup of an exact caller value. The completion summary
is keyed by the selected result and condition as well as its callback context
and target. A direct forwarding return composes the same condition through
the next helper; unrelated nested calls still require unconditional cleanup.

Resource lifetime and cancellation ownership credit this evidence only on the
tested control-flow edge. The opposite edge and earlier returns remain subject
to their own obligations. Partial cleanup, wrong targets, asynchronous cleanup,
opaque dispatch, recursion, and exhausted searches cannot establish completion.
An unknown returned value is treated as possibly satisfying the condition, not
discarded to manufacture a proof. Compiler-spilled Boolean results can use the
shared storage proof; error boxing is preserved so typed nil errors are not
mistaken for nil interfaces.

An exact external resource-state contract can seed the same relation. For
example, `database/sql.Rows.NextResultSet` closes its receiver before returning
false. `resourcemodel` verifies that the receiver is the exact target; the
conditional lifecycle summary can then carry this false-result release through
a forwarding wrapper. The local resource model can also bind a visible
straight-line owner's field to an exact caller value using the existing
storage/heap identity proof. It does not publish an owner-field conditional
relation across packages yet. A true result, a different receiver, and
`Rows.Next` do not inherit the false-result guarantee.

#### Argument conditions

A call that passes a Boolean literal, nil, a value that is never nil (an
allocation, a made map, slice, channel, or closure, a function, or an
interface box, which is non-nil even around a nil pointer), or a caller
value the caller's own call already fixed, decides every branch in the
callee that tests that parameter: a Boolean directly or negated, a nilable
value compared with nil. `calls.ProveFixedArgumentsWithin` binds the callee's
parameters at each call the completion search enters, as `calls.FixedValues`,
and the obligation walk's `Constants` narrows a decided branch to its arm, so
a helper that closes only under `!keep` completes the target at `finish(f,
false)`, and one that closes only when `options == nil` completes it at
`close(f, nil)`. Nilness is bound only for a parameter the callee compares
with nil or captures, so an untested pointer argument adds no binding. The
binding reaches a value a deferred closure captures: Go captures by reference,
so the closure's free variable is a cell, bound only when
the once-stored census proves all captures are read-only and the exact store
dominates closure creation. `ssaflow.WrittenOnceCellWithin` alone supplies identity
after its store. `ssaflow.WrittenOnceCellAtWithin` combines that census with
instruction dominance at the caller-selected observation; fixed-argument
binding selects closure creation, while concurrency spill paths select the
actual load. These uses share ordering mechanics without imposing one universal
capture or invocation policy. A sole assignment after registration or
on only one incoming path cannot fix earlier reads. This includes a named
result assigned by a later return: its outcome is supplied by the existing
caller-fixed return analysis, rather than inferred from that later store.
Late initialization before a synchronous call remains unbound by this
creation-based contract unless the caller supplies the current cell outcome.
Any other comparison, a phi, or a derived value decides nothing.
The memo keys every body by the values fixing its parameters, because the
same call can complete under one binding and not another. The result facts'
parameter-nil cases are proved by the same binding.

For callers in other packages, export proves each case with its parameters
bound. Only guarding parameters are considered: Boolean parameters that reach
a branch, a call, or a captured cell, and nilable parameters the body compares
with nil, directly or in a closure; at most two of them, so a function has at
most eight assignments. A case implied by a proven case with fewer
assumptions is not repeated, and a case with no result condition answers any
result condition. An importing call selects every case whose assumed
constants it supplies, including constants its own enclosing search fixed,
and matches cleanup paths exactly as it matches Must discharges. A function
outside the bound keeps only its unconditional and result cases, which is
the summary it had before argument cases existed, so a constant call through
it stays as it was.

Completion inference shares its request allowance with constant-bound block
census, independent action/return witnesses, feasible-return coverage and
conditional work-list states and successor selection. Exact-type coverage keeps
its existing anywhere witness before the nonnil/type-constrained return walk;
both phases spend the same allowance. Interrupted reachable-block census is
discarded. An interrupted case reports budget unknown and cannot become a
published discharge, even if cleanup was witnessed before the cutoff. Ordinary
completion memoization likewise discards shortened answers and a fresh request
can retry. A summary lookup can exhaust before body discovery; that cutoff
retains its reason without inventing local-body provenance. A completed opaque
metadata lookup which spends no steps stays unavailable, even at a zero limit.
Type-system and graph construction costs remain independent.

Export examines at most four result slots and two guarding parameters with one
shared 2,000-step budget per function. Independently proved cases may survive
exhaustion; an interrupted proof never becomes a guarantee.
`LifecycleEvidence.CompletionOnEdge` binds local and imported evidence to the
caller's tested branch with exact identity.

This is not a general conditional effect language: comparisons with integer,
string, or other constants, relations between arguments, and independently
returned worker handles remain outside it.

### Returned cleanup and completion handles

`lifecycle.ProveReturnedCleanup` relates a callback result to an exact parameter
or sibling result from the same factory invocation. Every return must supply
a callback that performs the requested method or invokes the target callback.
Forwarding factories compose this relation; the versioned `ReturnedCleanup`
lifecycle fact preserves it across package boundaries without claiming that
calling the factory itself performs cleanup.

Resource lifetime and cancellation ownership consume these relations when the
callback is actually called or deferred. Goroutine ownership can credit a
returned waiter for an already-established WaitGroup completion obligation.
Closing or stopping a worker's resource remains shutdown participation, not
proof of a join. This does not infer arbitrary worker obligations hidden inside
factories or relate independent calls by their names or types.

Wrong siblings, callbacks from another invocation, mutable captures, no-op
alternatives, asynchronous invocation, recursion, and exhausted searches do
not prove completion. Export considers four result slots and shares a
2,000-step budget per factory. Missing records remain unknown.

### Ordered concurrency facts

`internal/analysis/passes/concurrencyfacts` shares the complete ordered-effect model
used by the ordered helper path in `lockorder` and the helper-effect paths in
`goroutineownership` and `producerlifecycle`.
It records channel send/receive/close, `WaitGroup.Add(1)`/`Done`/`Wait`, and
`sync.Mutex.Lock`/`Unlock` and distinct `sync.RWMutex` read/write events, including completion and unlock defers in
execution order. The generic summary infrastructure still owns caching,
recursion guards, and budgets; each analyzer owns its defect or hazard proof.
Deferred helpers can contain several close, group-completion, or unlock events.
The deferred stack reverses helper registration order without reversing the
events inside a helper. Acquisition, communication, incomplete effects, and
unstable captured bindings still prevent a deferred summary.

Its versioned `Fact` serializes event kinds, formal parameter positions, and
up to four parameter-relative child-launch templates, with the receiver at
position zero. A complete empty fact is positive evidence
of no supported synchronization effects, not the fallback for a missing fact.
Only complete linear summaries with exportable identities are exported. Acyclic
branches merge when ordered effects and pending defers agree. Local inference
can otherwise preserve up to eight complete alternatives, sharing the same
work budget; those alternatives are not exported as linear facts. Graph
consumers must prove their property on every variant. Opaque calls, launches within a worker, resource escapes,
local resource allocations, unexportable captured resources, recursion, and
exhausted budgets make export unavailable. Facts do not encode arbitrary
conditions or schedules.

Imported events and child templates are bound to exact actual arguments and
retain their order. Each call instantiates a separate child; the launch site is
the importing call site. Token positions and SSA pointers are never serialized.
Transitive exports remap the effects to the forwarding function's own parameters.
The field-path format also exports
embedded mutex field paths, up to eight fields deep. Binding requires an exact
existing caller address. Mutable pointer-field dereferences, concrete global
identities, and local allocation identities are not exported. Whole-owner
stores containing embedded synchronization state invalidate completeness.
Local templates can forward a receiver-relative field through a helper that
does not select it itself. Root and call-site queries materialize these
projections to an existing caller address before exposing bound evidence.
Version 6 adds explicit RWMutex read modes; two read acquisitions are not
treated as mutually exclusive. Version 7 adds `Invoke` effects: a call through
a function-typed parameter is published as a hole at its position, and the
importing call site fills it with the function or closure it supplies. Version 8 adds
bounded path alternatives for functions whose branches have different
effects: each carries its conditions by parameter position, numbered opaque
inner conditions, and the constants or non-nil values it returns. Version 9
adds interface holes: an `Invoke` effect on an interface-typed parameter
names its method, and the importing call site fills it with that method of
the one concrete value it boxes. Only exported methods are published.

Local composition also resolves exact interface boxes through shared dispatch
evidence, preserving the concrete receiver's argument position. Loaded channel
fields can bind through a fresh receiver only after the heap model proves the
slot stable across all visible uses; these indirect field templates are not
exported. The richer local query can expand exact constant-count loops under
its worker and event budgets. The linear publication cache still declines
loop expansion, rather than charging every exported function for this policy.

All fact access belongs to this prerequisite, which exposes an engine rather
than raw facts to consumers. Public queries serialize access to the shared
cache and recursion guard because sibling analysis passes may run concurrently;
each query supplies its own budget. Its export work has a 2,000-step budget per
exported function and a 32-operation limit. Unknown summaries never become
absence proofs, and budget-shortened answers never become completed cache
entries. The mixed-dependency checks remain experimental.

Consumers need not require a straight-line root function.
`goroutineownership` uses exact receives and waits as positive joins for
already-established obligations, including inside its branch-aware helper
search. It does not replace its existing proof with summary absence.
`lockorder` carries complete helper lock state into subsequent instructions.
`producerlifecycle` expands both sends and receiving helpers, and treats opaque
channel consumers as unknown rather than as zero receives.
For competing workers with incomplete protocol summaries, it reuses the shared
resource-specific call-effects proof to establish non-receiving uses when possible.

### Bound callbacks within one package

The local completion search carries invocation-specific bindings for direct
function arguments. A visible helper calling `fn(resource)` can resolve `fn`
to the function supplied by its caller and map `resource` onto the callback's
parameter. Forwarding through another visible helper preserves that binding.
The same search still requires the requested cleanup coverage; merely passing
or storing a callback does not establish completion.

Bindings preserve lexical capture environments through nested closures. A
captured local cell must have one dominating initialization and only read-only
uses. Function-valued fields and fixed-size slice elements can be resolved
through unchanged local aggregates; a dynamic index requires every slot to
contain the same callback. Writes through helper parameters, opaque escapes,
partial slices, and ambiguous elements prevent a proof. Passing an aggregate
resource argument still uses the existing parameter/field identity mapping.

Bindings are part of the memoization context and share the search budget and
recursion guard, including storage-use scans. Unresolved callback dispatch
produces unknown rather than a disproof just because the containing helper has
a visible body. Callback arguments selected through phis, returned by factories,
or dispatched through interfaces remain outside this binding resolver.

This does not enumerate all callers, prove iteration over mixed callback
collections, or add cross-package relational facts.

An enclosing-scope completion query can start at a literal's lexical owner and
follow these bindings to its invocations. Every visible invocation must receive
the exact value covered by a dominating deferred cleanup registration. Known
`testing.T.Run` and `Cleanup` callbacks retain their enclosing context. Escapes,
unsupported dispatch, mutable wrapper fields, and exhausted budgets stop the
proof. Receiver mappings use exact identity rather than aggregate containment
or a phi that merely includes the target. This is bounded local context
tracking, not whole-program caller enumeration.

Resource lifetime uses that query only for statements prepared through
`database/sql.DB`: connection finalization closes its tracked driver statements.
It does not equate `DB.Close` with `Stmt.Close`, or discharge rows, transactions,
or statements prepared through `Conn` or `Tx`.

Cancellation ownership uses the same completion search with `InvokeTarget`
instead of a method name. This mode requires exact function identity, not
aggregate containment or a phi that merely includes the target. Stable captured
cells and direct helper forwarding are supported; asynchronous invocation and
exhausted searches remain unknown. The analyzer retains its own policy for
transfer, registration, and opaque handoffs. A function being stored or passed
to a helper is not itself proof that it was invoked.

### Fact package boundary

Facts are imported and exported (`analysis.Pass.ImportObjectFact` and
`analysis.Pass.ExportObjectFact`) only inside the package that defines the fact
type. Consumers use `lifecyclefacts.LifecycleEvidence`, which checks local evidence
first and imported facts second, all on one path. A second fact family with a
different evidence model — `concurrencyfacts`' ordered effects — gets its own package
for the same reason: one vocabulary per family.
`TestObjectFactsStayInTheirDefiningPackage` enforces the boundary.

Return-specific result-guard completion also shares its request allowance with
named-cell store selection and outcome lookup. Guard registration uses the
shared structured reachability proof; interrupted registration or binding is
unknown. This does not change declaration guarantees or make a bare return's
value available when its store is outside the return block.

Cancellation's result-guard capture filter shares discovery's allowance with
once-stored-cell identity and exact deferred-reader registration. The selected
guard list is published only when the full filter completes. Store and defer
classification likewise retain unknown at capture cutoff. Shared cell identity
still answers identity only; it supplies no cleanup or registration guarantee.

Fixed-argument binding publishes `FixedArgumentsProof` only after parameter,
capture, read-only, once-stored-cell, ordering and nil-relevance searches
complete under the request allowance. Cutoff discards every binding, including ones collected
before an interrupted later argument. Completion stops before callee coverage
or memoization can consume a partial map. A missing body yields an empty
metadata census without local SSA provenance; it establishes no behavior.
Nil-budget binding retains the old literal, known-value, typed-nil-interface,
mutable-cell and caller-fixed named-result policies. Summary guard discovery's
default `ComparesWithNil` remains a separate setup query.


### Concurrency spill paths and canonical-load availability

Concurrency field paths retain the read time of a spilled parameter. A unique
parameter assignment names a load only when its store dominates that load.
A saved zero value before initialization is not the parameter stored later.
Multiple-write spills use the existing storage identity query at the load, so
reassignment preserves the old snapshot and names later reads as the new
parameter when proven. Closure roots are bound separately at their invocation.

Field-path reaching walks, spill/capture referrers, initialization order,
canonical load enumeration and nested capture forwarding share the engine's
request allowance. A cutoff supplies no path binding or canonical capture read.
The package-shared canonical field map drops any sentinel or negative result
from an interrupted search; a fresh request can recover the same canonical load.
Complete positive field identities remain reusable.

The actual SSA and child/fresh controls are in
[spill_paths_budget_test.go](../../internal/analysis/passes/concurrencyfacts/spill_paths_budget_test.go).
Existing imported formal-field guarantees and the fact schema remain unchanged.

### Concurrency publication allowance

Export inference, linear publication validation, and a branch-alternative retry
share one 2,000-step allowance per exported function. Local `Declaration` queries
likewise retain the supplied allowance through publication. Cached summaries save
inference work but do not bypass validation. Formal-parameter searches, embedded
field paths, field copies, worker metadata, path conditions and their context
positions, and returned entries spend that allowance. The schema remains version 9;
no caller-specific identity or application target set becomes a declaration fact.

Any failed publication returns only the version marker, never earlier effects,
cancellation requirements, workers or path alternatives. A pre-inferred projection
cannot rescue an interrupted field-path search. Actual SSA cold/cache queries,
child/parent/fresh controls, cancellation requirements, a padded spill path, and
an alternative cut after an earlier path are covered in
[publication_budget_test.go](../../internal/analysis/passes/concurrencyfacts/publication_budget_test.go).

Imported `Declaration` lookup also shares its allowance with metadata copying.
One copy path detaches effects and nested fields, cancellation inputs and worker
effects for both linear and alternative bodies; alternative conditions and
returned metadata are detached as well. Strings and scalar constants remain
immutable values. Empty slices retain their shape, and an absent or incompatible
fact remains unavailable rather than a complete empty declaration. A cutoff
returns no declaration and leaves cached publication data unchanged, so a fresh
query can recover it. Mutation and child/parent/fresh controls are in
[declaration_copy_test.go](../../internal/analysis/passes/concurrencyfacts/declaration_copy_test.go).

Package write-once inventory setup, heap graph and type-system internals,
constant/string formatting, fact encoding and decoding retain their independent
costs. These bounds count evidence steps; they are not a wall-time or memory
guarantee.


## Mutex heap effects and object exclusivity

Direct synchronous `sync.Mutex` and `sync.RWMutex` locking/unlocking methods,
including the try methods, mutate their internal receiver state without
retaining an enclosing user object. The heap model applies that language-library
contract at its existing known-call boundary, invalidating the exact mutex slot
through summary substitution's storage mechanics. It preserves sibling fields
and previous exposure. Launched calls, opaque interface receivers, `RLocker`
adapters and project-defined lookalikes do not use this contract. No cleanup,
completion, try-result or field-protection guarantee is inferred from it.

Exactly registered deferred calls use the same known-call dispatcher at
`RunDefers`, in reverse registration order, with the receiver and arguments
evaluated at registration. Conditional or repeated registrations remain opaque;
the known contract cannot bypass that boundary. Unmodeled calls retain the
existing summary lookup and conservative invalidation. Builtins use the same
execution boundary: exact defers apply their storage effects at `RunDefers`,
while asynchronous calls cannot establish synchronous contents or exclusivity.

For `sync/atomic.Pointer` and `sync/atomic.Value`, `Store` and `Swap` replace
contents on normal completion. `CompareAndSwap` retains both the prior contents
and the possible replacement, using the existing weak-store mechanics. Its heap
publication cannot claim that the replacement is the only value at exit. The
contract establishes neither CAS success nor the value returned by `Swap`.
Asynchronous atomic calls and project-defined lookalikes remain opaque.
Compiled SSA controls in `store_calls_test.go` cover direct/deferred updates,
publication, captured values and uncertain execution.

Synchronous `clear` forgets the selected collection storage through the existing
summary invalidation path. It neither exposes the collection nor recursively
clobbers objects formerly held in its elements, and it establishes no exact
zero value or view extent. `copy` retains its existing possible-element writes;
deferred copy uses the slices evaluated at registration. Deletion retains
possible map-content history rather than establishing exact element absence.

Invalidating a sub-slot also drops whole-aggregate content above it. A later
aggregate load cannot reuse the pre-write value. Subtree copies carry unknown
write stamps as well as content and backing copies, so later snapshots preserve
unknown fields while earlier snapshots and untouched siblings remain intact.
`store_calls_test.go` covers these boundaries and asynchronous,
conditional and repeated builtin execution.

SSA scalar stores and imported heap edges share one selected-slot update in
`store_regions_writes.go`. Exact replacements clear prior contents; possible
updates retain prior/replacement possibilities and invalidate cached enclosing
aggregates. Wildcard elements use the existing weak-element mechanics, even
when an edge says Must: an uncertain selection cannot establish one exact slot.
A first wildcard update retains existing elements and the unwritten nil or
foreign possibilities before unioning the replacement. An unknown index cannot
establish the contents of a particular untouched element; known-index strong
writes keep their exact replacement guarantee. `store_writes_test.go`
covers fresh and caller-supplied arrays. Earlier snapshots and untouched siblings
stay intact. SSA destination exposure
and foreign-write invalidation remain outside this update, as do imported
escape metadata, result binding and aggregate-value writes. The compiled
summary controls are in `store_writes_test.go`.

A possible or undescribed aggregate replacement invalidates both the selected
concrete contents and cached aggregates above it. An unknown write stamp alone
cannot override an earlier concrete field entry. The shared stored-slot
invalidation also serves summary forgetting of local storage; foreign epoch and
closure invalidation retain their separate policies. Definite aggregate copies
remain exact, and a definite zero clears selected fields. Prior snapshots,
untouched siblings and former pointee storage remain intact.
`store_writes_test.go` covers mixed destinations, mixed source values,
exact replacements, zero values and preservation controls.



`ExclusiveAt` asks whole-object identity rather than exact content identity.
Different or unknown selections of one non-stale object can still be private;
unknown/stale/mixed origins cannot. Fresh `make` maps, slices and channels can
be local while their elements remain opaque, provided no selection has been
exposed. Opaque calls and loaded unknown values remain unavailable. The query
lives in `store_exclusivity.go`; exact slot and content policy remain separate.

## Reference capability of SSA results

Shared containment checks distinguish reference capability from provenance.
By-value type traversal follows struct fields, arrays and SSA tuple components;
reference edges remain leaves. Empty and scalar-only result tuples cannot
retain lifecycle owners. Mixed tuples and their reference-bearing projections
can, without proving that any particular result actually owns the supplied
value. Process startup discovery applies this prerequisite before wrapper-owner
completion; call effects remain a separate query about the instruction itself.

## Shared discovery without shared diagnostic policy

Concurrency mutex-root canonicalization and caller binding share one bounded
first-field-load census. It selects the first block-order load with both the
same declared field and exact embedded access path. Canonicalization retains
its write-once inventory, recursive sentinel and cutoff invalidation; caller
binding retains its own canonical-load query. Neither field-name similarity
nor a shared prefix proves stable storage or completion.

Whole-written aggregate derivation shares the read-use classifier for selected
field/element addresses. A store of the whole root is an explicit outer
exception; stores beneath that root and escaping selected addresses remain
opaque. Heap transfer uses shared wrapper operands with its selected four
transparent forms, retaining its separate slice-to-array-pointer and assertion
handling. These are evidence mechanics; analyzer acceptance remains local.

## Shared slot names and argument-consumption mechanics

Current-state edge projection, historical edge projection and escape projection
share one root/path naming query bounded by `SummaryPaths`. Naming does not
establish contents or retention: state slot counts, history membership and
per-return escape coverage remain separate. The helper does not replace target
value naming, reads or requirements; those retain their existing contracts.
Depth controls explicitly preserve forwarded targets beyond the source-slot
bound rather than silently applying one publication rule everywhere.

Lifecycle field-result transfer and returned deferred-cleanup detection share
argument alias consumption. Each argument remains the first alias operand,
matching the graph context chosen by `ProveMayAlias`; the query stops after the
first match, as the prior Boolean short circuit did. Containment is not alias
consumption, and positive consumption is not cleanup or ownership proof. The
field-store and result/defer searches remain their own structural contracts.


### Paired-result consumer allowance

The summaries broker's paired-nilness consumer charges case visits, exact
error-result referrers, the dominating branch census and `SuccessBranchWithin`
to the existing query allowance. Direct `CallResultSource` decoding remains
constant dispatch with no wrapper or alias traversal. Cutoff cannot establish
the paired error's nilness or prune a successor; a fresh query can reuse the
completed callee summary and finish its caller evidence independently.
`paired_nilness_budget_test.go` uses an actual 64-call error provenance chain,
a warmed complete result summary, a child cutoff with a live parent, allowance
sweeps and final feasible-edge recovery. Existing wrong/unguarded result pairs
remain unknown. Fact schemas and declaration guarantees are unchanged.

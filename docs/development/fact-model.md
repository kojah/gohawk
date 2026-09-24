---
title: Inferred facts
description: Modular function summaries, their fact publication, and the guarantees each component can establish.
sidebar:
  order: 3
---

Function summaries are the consumer-facing model. Analysis facts publish
supported summary components across package boundaries; they are not a separate
kind of knowledge. `internal/summaries` brokers access to independently computed
result, lifecycle, and concurrency components. Each domain keeps its inference
and publication in `resultfacts`, `lifecyclefacts`, or `concurrencyfacts`.

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

Function views expose uninstantiated declaration guarantees. The provider's
lifecycle-evidence and concurrency-at-call adapters retain the existing exact
argument, capture, and result binding machinery. A formal parameter mask is not
already a guarantee about an arbitrary caller value. Private lifecycle bodies
without a published declaration summary use the existing local evidence path.

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

## Unconditional result guarantees

The result component records `AlwaysNil`, `AlwaysNonNil`, `AlwaysTrue`,
`AlwaysFalse`, or `Unknown` separately for each result. Guarantees require
agreement across normal returns, including recovery returns, and a return
witness. Absence of a return does not prove termination or any result property.
The initial proof conservatively includes all SSA return blocks; it does not
solve arbitrary path conditions.

Literal results, fresh allocations, interface boxing, agreeing phi alternatives,
and direct forwarding calls are supported locally and across packages. A boxed
typed-nil pointer is a nonnil interface. Loads remain unknown: neither a mutable
package sentinel's initializer nor a named result before deferred modification
proves the value returned later. There are no name-based `errors.New` contracts.
Recursion and exhausted searches produce unknown evidence and do not poison
the summary cache. Queries share a 2,000-step budget in the initial consumer;
export also has that per-function budget and a 16-result limit.

Nilness does not imply ownership. A conditional cleanup guarantee does not
imply that its Boolean result is always true. Unknown never removes a branch.
Existing literal/integer branch evidence remains in place where the result
component does not replace its semantics.

## Result relations

Beside the unconditional guarantees, the result component proves bounded
relations between one result and a parameter or another result, and exports
them with the fact:

| relation | meaning | who uses it |
|---|---|---|
| `FalseWhenParameterNil` | the Boolean result is false on every return reachable when the exact parameter is nil | `resourcelifetime` treats `if failed(err)` as an acquisition-error guard, for local, captured, and imported predicates alike |
| `TrueWhenParameterNonNil` | the Boolean result is true on every return reachable when the parameter is non-nil | reserved for the symmetric guard |
| `NonNilWhenResultNil` | the result is non-nil on every return where the paired error result is nil | the summaries provider prunes `result == nil` below the success arm of that error's check |
| `NilWhenResultNonNil` | the result is nil on every return where the paired error result is non-nil | the provider prunes `result != nil` below the failure arm |
| `ReturnsParameter` | the result is the exact parameter, under the same static type, on every normal return | `resourcelifetime` resolves a cleanup receiver through such a call, so `wrap(file).Close()` settles `file` and a later use of `file` is a use after release; `lifecyclefacts` uses the same mechanic to keep an unchanged return from becoming a view |

`ReturnsParameter` is exact storage identity: a wrapper, an interface
conversion, or a value chosen between the parameter and something else is not
the parameter. A parameter relation walks the body under the assumption and requires the
expected literal, or a nil comparison of the exact formal decided by the
assumption, on every reachable normal return. A result relation checks every
normal return and lets a return that forwards both positions of one call
inherit that callee's relation. Both need a return witness on the assumed
side; a missing relation says nothing about the opposite implication.

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
hook of `ssaflow.InstructionTerminatesWith` and `NormalReturnReachableWith`:
a path that calls such a function ends there, exactly as it ends at
`os.Exit`, so an early return behind the call is not reached.

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
`internal/passes/lifecyclefacts/fact.go` by `go generate ./...`, so the mask
list is always the one the code has; do not edit it by hand.

<!-- gohawk:generated-fact-fields:start -->
```go
// Fact is the compact cross-package ownership summary exported for a
// function. Each bit identifies an SSA parameter position. This package is
// internal analysis infrastructure, not a public extension API.
type Fact struct {
	Invoked			ParameterMask
	SynchronouslyInvoked	ParameterMask
	Closed			ParameterMask
	Finalized		ParameterMask
	Released		ParameterMask
	Shutdown		ParameterMask
	Stopped			ParameterMask
	Waited			ParameterMask
	Committed		ParameterMask
	RolledBack		ParameterMask
	ReturnedOwner		ParameterMask
	// ReturnedView narrows ReturnedOwner: the parameter is stored in the
	// returned struct, but no method of that type releases the field, so the
	// caller keeps the obligation. See fields.go.
	ReturnedView	ParameterMask
	// Retained marks parameters the callee may keep beyond the call; see
	// retention.go for the over-approximation it deliberately makes.
	Retained	ParameterMask
	// Stored is the strict form of Retained: positive structural evidence that
	// the callee keeps the parameter, safe to treat as an ownership transfer.
	Stored	ParameterMask
	// Kept widens Retained to what is loaded out of a struct-shaped
	// parameter, by access path, so a caller can ask whether the resource it
	// stored at one path may outlive the call. See contents.go.
	Kept	[]Kept
	// LoopReleased marks parameters whose derived values the callee releases
	// inside a loop, as a variadic close helper does to each of its files. It
	// is a may-claim: which element an iteration releases is decided by
	// iteration, so a consumer treats the call as unknown, never as settled.
	LoopReleased	ParameterMask
	// OwnedFields and ReleasedFields are indexed by struct field, not
	// parameter; see fields.go for the constructor and method summaries.
	OwnedFields	ParameterMask
	ReleasedFields	ParameterMask
	// OwnedResults is indexed by result position: the function hands back a
	// fresh resource it acquired itself, and the caller owes its cleanup.
	// See owned_results.go for the freshness the proof requires.
	OwnedResults	ParameterMask
	// Discharges are the exact cleanup claims: which method is called, on
	// which parameter, at which access path beneath it, on every normal
	// return. The method masks above are the empty-path discharges; a
	// cleanup of a field or element is recorded here and nowhere else, so a
	// caller matches the resource it stored at that path rather than any
	// resource the argument contains.
	Discharges	[]Discharge
	ReceiverStore	ParameterMask
	// Conditional holds positive, result-specific guarantees. It never widens
	// an unconditional mask, and missing entries do not establish no effect.
	Conditional	*ConditionalSummary
	// Heap is the projection of the function's points-to graph onto what a
	// caller can name: where each parameter, result, and global slot may
	// point at exit, how each object escaped or was released, what was
	// read, and where the projection was cut. See heap.go.
	Heap	*heapmodel.HeapSummary
	// ReturnedCleanup relates an invoked callback result to an exact factory
	// parameter or sibling result. Merely returning the callback does not clean up.
	ReturnedCleanup	*ReturnedCleanupSummary
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
cell is only ever written whole.

## Three answers to "what happened to my value?"

| mask | guarantee | what it means for the caller |
|---|---|---|
| a discharge verb (`Closed`, `Waited`, …) | always | the obligation is finished — a join |
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

A discharge verb on a parameter is exact about the parameter itself: `Closed`
on `file` means `file.Close()`. A cleanup of a field or element of the
parameter is not the same claim, and it is not recorded on the mask. It is
recorded in `Discharges` as the method, the parameter, and the access path
beneath it, such as `field:0` for `j.out.Close()` or `index:1` for
`files[1].Close()`, including through the cell a by-value parameter is
spilled into. Each path is proved on every normal return on its own.

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
reads them through `ArgumentMethodsRequired`; the use-after-release check
treats a helper call whose requirement names an invalidating method of
the released resource as the use. See [Preconditions](../preconditions/).

An escape is recorded per slot, not per object: the address of a field
handed to a callee escapes what that field holds and everything beneath it,
never the object above it. A receiver whose embedded mutex is locked is not
thereby stored. Map keys escape as surely as map values, because a range
hands them back.

## Serialization

Every fact type encodes itself as JSON inside the gob stream go/analysis
uses, through `internal/passes/factcodec`. gob compiles a decoding engine
per type for every stream it opens, and the analysis test harness
round-trips every inherited fact through a fresh stream, so a summary with
many fields cost more to compile than to decode; a byte slice costs gob
nothing. The lifecycle pass also exports no summary for a function proven
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

`ssainfer.MayAlias` follows possible origins: one matching phi alternative or
a value previously stored in a cell can match. Use that evidence for possible
consumption and conservative escape handling, not to establish a guaranteed
action. `ssainfer.DefinitelySameValue` requires agreement across alternatives
and does not equate separate loads from potentially mutable storage. A failed
definite match means unknown identity, not proven inequality.

Imported exact-argument matching, callback-invocation summaries, and
unchanged-return proofs use `ssainfer.Storage` to resolve local loads before
requiring definite identity. The query requires agreeing reaching writes, checks
address escapes and competing writes, and preserves the time of aggregate
copies and saved reads. `Content` observes before an instruction; `StableContent`
also rejects subsequent mutation outside that instruction, for callback
bindings whose accesses are independently checked. An ambiguous argument receiving
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
including `gohawk facts` — must keep the summarized-versus-absent distinction
visible for the same reason.

`LifecycleEvidence.CallEffects` exposes the separate local call-effect proof
and traces its read, mutation, retention, async, and invocation evidence. A
clear `Retained` bit is not a read-only guarantee. This query requires a visible
body and does not synthesize effects from missing lifecycle-summary bits; see
[Local storage model](../storage-model/).

## What the model can express

- A lifecycle action guaranteed on every normal return of the callee.
- Ownership transfer to the result or to an escaping receiver, and its
  opposite, a returned view.
- Invocation of a func parameter (`Invoked`), and the stricter guarantee that
  it is invoked in the same goroutine before return (`SynchronouslyInvoked`).
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
  *every return*. It collapses to a clear bit, which looks the same as never.
- **Per-call answers** — *once per function*. There is one fact, however
  differently two call sites use the function.
- **Panics and other abnormal exits** — *normal returns only*. A callee that
  closes on every `return` but leaks when it panics still counts as `Closed`.
  A panic-only or non-returning body does not count as invoking, releasing, or
  transferring anything merely because it has no contrary return. Action
  masks require a positive action witness and a normal return; returned-owner
  claims also require a reachable normal return. In particular, an unimplemented
  panicking method cannot become a type's cleanup contract.
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

For visible SSA bodies, `ssaflow.FunctionSummaries` shares the mechanics of
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

### Result-conditioned local completion

`ssainfer.ProveCompletionOnEdge` connects a synchronous helper's Boolean or
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

The lifecycle prerequisite exports these positive relations separately in the
versioned `Conditional` portion of its fact. Each record names a result slot,
Boolean/error outcome, exact parameter mask, and method or synchronous callback
invocation. Forwarding wrappers can compose imported records. The ordinary
`Closed`, `SynchronouslyInvoked`, and other unconditional masks never inherit a
conditional guarantee. Missing records are unknown, not absence of effects.

Export examines at most four result slots with one shared 2,000-step budget per
function. Independently proved records may survive exhaustion; an interrupted
proof never becomes a guarantee. `LifecycleEvidence.CompletionOnEdge` binds
local and imported evidence to the caller's tested branch with exact identity.

This is not a general conditional effect language: arbitrary argument predicates
and independently returned worker handles remain outside this relation.

### Returned cleanup and completion handles

`ssainfer.ProveReturnedCleanup` relates a callback result to an exact parameter
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

`internal/passes/concurrencyfacts` shares the complete ordered-effect model
used by the ordered helper path in `lockorder` and the helper-effect paths in
`channelsafety`, `goroutineownership`, and `producerlifecycle`.
It records channel send/receive/close, `WaitGroup.Add(1)`/`Done`/`Wait`, and
`sync.Mutex.Lock`/`Unlock` events, including completion and unlock defers in
execution order. The generic summary infrastructure still owns caching,
recursion guards, and budgets; each analyzer owns its defect or hazard proof.
Deferred helpers can contain several close, group-completion, or unlock events.
The deferred stack reverses helper registration order without reversing the
events inside a helper. Acquisition, communication, incomplete effects, and
unstable captured bindings still prevent a deferred summary.

Its versioned `Fact` serializes event kinds and formal parameter positions,
with the receiver at position zero. A complete empty fact is positive evidence
of no supported synchronization effects, not the fallback for a missing fact.
Only complete summaries with exportable identities are exported. Acyclic
branches merge only with identical ordered effects and pending defers; each
block is visited once, without path enumeration or conditional summaries.
Divergent branches, opaque calls, nested launches, resource escapes, local resource
allocations, captured resources, recursion, and exhausted budgets make export
unavailable. Facts do not encode arbitrary conditions or schedules.

Imported events are bound to exact actual arguments and retain their order.
Evidence from a dependency is attributed to the importing call site; token
positions and SSA pointers are never serialized. Transitive exports remap the
effects to the forwarding function's own parameters. Version 2 also exports
embedded mutex field paths, up to eight fields deep. Binding requires an exact
existing caller address. Mutable pointer-field dereferences, concrete global
identities, and local allocation identities are not exported. Whole-owner
stores containing embedded synchronization state invalidate completeness.

All fact access belongs to this prerequisite, which exposes an engine rather
than raw facts to consumers. Public queries serialize access to the shared
cache and recursion guard because sibling analysis passes may run concurrently;
each query supplies its own budget. Its export work has a 2,000-step budget per
exported function and a 32-operation limit. Unknown summaries never become
absence proofs, and budget-shortened answers never become completed cache
entries. The mixed-dependency checks remain experimental.

Consumers need not require a straight-line root function. `channelsafety`
uses complete call effects as close/send witnesses in its existing reachability
proof. `goroutineownership` uses exact receives and waits as positive joins for
already-established obligations, including inside its branch-aware helper
search. Neither replaces its existing proof with summary absence.
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

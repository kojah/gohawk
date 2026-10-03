# lockorder design notes

The public page is [lockorder](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Rationale in detail

### Deferred cleanup through a captured owner

A deferred closure may release an embedded mutex through its captured owner
pointer. The shared lifecycle mapping first proves the captured cell's contents
remain stable through cleanup, then matches the mutex's exact field path beneath
that owner. Writing another owner field does not replace the captured pointer;
unlocking a sibling mutex does not settle the obligation. Reassignment and
opaque mutation of the captured cell establish no cleanup guarantee.

`captured_owner_handoff.go` pins an unchanged owner handed to a callback,
a real missing unlock after that handoff, and a deferred unlock around a loop
that temporarily releases and reacquires the mutex. Shared completion tests
cover sibling fields and owner-cell mutation. The deferred loop is minimized
from [Basecamp's queue drain](https://github.com/basecamp/basecamp-cli/blob/d91fc7b3ae5ee3c54a7fea389f59e791173e647b/internal/connector/queue.go#L264-L290).

### Which locks are compared

Two mutexes held in a package variable are compared as the individual locks
they are. A mutex held in a struct field is compared by the field it is
declared in, because there is no way to tell whether the receiver in one
method is the object another method locks. Ordering is therefore reported
against the declaration: taking one field before another in one place and the
reverse elsewhere leaves the program without a consistent order, even where
today's callers happen to pass different objects.

When local storage proves that a field holds a particular locally allocated
mutex, that allocation keeps its instance identity rather than inheriting the
field's declaration class. This prevents construction-time locking from being
merged with locking of unrelated established objects. Cross-function ordering
after such an allocation is published can consequently be missed.

If a direct field of a fresh local owner was initialized with a fresh mutex,
but escape makes its later loaded identity opaque, the analyzer also declines
declaration-class widening. This is uncertainty, not a freshness or publication
proof. Visible replacement with a shared mutex vetoes this boundary, including
source-visible helper writes to the same field or its address. Opaque code
could still replace the mutex, so some real cross-function cycles are missed.
Initializers of another field do not establish this uncertainty.

Source-visible helper acquisition witnesses retain bounded embedded-field paths
and bind their roots at each call. A fresh caller allocation therefore keeps its
local mutex identity even when only the helper computes the field address.
Distinct formal participants are not merged before binding. Pointer loads remain
exact snapshots, not their mutable cells. A loaded owner may instead have an
exact dominating slot initializer from a source-visible constructor whose every
normal return is its own fresh allocation. This makes the embedded by-value
mutex's declaration identity uncertain; it does not prove private publication.
Visible shared replacements veto that boundary. Pointer-valued mutex fields do
not inherit their owner's freshness. Opaque replacement or a constructor that
publishes its allocation can therefore hide real cycles: this is a documented
coverage loss, not a synchronization proof.

For two mutex fields beneath the same owner type with both participants bound
in one caller, declaration-class ordering requires an exact same-owner
relationship. Otherwise only exact local instance
ordering is retained. Two distinct SSA participants are not proved different or
safe; their relationship is unknown. This intentionally misses cross-function
cycles between different instances of the same owner type. Same-owner field
cycles, exact opposing instances within one function, and ordering between
different owner types remain supported, including bound helper paths. Unbound
callee snapshots retain the existing class policy; they do not identify two
caller participants.

Two locks of the same declaration are not compared. Whether
`transfer(from, to *Account)` conflicts with a caller passing the same pair in
the other order depends on the objects involved, and nothing in the code
settles that, so the pattern is accepted.

A visible pointer-receiver getter that only returns `&receiver.field` keeps
that field's identity across calls. Different receivers and different fields
remain distinct. Getters with branches, loads, side effects, or unavailable
bodies are opaque; the analyzer does not guess that repeated calls return the
same mutex or invent separate held locks from their call sites.

### Ordering cycles and evidence

The ordering check recognizes reversed pairs and longer cycles such as
`A -> B -> C -> A`. It reports one short witness when a new ordering edge
closes a cycle, rather than enumerating every possible cycle. Related diagnostic
locations show both acquisitions on each edge, their `Lock` or `RLock` mode,
and a representative chain of visible synchronous helper calls. These notes
are available in ordinary output and JSON.

Primary cycle messages keep declaration names for fields and globals. Local
allocation identities instead reuse the acquisition's source receiver text,
as the related evidence already does; unavailable source retains the identity
fallback. This affects rendering only, not graph keys or cycle detection.
`gate_mutex.go` pins a published local gate displayed as `gate` rather than its
SSA allocation name, matching [rune's installation gate](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/ide/idepkg/manager.go#L419-L422).
The publication-order precision question at that site remains unresolved.

Longer witnesses exclude iteration-dependent lock identities and are omitted
when a reversed pair within the witness already explains a shorter conflict.
This avoids amplifying pooled-instance uncertainty or repeating an existing
warning as several larger cycles.

The graph still compares declaration classes, not runtime object aliases.
A cycle is a hazard, not proof that those calls run concurrently
on the same objects. Same-class pairs and unclassified locks do not form edges.
Opaque calls, asynchronous calls, and dynamic indexed locks do not contribute
guessed helper acquisitions.

For complete ordered mutex helpers, shared concurrency summaries bind
parameter locks to caller values and retain acquisition/release order. This
works across package boundaries as well as within a package: releasing A
before acquiring B does not create an A-to-B edge. Acquiring B before releasing
A does. Their final held-lock state continues through the caller's later
instructions, supporting later ordering and missing-release proofs. Direct operations and helper effects share the same state transfer.
Incomplete sequences retain the existing conservative class-witness fallback.

Passing an unlock callback to another consumer, or storing it in an owner's
field, makes the prior held-lock state unknown. It does not prove the callback
runs or that the protocol is live. Registering a deferred acquisition also does
not establish that the lock is held in the function body.

A direct unlock whose receiver may alias a held mutex also makes that mutex's
state unknown when their exact identity keys differ. The flow forgets that
held state and its exclusive guard, but does not record an exact release or
an acquire-for-caller contract. This covers a captured owner whose later load
becomes opaque: the heap model retains a possible alias between matching field
paths of the earlier owner and the opaque read. Sibling fields do not alias,
and an opaque read never proves the two receivers identical. This can miss a
real leak when an unlock selects only one of several possible mutexes.

`uncertain_unlock_identity.go` pairs the accepted nested-callback release with
a reported sibling-mutex release. The shared heap tests additionally pin nested
fields, array elements, and the distinction between an owner and its field.
The real-world case is [Centrifuge's final unlock](https://github.com/centrifugal/centrifuge-go/blob/080126041ccc71654718bd0601b920ff8b22a8bf/client.go#L1435-L1762).
Reported-return traces retain distinct observed mutex actions and their identity
keys, rather than representing their merged observations as one execution path.

The flow preserves the incoming control-flow edge when a merged local flag
selects a constant. A path that already released a lock is not combined with
the flag value from a different predecessor. Up to four exact Boolean/integer phi
constants are carried through later blocks and refreshed at each merge; unknown
incoming values discard the old binding. Exact equality/inequality tests can
use those literals, but arithmetic and iteration counts are not evaluated.
This does not assume mutable fields stay unchanged or solve arbitrary
conditional lock protocols. Up to eight stable comparisons of parameters and
constants can also be retained across
branches, allowing a repeated compound guard to keep the same meaning. Loads
and computed loop values are not treated as stable parameters. A checked error
returned unchanged also counts as successful when its exact nil branch dominates
that return, preserving held-for-caller contracts without guessing from names.

The Boolean type check and identity construction use one branch for parameters
and computed SSA values outside cycles; comparisons retain their own operand
identity. `condition_identity_test.go` keeps named Boolean parameters,
loads/calls/phis, cyclic computations and comparison boundaries separate.
This consolidation changes no branch policy.

Return evidence distinguishes possible retention from retention on every
visited path to the same SSA return. A held-for-caller contract requires definite
retention, so merged held and released paths cannot establish it. A helper that
retains the exact lock on every normal return hands the critical section to its
caller even when its error result is opaque. This covers
[go-diameter's read-lock handoff](https://github.com/fiorix/go-diameter/blob/c7794c55a5412a3d91b17165971be4c6bc6b3ced/examples/s6a_proxy/service/util.go#L51-L87).
It does not prove the caller eventually releases the lock. Fixtures in
`lockorder/return_handoff.go` pair the handoff with mixed-state returns that must
remain reported. Boolean held-result contracts likewise decline mixed states.

Functions without a recognized direct or summarized mutex acquisition do not
need lock-state exploration. Each remaining function is limited to 4,096
distinct states and a shared 200,000-step traversal allowance. Queued visits,
state keys and copies, phi selection, guard/cycle queries, successor selection
and result-backed termination consume that same allowance. Exhaustion makes
the whole function inconclusive: its buffered
diagnostics and order edges are discarded, rather than treating a partial walk
as a complete release contract. Complex lock protocols can therefore be missed.

`flow_walk.go` owns expansion and the publication barrier; `state_copy.go`
detaches mutable predecessor collections before transfer. `flow_budget_test.go`,
`state_budget_test.go` and `branch_budget_test.go` cover late cutoff after buffered
findings/order edges, child versus parent exhaustion, fresh retries, phi/cycle
availability and nested result inference. `setup.go` builds one bounded
instruction/call/defer inventory, memoizes direct effects for state transfer,
and derives acquisition eligibility, caller-owned first actions and possible
deferred writer witnesses from it. Helper summaries retain a shared 2,000-step
setup cap while charging the same function pool; binding/census/dominance work
also charges that allowance. An interrupted setup exposes no partial metadata,
including completed earlier summaries. `setup_test.go` covers direct/forwarded
acquisitions, borrowed locks, builtins, opaque wrapper writers, cold/fresh/warm
cutoffs and independent summary-cap exhaustion.

`return_contracts.go` consumes the setup's return/branch inventory for
held-on-success and conditional caller-release contracts. Return masks,
acquisition dominance, Boolean polarity, caller cycle/coverage and private-lock
use censuses share the function allowance. Final report metadata reuses cached
direct effects; interrupted final queries discard earlier buffered findings
and order edges. `return_contracts_test.go` covers both Boolean polarities,
held-success, exact guarded errors, cold/fresh queries, padded caller coverage
and final-stage publication cutoffs.

`return_retention.go` records held/deferred returns, detached first masks and
possible/definite merges under that same allowance. Returned unlock capabilities,
containing-owner access paths and opaque callback handoffs use the bounded
lifecycle callback-capability proof. Its shared reaching fold keeps the existing
any-origin policy and one origin history per completion request; revisits
invalidate shortened memo answers. A capability establishes ownership uncertainty,
never invocation. `return_retention_test.go` covers owner/callback returns,
path merges, late cutoff and fresh recovery. The three flow cutoff families
share one assertion helper.

Heap queries and order publication retain independent costs. Lock release
completion shares the traversal pool through
`release_queries.go`: synchronous and spawned exact release, registered and
pre-acquisition possible defer release, and synchronous may-release use one
completion request owner. Each question retains its 250,000-step cap. A cutoff
at either level makes the function unavailable before publication; it never
stands in for an unlock. Only synchronous call instructions enter the
synchronous release path. Candidate/identity/defer censuses and pre-acquisition
dominance also charge the pool. `release_queries_test.go` covers launch/coverage
semantics, opaque callbacks, cold and fresh evidence, independent question
cutoff and late cutoff after buffered findings/order edges.

This is a traversal/completion boundary, not a whole-query time bound. Beads
`gohawk-dho.44.11.5.27` retains callee-order and identity review; named-result
storage, alias, type and graph internals keep their separately recorded costs.

For a private non-escaping helper, an exact Boolean result can also describe
which return still owns a package-global `sync.Mutex`. This is accepted only
when all bounded, directly visible callers test that same result and release
the exact mutex on its held-result branch. A leaking caller, an escaped function
value, a changed result, or a different mutex leaves this contract unknown.
Same-site loop reacquisition is also left unknown when an exact possible release
uses the same loaded Boolean guard and polarity. This does not establish that a
mutable field is unchanged: changed-field bugs remain a documented coverage
gap. Distinct guards and repeated acquisitions inside one guarded region still
need their own release evidence.

A conditional acquisition guarded by a direct Boolean load remains unknown
for missing-release. The same boundary applies to an exact three-instruction
getter: select a direct parameter field, load it, and return it. Separate calls
do not prove the field unchanged, so this declines a suspected leak rather
than establishing an unlock. Constant-returning getters, computed predicates,
and unconditional acquisitions retain their existing behavior.
`lockorder/loaded_getter_guards.go` pins these distinctions. The case is
[SCTP's optional write lock](https://github.com/pion/sctp/blob/a09fb03516289d7cd89bc589ac49ee84ac331c62/stream.go#L324-L350).

A cycle is suppressed when every edge has the same exact package-global
exclusive mutex held as a guard. This is intentionally narrower than matching
field names, receiver types, or read locks. Local object and parameter-relative
guards across different functions are not guessed to be the same instance.

An acquisition on an object no other goroutine can reach yet orders
nothing. When the points-to graph shows the locked object has not escaped
at the acquisition and is stored into a global or a field, sent, or handed
to a goroutine only afterwards on that path, the edge is initialization,
not an order: locking a job under the registry lock before publishing the
job does not contradict the steady state of locking the job first. The
same holds for a parameter of an unexported function when every call in
the package hands in a fresh local that has not escaped at the call. An
object that is never published, an object published first, an exported
function, and a caller that passes a shared object all keep the edge.

Cycle witnesses contain at most eight edges. Each search examines at most
512 edges, and a package retains at most 4,096 distinct order edges. Helper
evidence retains a bounded representative route rather than all call paths.
Exceeding a bound means coverage is lost, not that a cycle is proved.

Read/read edges are not automatically discarded. Go's `sync.RWMutex` blocks
new readers when a writer is waiting, so opposing read acquisition orders can
participate in a deadlock with queued writers. Modes explain the evidence;
they do not imply that a read-only pair necessarily deadlocks by itself.

### Package caller inventories

Package caller discovery uses one complete caller set for conditional release
and exclusive ownership. It records private non-method
function operands, including initialization, and rejects escaped values or more
than 32 synchronous callers. Its 20,000-instruction cutoff discards the entire
set for both consumers. Exclusive ownership checks every recorded caller's exact
argument, so a fresh direct caller cannot excuse a callback, Go/Defer use or
shared initialization caller. Methods remain unknown because this operand census
does not establish their complete interface/method-value callers. The consumers
retain separate cleanup and ownership policies; neither infers a field's guard
or participant confinement. `callers_test.go` and `exclusive_callers_test.go`
pin scope, escape, call-count, fresh caller and cutoff boundaries with compiled
SSA. `exclusive_callbacks.go` keeps the callback cycle diagnostic alongside the
accepted direct-only initialization pattern in `exclusive_owners.go`.

### Writes under a read lock

Possible imported writer guards use the completed setup call census when
checking for an explicit intervening writer unlock. The flow never rescans the
function body for that query. Dominance, temporal reachability, mutex identity,
read-versus-write unlocks and possible aliasing retain their existing policies;
the setup publication barrier still rejects incomplete inventories. The
`opaque_writer.go` fixtures retain both a held writer and an early explicit
release. This shares discovery only; temporal and heap-query internals retain
their independent cost boundaries.


One proven instruction/lock pair is reported once per function, even when
short-circuit predecessors reach it with different branch evidence. Proof and
tracing still run for each state; unknown states do not reserve a diagnostic,
and different lock identities at the same write remain distinct. The function's
existing report buffer still discards all findings if analysis is interrupted.
`readlockpaths` pins converging paths, two distinct writes, an uncertain writer
guard beside a reportable path, and two lock identities. This removes duplicate
output from [Skywalking's cursor initialization](https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L644-L649);
the underlying field-association reports are handled separately by the
receiver-field uncertainty boundary described below.

A read lock is shared: any number of readers may hold it at once. Writing to
the object it protects while holding only the read lock therefore races with
every other reader, and it needs nothing unusual from a second goroutine.

The report is about the object rather than about which field a lock guards.
Working out which of several mutexes covers a particular field is not something
this analyzer attempts, so a write is left alone when another lock is held for
writing, including one on another object, when the value being written was loaded out of the
object and is a separate cell, or when the field is updated atomically.
An unrelated exclusive lock can therefore hide a real race; this is an explicit
precision boundary, not a guarantee that the other lock protects the write.
The same uncertainty applies to an opaque imported wrapper call followed by a
dominating deferred standard exclusive unlock on that wrapper's mutex. This is
not a proved acquisition or a guard-to-field relation. Unrelated receivers,
known-empty wrappers, and explicit intervening releases do not qualify.

A result the reader builds for itself is not the object. A slice appended
from a nil start and then shuffled, reversed, or filled in under the read lock
writes only the reader's own memory. Those writes were reported because a
selection through the nil start widened the element address to `unknown`,
which aliased the owner; the points-to model now drops the nil base instead
(see the points-to model note). The walk from a written address to the owner
still does not follow a phi, so a slice that is nil on one path and the
owner's own storage on another is a recorded false negative. Seen in boxo:
https://github.com/ipfs/boxo/blob/3d6ac39a0f76c824ffbb90e3a729e78f62bc2d12/routing/mock/centralized_server.go#L61-L77
Fixtures: `lockorder/local_results.go`.

### Retired: recursive-acquire

A `recursive-acquire` check reported a `Lock` of a mutex this function already
held. It was removed on 2026-09-27 after about three true positives against
twenty false positives across the batch 56 to 61 audits. `sync.Mutex` is not
owned by a goroutine, so a second `Lock` deadlocks only when nothing else can
ever release the mutex, which a per-function proof cannot establish. Its false
positives were per-iteration mutexes sharing one SSA value, correlated guard
flags, a mutex used as a gate released by another goroutine, and helpers that
unlock internally. Its true positives were paths that forget an unlock, which
`missing-release` also covers.

## Sibling mutexes of one owner

A helper that releases a caller's held lock must release that exact mutex.
When the held mutex is a field of the owner the helper receives, such as
`&c.monitors` passed as `c`, the completion search maps the target as the
mirrored field beneath the helper's parameter, so a helper that locks and
unlocks `c.rpc` does not release the caller's `c.monitors`. Before this, the
owner was treated as an aggregate storing the mutex, any unlock beneath it
counted, and the caller's lock order was lost. A helper that unlocks the held
mutex itself still hands it off. The same argument binding as
`resourcelifetime` now also decides a callee's branches on a constant flag
inside the completion search, which is what exposed the sibling unlock in the
libovsdb fixture:
https://github.com/ovn-kubernetes/libovsdb/blob/6acd868996b9393b932a1eeeec1ea4e6c722ebe8/client/client.go#L286-L299
Fixtures: `lockorder/sibling_mutexes.go` and `lockorder/constant_arguments.go`.

When a closure captures the receiver, the receiver is spilled to a cell and
every use of it is a separate load, so the locked mutex and the argument handed
to a releasing helper start from different SSA values. The completion search
maps the mutex through the argument when the storage identity proof shows the
two loads are one value, as it is for a cell written once. centrifuge-go locks
`s.mu` and calls `moveToUnsubscribedLocked`, which unlocks it, in a function
whose callback captures `s`:
https://github.com/centrifugal/centrifuge-go/blob/080126041ccc71654718bd0601b920ff8b22a8bf/subscription.go#L1156-L1183
The mapping is pinned by a unit test in `internal/lifecycle`, and
`lockorder/captured_receiver_release.go` covers the shape with a helper that
always unlocks and one that may not. A `Lock` with a deferred `Unlock` inside a
callback closure, reported against the enclosing function in the same
repository (`client.go`), is a separate open follow-up.

## The order graph result

The analyzer returns the package's order graph as its result, for
`gohawk dump locks`: every recorded edge with one witness, and each reported
cycle as the edges its diagnostic names. The result is a copy made after the
walk; nothing decides from it, and it lists only cycles the analyzer
reported, so it cannot drift from the diagnostics.

## Final release and write decisions

The completed bounded function walk supplies held-return witnesses to
`proveMissingRelease`. That proof owns loaded-guard uncertainty, private-mutex
exclusion, the local release witness and the two caller-transfer contracts.
The reporting loop consumes its state and reason without repeating those
rules. Private mutexes remain outside the diagnostic, while a lock with no
local release witness remains unknown ownership. Neither an unknown loaded
guard nor a caller contract is replaced by a naming convention.

`proveReadLockWrite` separately owns the owner/write relation and the current
writer-state exclusions. It retains the existing owner-query order for unknown
candidates and selects the same first reportable owner. An explicit exclusive
lock or a possible imported writer is unknown field protection, not a release
or a proof that the field is guarded. One proof result is traced per invocation;
multiple paths in the lock walk may still examine the same instruction. The
function walk still buffers diagnostics and discards them if its state budget
is exhausted; this refactor does not widen that proof boundary.

Both reporters use `lockDiagnosticProof`, and the shared trace adapter only maps
its state and reason to an outcome. Reportable return and mutation witnesses
now expose `unreleased-return` and `read-lock-write`; private mutex, unobserved
release policy and explicit writer uncertainty also have stable trace reasons.
The existing trace assertions for caller transfer, imported writers, loaded
guards, state-budget exhaustion and release-attempt evidence remain alongside
these boundaries. Parent/current fixture JSON is compared independently of
trace events; no new guard-to-field inference or FP removal is claimed.

### Fixed-argument binding allowance

`ProveFixedArgumentsWithin` supplies a complete argument/capture outcome census
before callee branch pruning. Root binding, nested binding and constant-bound
block discovery share the context search's allowance. Budget cutoff drops
acquisition witnesses at the root: an unconstrained summary could revive an
impossible arm. Complete empty binding censuses and non-budget depth limits
retain the existing ordinary declaration-summary fallback.
No partial binding map can select a callee arm or enter its context search.

`callee_constants_budget_test.go` covers both constant arms, forwarded flags,
child cutoff/fresh queries and a large nested binding census whose body would
otherwise be pruned. Existing constant-argument and helper-field fixtures keep
their policies. `callee_lock_bindings.go` now owns the distinct caller snapshot,
embedded field and constructor-slot identity mechanics formerly beside summary
traversal; this is an extraction with unchanged evidence and rationale comments.
Independent key, identity, type and ordinary fallback-summary costs remain
separate; this does not claim a bound on the entire lock proof.


## Private read-lock mutation destinations

The read-lock-write proof selects one mutation destination for stores, map
updates and the supported `delete`, `clear` and `copy` builtins. Owner matching
and exclusivity consume that same selection. A local object that has not been
exposed at the mutation cannot race with another reader; this is an exact
exclusion, traced as accepted `private-write-storage`. An unpublished wrapper
alone is insufficient: a borrowed map or slice retains its caller's storage
and the existing report boundary.

Heap object exclusivity now distinguishes fresh language `make` allocations
from other opaque origins. An unknown element index still belongs to one
backing object; stale, unknown or mixed object identity remains unavailable,
and exact content identity still requires one exact slot. Any recorded exposure
of a fresh collection or its selections defeats local exclusivity. Standard
synchronous mutex methods invalidate only their exact mutex receiver storage
without retaining its enclosing user object. Async launches, interface calls,
`RLocker` and project-defined lookalikes retain conservative effects.

The `privateread` fixture pins local scalar and container writes, initialization
before publication, publication before mutation, uncertain caller owners and
borrowed map/slice storage. Shared heap controls cover mutex API identity,
async/adapter/lookalike calls, published owners, fresh/borrowed/opaque/mixed
collections, selected-element exposure and loop allocation. This does not
infer which field a mutex guards or who can participate in its read protocol;
the two recorded Skywalking cursor sites remain unresolved.


## Receiver fields with caller-supplied synchronization

A bounded package census identifies exact receiver fields written in methods
whose complete SSA body contains no call, defer or goroutine launch. Such a
method carries no acquisition in its own frame. Its callers may supply a guard
or exclusively own the field, but the read-lock proof does not establish that
contract. A store to that declared field therefore returns `field-guard-unknown`
at the existing decision point. This is positive uncertainty, not a claim that
the setter is safe or independent of the read lock.

The census resolves only embedded field-address paths rooted in the exact
receiver parameter. Loaded pointers and other parameters do not qualify.
Declaration identity keeps sibling fields and same-named fields of different
types separate. Any call disqualifies the whole method, including one after a
store; a write-locked setter supplies no witness. The index is constructed once
per package, rather than rediscovering methods for each mutation. Cutoff
invalidates the index and suppresses read-lock reports as budget-unknown;
partial negative evidence must not authorize a report. Order and release checks
retain their existing evidence.

The `readlockfields` fixtures reproduce pointer cursor replacement and a scalar
cursor update, retaining unrelated-field and guarded-setter diagnostics.
Actual SSA controls pin loaded-pointer exclusion, late-call exclusion,
declaration identity and cutoff/fresh-allowance recovery. This deliberately
loses a real read-lock write when a field-only setter is always called under the
same guard; that caller contract remains a known gap. It does not infer a
field-to-lock relationship for fields without a witness. The representative
cursor reset is [Skywalking Buffer.ResetForLoopReading and PrepareForReading](https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L629-L649).


## Initial map publication under an owner writer

The first direct Lock of a same-block allocation can have an unknown ordering
role when the allocation was just stored as a map value and exactly one held
writer belongs to that map's owner. The acquisition decision returns
`initial-publication-order-unknown` and omits only that writer's ordering edge.
The new mutex stays held, and all unrelated held locks retain their edges.
This is initialization uncertainty, not a proof of exclusive ownership or of
the map's reader contract. `ExclusiveAt` continues rejecting exposed objects.

The interval permits only field addresses, loads, debug references and one
exact map-value publication. Calls, stores, sends, prior acquisitions, branch
boundaries and another publication retain ordinary ordering evidence. Loaded
gates, reader guards, a different owner, multiple matching writers and a writer
whose release is uncertain do not qualify. Exact owner identity uses the shared
bounded structural query; cutoff cannot supply a matching identity.

The `initialpublication` fixtures retain wrong-owner and intervening-exposure
cycles. Compiled SSA controls cover read guards, prior acquisitions, late guard
acquisition, loaded gates, multiple writers, unrelated held locks, uncertain
release and interrupted allowances. Tracing projects the authoritative proof.
The former `lockorder/gate_mutex.go` diagnostic fixture is deleted: readers that
bypass the writer can still cause a real cycle, which this boundary deliberately
misses. Its loss of recall is recorded in the new fixture header.

The motivating publication is [Rune's package iterator gate](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/ide/idepkg/manager.go#L416-L422).

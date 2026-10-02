# goroutineownership design notes

The public page is [goroutineownership](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Reports goroutines whose proven completion obligation is not honored on every return path.

Constructor discovery draws a `SummaryBudget` allowance from the spawn's
observed candidate pool. Its instruction census, exact/stable binding queries,
nested notification and group coverage, terminal completion tails and callback
wrapper invocation proof share that allowance. Return coverage uses the shared
tri-state obligation walk; the adapter charges examined instructions as well
as expanded path states. Discovery exhaustion yields
`completion-discovery-budget-exhausted` and unknown for the whole candidate,
even if a partial census found a signal before missing an alternative handle.
The probe is initialized before discovery, and its cutoff evidence records
whether the local allowance or candidate pool ran out. Constructor follow-up
work stops at that cutoff. No incomplete discovery is cached.
`discovery_budget_test.go` checks an oversized actual SSA body, attributed trace
cutoff, partial evidence at a candidate-pool cutoff and a fresh full-budget
missing-join proof. This can miss defects in oversized workers. Owner/lifecycle
suppression adapters after constructor discovery remain separate review scope.

Retained-owner calls and selected context observations now share one allowance
per query across worker resolution, send/output census, capture and cleanup
target enumeration. Factory returned-cleanup queries and legacy sibling
callback checks use that same allowance. The shared unreadable-callback query
has a budgeted capture/body census; its default facade preserves existing
possible-consumption policy. A retained-owner cutoff produces unknown at the
affected instruction or selected edge, with `retained-owner-budget-exhausted`
and attributed evidence. It does not excuse a return that bypasses that point.
`retained_budget_test.go` exercises the classifier, path-local flow, fresh
unrelated owners, factory availability and selected-context cutoffs;
`capture_evidence_test.go` checks fresh and exhausted shared callback queries.
Constructor discovery remains independently guarded. These changes bound the
enumerated scans, not every transitive identity/alias helper or method-set
construction; those mechanics and the remaining caller-bound routes still need
review. No new worker-completion guarantee or published fact is introduced.

Caller-bound and relay-dependency queries use supplied observed allowances for
worker resolution, direct receive/select and binding census, receiver-field
write checks, cancellation coverage and opaque callbacks. Local cancellation
uses the shared obligation flow with bounded states/instructions; a preceding
defer still requires dominance. The lifecycle proof checks availability after
each query before naming a bound, and stops immediately on exhaustion. Relay
cutoff is `relay-dependency-budget-exhausted`; caller cutoff retains
`worker-receive-budget-exhausted`, with attributed phase evidence.
`caller_budget_test.go` checks fresh/cutoff channel, receiver, cancellation and
relay queries, plus exact, deferred, conditional and asynchronous cancellation.
`call_binding_budget_test.go` pins budgeted possible capture mapping and the
existing reachability direction. Relay participant filtering preserves the
prior participant-to-launch order; this change does not broaden it to later
participants. Caller lifetime policies live in `caller_bounds.go`; the shared
receive engine retains value-specific memoization and possible-consumption
semantics. Underlying identity, call-result scans, initial flow guards and
metadata allocation remain transitive review scope, not a claimed complete
wall-clock bound.

Budgeted full-body census is shared through `ssaflow.InstructionsWithin`.
Worker send/output guards, caller receive/field-write/cancellation scans,
retained-owner opaque work, relay participants and the value-specific receive
engine use its block-order iterator. Shared callback evidence uses the same
census. It charges before yielding and stops when the consumer finds a witness;
policy, select-state/argument spending and cutoff availability stay at callers.
Receive memoization still rejects incomplete negative answers, caller receive
stops before helper search on exhaustion, and opaque callbacks remain possible
consumption. The shared iterator's partial/fresh/early-stop test supplements
the existing candidate-attributed cutoff and path-local flow controls.
Launching background work without a recognizable completion obligation is not
itself a diagnostic, including with `-enable-all`.

For an already-proven worker completion obligation, complete concurrency
summaries can establish that a synchronous or deferred helper receives from
the exact completion channel or waits on the exact WaitGroup. This includes
imported helpers and local wrappers that call them. Existing branch-aware
helper and ownership proofs remain in use when an ordered summary is
unavailable; missing summaries are never treated as evidence of no join.
An asynchronously launched waiter does not join the worker in its parent.
`gohawk-dho.44.7` applies that boundary before receiver, library and helper
completion contracts. A launched WaitGroup observer is unknown handoff in both
the caller classifier and helper-body search; it cannot borrow synchronous
receiver semantics. Synchronous and deferred waits retain exact coverage.
`asyncobservers/waits.go` pairs direct/helper launches with those exact forms
and an unrelated observer whose worker still requires a join. Trace controls
require one label per observer with the corresponding proof strength.
`gohawk-dho.44.8` keeps recursive helper cutoffs unknown too. A helper that
forwards the tracked completion value into a recursive call may wait before
returning, even when the bounded search cannot establish that coverage. The
cutoff therefore cannot prove absence of a handoff. This intentionally misses
some omissions behind recursion; it does not infer a recursion count or promise
termination. Independent exact observations still win when they cover every
normal return. `recursivehelpers/waits.go` pairs direct/mutual forwarding with
an explicit final Wait and an unrelated-group diagnostic. A retry test enters
the same helper on an active path, gets unknown, then proves its receive on a
fresh path, checking that the cutoff does not poison the memo.
Return ownership is classified through the same cached instruction path as
receives, waits, stores and calls. The ordinary flow and guarded non-nil retry
reuse that label, including when different branch states reach one return.
`returnlabels/returns.go` pairs a merged return of the worker's completion
channel with an unrelated returned channel; the trace regression requires one
transfer label associated with the accepted candidate and preserves the
unrelated channel's rejected decision.
`gohawk-dho.44.2` separates transfer identity from possible containment: returns
and stores outside the function credit an exact transfer only when the shared
storage identity proof matches a tracked value. Mixed phis, overwritten
aggregates, captures and wrapper results remain unknown handoffs. An exact
handle returned beside an opaque result still covers the obligation.
`transferlabels/transfers.go` pins both proof outcomes; the reporter continues
to accept unknown ownership. GoMock result registration likewise retains
possible containment as unknown rather than establishing exact stream identity.
`gohawk-dho.44.3.1` additionally requires exact call-site identity before a
helper's join can cover the tracked value. A mixed argument or an aggregate
that previously contained the channel keeps the helper use unknown. Direct
WaitGroup acceptance likewise requires the exact receiver; possible group
receivers are labelled `possible-join-receiver` and stay unknown.
`joinbindings/joins.go` pairs those uncertain forms with exact helper and
WaitGroup joins and an unrelated-channel diagnostic.
`gohawk-dho.44.4` also requires exact storage identity for direct receives,
selected receive edges and helper-internal completion observations. A mixed
phi, sibling aggregate selection or possible nested binding supplies unknown
ownership, never an exact join. Counted drains retain their sender/count proof
and distinguish exact completion handles from possible ones. Guarded joins
keep the broader matcher because their result is only unknown. Owner helper
coverage still identifies possible lifecycle participation, which its worker
consumer projects as unknown. `receiveidentity/receives.go` pairs mixed direct,
selected and helper/group observations with exact receives, nested forwarding
and unrelated/default-arm diagnostics; the trace pins unknown selected edges
and one cached label per direct or helper observation.
`gohawk-dho.44.5` keeps lifecycle methods on tracked owners as unknown shutdown
participation even when the receiver is exact. Calling or deferring a project's
Close, Stop, Shutdown, Wait or Kill method does not establish a worker join.
The helper-effect projection preserves that distinction too: positive owner
method coverage remains useful to retained-resource cleanup queries, while the
worker flow receives unknown. `ownerparticipation/owners.go` pairs no-op methods
with a real completion-channel join and an unrelated-owner diagnostic. Direct
participation has its own `owner-lifecycle-participation` trace label.
Caller-owned channel and stable receiver-context bounds share one receive
search, keyed by both function and local value. Repeated calls to the same
helper therefore retain distinct formal bindings. These are possible lifetime
bounds, never joins; the search preserves the existing opaque captured-cell
boundary and receiver-field mutation checks. Its queries share the candidate's
budget. Exhaustion suppresses reporting as `worker-receive-budget-exhausted`,
while recursive and unavailable bodies remain unknown within the search.
`gohawk-dho.44.6` projects the caller stop/context bounds as unknown too. The
search finds a receive on any path; it does not show completion before the
spawning function returns. Completion-handle ownership is resolved separately
before those bounds, retaining the existing factory/registry opacity rules and
the caller-owned channel/group transfer contract. `callerbounds/bounds.go`
pairs both outcomes with real join and missing-join controls. The channel type
guard still does not select a captured pointer-to-channel cell for the helper
search; that form retains its uncovered-send diagnostic. Matcher coverage is
unchanged by this outcome correction.
Fixtures in `goroutineownership/receive_bindings.go` cover a receiver bound
through the second helper call and an unrelated local receiver that supplies
no caller-owned bound. Unit tests additionally cover diamonds, nested launches,
capture opacity, recursion, missing bodies, field replacement and fresh-budget
retries after an incomplete search.
Local callback wrappers use the shared exact invocation proof, including
an invoker passed as another bound callback. The wrapper must synchronously
invoke the exact worker before every normal return; forwarding to another
goroutine or dropping it supplies no worker promise. Fixtures in
`goroutineownership/callback_bindings.go` pair the joined and unjoined forms
with those opaque controls. The body-only promise never credits the outer
launch as a join.
A `select` receive joins only the path that selects that case. Timeout,
default, send, and unrelated receive arms do not inherit its completion;
each must independently honor the obligation or terminate the goroutine.
A direct channel close establishes a completion obligation only when that
same channel is closed or sent on before every normal worker return. An error-only close
does not promise a completion notification on successful exits.

Nested literal notifications use the same operation selection as direct
notification discovery and return coverage. Detached `go close` operations,
loop-local closes and sends followed by work cannot supply this notification.
A nested closure must notify the exact channel before every normal return;
its invocation or deferred registration must also cover every outer return.
A synchronous invocation must be terminal in the worker. Conditional nested
notifications and registrations therefore create no unconditional join promise.
This deliberately misses genuine omissions in those conditional forms rather
than inferring caller/worker guard relationships. `notificationpromises` pairs
those accepted forms and inner/outer progress work with unconditional deferred
and synchronous missing-join diagnostics and exact receive controls. The same
return-coverage query requires a return witness rather than a vacuous proof
from a body with no returns. No application-specific contract is added.
A close-or-send worker whose send
is followed by arbitrary deferred cleanup is also outside coverage; the
previous diagnostic fixture was removed as an accepted false negative. A
close-or-send worker with no later work retains its missing-join diagnostic.

Completion binding at a launch requires exact worker-local identity, rather
than selecting the first parameter or capture that may alias a notification.
Captured scalar/pointer cells must be read-only in the worker and have stable
contents at the spawn; a raw captured struct address already identifies its
object. The existing storage and callback-effect queries supply this evidence.
Reassignment inside the worker yields no unconditional caller obligation;
reassignment before the spawn can map the latest stable value. Nested closure
notifications preserve an exact read-only cell binding until the outer launch
maps its stable contents. Aggregate roots retain possible-owner acceptance,
with exact root mapping and unknown element observation rather than exact join.
`completionbindings/bindings.go` pairs mixed parameter and replaced capture
acceptance with stable snapshots, exact argument/capture/nested signal
missing-join diagnostics, exact receives and group controls. Uncertain bindings
can hide genuine missing joins; choosing one possible caller value would create
false obligations. No body is specialized for a caller's branch condition.


These summaries do not introduce new worker obligations: a close or `Done`
inside a helper may be an early readiness signal rather than completion.
A direct `Done` followed by more work is also insufficient to establish a
completion obligation: it may intentionally signal startup. Mistaken early
joins remain a coverage gap; independent deferred or terminal completion
obligations are still checked.
A terminal `Done` followed only by deferred `Done` or channel-close operations
can provide an alternative completion handle: there is no remaining blocking
worker work. Arbitrary deferred calls do not satisfy that boundary.

When a worker's completion field resolves only to its aggregate owner, receiving
a channel supplied to that aggregate is uncertain rather than a proven join.
The analyzer cannot establish which field is involved, so it declines the
diagnostic. Returning a projection of the captured aggregate is also uncertain,
not proof of a join. Factory- or registry-supplied completion channels remain
outside the local ownership proof until their allocation and ownership are
established. This deliberately misses some unjoined constructor-created workers.
Copying a factory's returned owner struct into a local does not change that
uncertainty. Passing the aggregate from which a completion channel was loaded
to a helper with lifecycle activity is likewise uncertain, not an exact join;
the helper may act on a different field. Merely inspecting that aggregate does
not suppress the diagnostic.

The same existing opaque-group boundary covers an embedded WaitGroup selected
through fields of an unreadable factory or getter result. Storage loads are
resolved before field provenance: a fresh pointer installed into a returned
owner's group field remains a local completion obligation, as do local owners
and visible fresh constructors. Nested fields and mixed local/registry owner
alternatives are possible external ownership and remain unknown. This adds no
callback registration, private-data identity, or completion guarantee.
`registry_group_fields.go` covers these boundaries alongside unrelated factory
calls and captured-owner forms. It deliberately misses a fresh owner returned
by an unreadable factory; unresolved pointer loads, dynamic indexes, and other
unsupported projection forms are not expanded by this field-address rule.
The FDio callbacks obtain private data from another package and settle its
embedded group, while a separately registered disconnect callback waits:
https://github.com/FDio/govpp/blob/c71484d8c74da940abbd70407b53894fa4c56f01/extras/gomemif/examples/bridge/bridge.go#L33-L100
The correction abstains because that owner may already be registered; it does
not prove the disconnected callback runs or waits for this worker.

Stores through a type-asserted owner remain visible when that same owner is
later handed to a helper. The assertion is not an ownership guarantee, and
constructing or filling an owner without handing it off still does not settle
the worker.
Closing collection entries inside a loop is per-item cleanup, not evidence of
worker completion.


Relay discovery now uses the same exact/stable worker-to-caller bindings as
obligation discovery. A group reassigned before launch maps its latest stable
value, rather than its first initializer. Relay closes use the classifier's
exact channel matcher, with the constructor's allowance, so possible aggregate
ownership cannot establish a relay. `relaybindings/groups.go` pairs the stable
snapshot and exact independent Wait with unrelated Wait and extra local work
diagnostics; caller-owned extra work remains unknown through its lifetime bound.
Relay resolution/census, owner capture/method selection and pipe worker/binding
search now share the constructor allowance. Each stage records an attributed
cutoff and stops discovery; partial alternatives or owner/peer witnesses never
justify a default diagnostic. Adapter cutoff tests exercise relay, owner and
pipe phases. Budgeted possible-capture extraction keeps its historical first-
initializer candidate semantics, as the shared helper's cutoff/fresh controls
confirm; it is not used to establish exact relay binding.

A straight-line relay that only waits on one exact WaitGroup and closes its
completion channel can use that group as an alternative completion handle.
Additional work, sends, defers, or control flow do not qualify. An existing
cancellation proof for a worker settling that exact
group makes relay shutdown uncertain. Sending that exact group in a queue item
likewise establishes possible external participation, not guaranteed completion.
This is one-hop evidence, not WaitGroup-count or scheduling analysis.

A lifecycle call on a resource retained by a captured reader can also make
shutdown uncertain. For example, closing the connection used to construct a
buffered reader may release its worker's read. This requires positive helper
retention evidence and cleanup covering every return; an unrelated connection,
an ignored constructor argument, or a close before launch does not qualify.
Nested captures preserve the identity of an interface cell and its loaded
connection. These are not join proofs.

An opaque method can instead consume the exact field of a captured owner that
the parent later cleans up. The retained-owner classifier maps that field with
`ssaflow.ProveIdentity` through `CallBindings`, requiring stable captured cells.
It credits only unknown ownership on the cleanup path, never a join. The opaque
call must be followed only by a nonblocking completion tail; a send, receive,
second call, loop or arbitrary deferred work declines this additional mapping.
`opaque_worker_fields.go` pins same-field cleanup, different fields and owners,
blocking publication and receive tails, a return bypassing cleanup, a visible
no-op method, and a reassigned capture.
[Lynx's gRPC shutdown](https://github.com/lynxbase/lynxdb/blob/7c4bf0432b0cef2807f0ddcd2cd2000ce7ffb8c1/pkg/ingest/receiver/otlpgrpc/server.go#L106-L121)
is the representative field shape. The rule does not infer shutdown semantics
from `GracefulStop` or `Stop`, and it does not track worker scheduling or hidden
field mutation. Some genuine missing joins after opaque operations are therefore
missed. The identity and tail predicates remain beside retained-owner evidence
despite taking that cohesive file just above the 400-line review trigger; they
extend the same classifier boundary rather than creating another proof path.

An invoked or deferred cleanup callback returned beside a resource can supply
the same evidence, but only when a visible returned literal captures that
exact resource and performs its lifecycle operation. Unrelated sibling
results and an unused cleanup callback do not qualify.
Retention may happen outside a helper's result, and the worker may block on
something else afterward, so this boundary deliberately loses some coverage.
The exact opposite endpoint returned by `io.Pipe` or `net.Pipe` similarly
provides uncertain lifecycle evidence when handed to a helper after launch.
The existing helper classifier still rejects a source-visible helper that
ignores the peer; creating a pipe, storing its peer locally, or calling a
helper before launch is insufficient. Neither boundary applies to a worker
with a visible channel send or send-select: releasing I/O cannot settle a
subsequent publication to an abandoned receiver. Sends hidden inside helpers
remain outside this bounded exclusion.

A worker receiving from a locally created `WithCancel`
context is also uncertain when the exact sibling cancel function is already
deferred before launch or called on every later return path. Captured context storage must remain stable. This is a
cancellation boundary, not a join.
The same uncertainty applies when a literal worker passes that exact canceled
context to an imported or dynamic helper whose body is unavailable. Such a
helper may ignore cancellation; this is an intentional coverage loss, not a
claim that cancellation always stops arbitrary I/O.
Likewise, a selected `context.Context.Done` receive is uncertain when a
worker-captured argument positively retains that same context and is handed
to an opaque callee. Only the canceled arm gets this evidence; ignored or
unrelated contexts do not qualify. Visible worker sends still require their
own completion handling. An opaque helper also receiving a send-capable
channel cannot use this exemption: cancellation does not prove that its
result publication stops, even when the channel is buffered. Receive-only
arguments do not establish this output hazard.

A worker that `main.main` of package `main` launches at most once, outside any
loop or closure, is not reported as unjoined: every way out of `main` ends the
process and stops it. This settles only the join obligation, and says nothing
about whether the worker finished its work first.

A counted loop that runs a blocking, receive-only `select` exactly N times
joins the workers whose channels it drains when there are at most N such
channels and each has at most one send per call. Receives cannot outnumber
sends, so leaving the loop proves that every worker sent. A `default` arm, a
`break`, a timeout arm, a dynamic bound, or a second sender voids the count.

A channel the worker only closes, and that nothing in the function, its
closures, or the static callees it is passed to ever receives from, selects
on, sends on, or hands away, is not a completion obligation: no code waits
for it, and close never blocks. Such a done channel is usually left over from
a removed wait or copied from a sibling that does wait. The census is
`ssaflow.ChannelValues`; any use it cannot follow keeps the obligation, and a
send nobody receives is still reported, because it blocks the worker forever.
Seen in agentsh's drain loops and dalec's progress display (batch 61):
https://github.com/canyonroad/agentsh/blob/0ce9939b6ccead8b21b9ce16783b287d18012777/internal/db/proxy/postgres/upstreamread_test.go#L229-L237
Fixtures: `goroutineownership/unobserved_signals.go`. Fixtures whose subject
is another boundary wait on their done channel on one path
(`if waitForWorker { <-done }`), so the obligation is real and the path that
skips the wait is the one they judge.

## Former public summary

Reports goroutines whose completion is promised but not awaited on every
return path. A worker makes the promise when it signals that it finished: it
closes or sends on a channel, or calls `Done` on a `sync.WaitGroup`. The
launching function must then receive, wait, or hand the channel or group to
code that does. Launching background work with no such promise is not a
diagnostic.

Joins are recognized through helpers in this or other packages, through
`select` arms, and through a counted loop that receives one message from each
worker. When the completion signal reaches code the analyzer cannot see
through, nothing is reported. A worker launched once by `main.main` of package
`main` is not reported, because program exit stops it.

### Reaching-value visit allowance

Possible spawned load mapping and retained-owner/pipe discovery attach their
existing allowance to `ssaflow.NewReachingWalk(forms).Within(budget)`. The fold
charges transparent wrappers, phi alternatives and revisits before inspecting
them. Independent must-branches retain separate visited sets and share the
allowance. Leaf queries that exhaust it cannot return positive fold evidence;
callers retain the existing authoritative unknown outcome at cutoff. Leaf-only
charges are removed from these routes to avoid charging one visit twice.

`internal/ssaflow/reaching_budget_test.go` uses actual SSA wrappers and phis to
check branch sharing, candidate-pool cutoffs, early possible witnesses,
unchanged opaque forms and nested-leaf exhaustion. This bounds value visits;
it does not bound visited-map cloning, all heap queries, metadata scans or
flow setup. Those remain the transitive-cost review in `gohawk-dho.44.11.5`.

### Call metadata allowance

Retained-owner and caller-bound queries enumerate arguments/captures through
`ssaflow.CallBindingsWithin` or `ClosureBindingPairsWithin`. These iterators
charge before yielding and allocate no binding slice. Default collectors use
the same enumeration policy. Arguments precede captures; possible spawned-value
selection still makes its capture-first pass before its argument pass.
The shared lifecycle callback query uses the same capture iterator.

Cleanup-target, local cancellation and pipe-peer queries select tuple results
through `CallResultWithin`, charging referrer inspection and stopping at the
exact selected result. Cleanup and cancellation stop before downstream queries
when selection exhausts the allowance. Cutoffs retain the existing authoritative
unknown decisions; a missing result at cutoff is not evidence of absence.
`internal/ssaflow/call_metadata_budget_test.go` pins actual SSA argument/capture
order, shared-pool partial census, early stop, tuple lookup and single-result
representation. Heap identity, access paths, dominance and flow setup remain
open in `gohawk-dho.44.11.5`; this is not a complete wall-clock bound.

### Structural identity allowance

Caller-owned capture projections and retained-owner field mapping share their
existing allowance with `ValueIsAccessPathFromWithin` and `ProveIdentityWithin`.
The authoritative structural identity and access-path engines now charge
comparisons, wrapper/phi visits, field/index recursion and path-step comparison.
Default facades delegate with a nil allowance and keep existing projection
policy. Shared path-step equality removes the separate Boolean comparison
previously maintained by `SameAccessPath` and `ProveIdentity`.

`ProveIdentityWithin` yields unknown with `EvidenceBudgetExhausted` when the
query is interrupted; it cannot report different paths or credit an owner from
partial evidence. Actual SSA controls in `identity_budget_test.go` cover
agreeing converted phi alternatives, distinct loads, static field/index paths,
differing and dynamic indexes, candidate-pool cutoff and exhaustion at the last
path-comparison step. Heap graph queries, storage's default structural calls,
other consumers and initial flow setup remain the parent transitive review;
this does not replace possible alias evidence with exact structural identity.

### Initial order lookup allowance

The shared obligation walk charges its initial instruction lookup to the flow
allowance. If that lookup is interrupted, it returns uncertain without a return
witness or classifier invocation. This is a setup cutoff, not vacuous honored
coverage. Caller cancellation's preceding-defer check also shares its allowance
with exact dominance. `flow_setup_budget_test.go` pins actual block positions,
same/cross-block dominance, shared-pool cutoff, initial uncertainty and fresh
honored/violated outcomes. Initial guard extraction shares the allowance as described below; downstream
feasibility internals remain open. Guard-state charging is described below.
This is not a whole-flow cost bound.

### Initial guard setup allowance

The shared obligation walk now seeds paths with `GuardsDominatingWithin`,
sharing its allowance through dominator visits, negation/address decoding,
computed-condition cycle checks, whole-body store census and invalidation order.
An interrupted setup returns no guard seed and uncertain coverage before any
classifier invocation. It cannot treat missing correlation as an uncovered
return or use a partial seed to prune paths. Default decoding and guard-limit
policy are preserved through the same engines; loaded guards remain unstable
and stores before the target still invalidate them.

`guard_setup_budget_test.go` constructs actual SSA for stable, loaded and
mutated guards; pool and flow-setup cutoffs; computed loop conditions; nested
field addresses; and negation parity. Fresh queries preserve default identity
encodings and uncovered-return witnesses. Initial setup lives in
`flow_guard_setup.go`, apart from guard identity/state transitions. Downstream
edge feasibility internals, graph costs and other consumers
remain the parent transitive review; this is not a whole-flow wall-clock bound.


### Flow state and edge guard allowance

The shared obligation engine uses `WalkStatesWithin`, charging every queued
visit before key construction, including revisits. Instruction visits, guard
key rendering, store/rerun identity filtering, and edge condition decoding
share the flow allowance. Default state and guard facades use the same engines
with nil allowance. Store and rerun invalidation share one identity filter;
stable contradictions still prune paths and loaded contradictions remain opaque.
The caller-cancellation classifier no longer charges the instruction visit
again; its nested storage queries retain their own shared work charges.

A classifier, return, edge, successor or termination callback that exhausts the
shared allowance supplies no answer: the walk becomes uncertain and clears its
return witness before accepting settlement, violation or path pruning.
`flow_state_budget_test.go` pins queued/revisit and interrupted-key behavior,
actual SSA stable/loaded guards, mutation/key/edge cutoffs and callback cutoffs.
Existing default flow controls pin fresh honored and violated paths.

These charges cover the listed visits, not complete callback internals.
Successor feasibility, graph construction/waiting,
leaf string/type rendering and allocation costs remain independent review
scope. This is not a whole-query wall-clock bound.


### Deferred termination allowance

The obligation walk asks `InstructionTerminatesWithin` with its shared allowance.
Direct call dispatch spends before consulting the termination callback and
rejects an interrupted callback's positive answer. At `RunDefers`, the engine
lazily inspects instructions through `InstructionsWithin` and asks
`InstructionDominatesWithin` for a catalog terminating defer. Conditional
registration remains insufficient: only a registration dominating deferred
execution can end that path. Default termination facades use the same engine
with nil allowance, retaining the documented library contracts.

An interrupted census or dominance query provides no termination evidence;
the obligation walk sees exhaustion and returns uncertain rather than pruning
the path or reporting a return reached with incomplete evidence.
`flow_termination_budget_test.go` builds actual SSA for unconditional,
conditional and unrelated defers, checks all interrupted allowance sizes and a
candidate-pool cutoff, and verifies fresh versus interrupted call callbacks.
The flow control preserves honored coverage for a fresh unconditional deferred
exit and uncertain coverage when its nested census cuts off.

Library-contract inference and callback internals retain separate work costs,
as do successor feasibility and heap graph construction. This step charges the
listed termination visits without claiming a whole-query wall-clock bound.

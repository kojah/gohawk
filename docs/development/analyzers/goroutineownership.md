# goroutineownership design notes

The public page is [goroutineownership](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Reports goroutines whose proven completion obligation is not honored on every return path.
Launching background work without a recognizable completion obligation is not
itself a diagnostic, including with `-enable-all`.

For an already-proven worker completion obligation, complete concurrency
summaries can establish that a synchronous or deferred helper receives from
the exact completion channel or waits on the exact WaitGroup. This includes
imported helpers and local wrappers that call them. Existing branch-aware
helper and ownership proofs remain in use when an ordered summary is
unavailable; missing summaries are never treated as evidence of no join.
An asynchronously launched waiter does not join the worker in its parent.
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
WaitGroup and lifecycle acceptance likewise require the exact receiver;
possible receivers are labelled `possible-join-receiver` and stay unknown.
`joinbindings/joins.go` pairs those uncertain forms with exact helper and
WaitGroup joins and an unrelated-channel diagnostic. Internal helper-body
derivation and aggregate receive selection remain under review in
`gohawk-dho.44.4`; the call-site correction does not certify them.
Caller-owned channel and stable receiver-context bounds share one receive
search, keyed by both function and local value. Repeated calls to the same
helper therefore retain distinct formal bindings. These are possible lifetime
bounds, never joins; the search preserves the existing opaque captured-cell
boundary and receiver-field mutation checks. Its queries share the candidate's
budget. Exhaustion suppresses reporting as `worker-receive-budget-exhausted`,
while recursive and unavailable bodies remain unknown within the search.
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

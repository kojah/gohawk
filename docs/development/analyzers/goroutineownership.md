# goroutineownership design notes

The public page is [goroutineownership](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Reports goroutines whose proven completion obligation is not honored on every return path.
Launching background work without a recognizable completion obligation is not
itself a diagnostic, including in `join` mode and with `-enable-all`.

For an already-proven worker completion obligation, complete concurrency
summaries can establish that a synchronous or deferred helper receives from
the exact completion channel or waits on the exact WaitGroup. This includes
imported helpers and local wrappers that call them. Existing branch-aware
helper and ownership proofs remain in use when an ordered summary is
unavailable; missing summaries are never treated as evidence of no join.
An asynchronously launched waiter does not join the worker in its parent.
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
Stores through a type-asserted owner remain visible when that same owner is
later handed to a helper. The assertion is not an ownership guarantee, and
constructing or filling an owner without handing it off still does not settle
the worker.
Closing collection entries inside a loop is per-item cleanup, not evidence of
worker completion.

A straight-line relay that only waits on one exact WaitGroup and closes its
completion channel can use that group as an alternative completion handle.
Additional work, sends, defers, or control flow do not qualify. In default
context mode, an existing cancellation proof for a worker settling that exact
group makes relay shutdown uncertain. Sending that exact group in a queue item
likewise establishes possible external participation, not guaranteed completion.
This is one-hop evidence, not WaitGroup-count or scheduling analysis; strict
join mode does not use these relay rules.

A lifecycle call on a resource retained by a captured reader can also make
shutdown uncertain. For example, closing the connection used to construct a
buffered reader may release its worker's read. This requires positive helper
retention evidence and cleanup covering every return; an unrelated connection,
an ignored constructor argument, or a close before launch does not qualify.
Nested captures preserve the identity of an interface cell and its loaded
connection. These are not join proofs and do not apply in `join` mode.
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

In default context mode, a worker receiving from a locally created `WithCancel`
context is also uncertain when the exact sibling cancel function is already
deferred before launch or called on every later return path. Captured context storage must remain stable. This is a
cancellation boundary, not a join, and does not suppress join-mode diagnostics.
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
arguments do not establish this output hazard. Strict join mode does not
accept the context boundary.

A worker that `main.main` of package `main` launches at most once, outside any
loop or closure, is not reported as unjoined: every way out of `main` ends the
process and stops it. This settles only the join obligation, and says nothing
about whether the worker finished its work first.

A counted loop that runs a blocking, receive-only `select` exactly N times
joins the workers whose channels it drains when there are at most N such
channels and each has at most one send per call. Receives cannot outnumber
sends, so leaving the loop proves that every worker sent. A `default` arm, a
`break`, a timeout arm, a dynamic bound, or a second sender voids the count.

## Former public summary

Reports goroutines whose completion is promised but not awaited on every
return path. A worker makes the promise when it signals that it finished: it
closes or sends on a channel, or calls `Done` on a `sync.WaitGroup`. The
launching function must then receive, wait, or hand the channel or group to
code that does. Launching background work with no such promise is not a
diagnostic, in any mode.

Joins are recognized through helpers in this or other packages, through
`select` arms, and through a counted loop that receives one message from each
worker. When the completion signal reaches code the analyzer cannot see
through, nothing is reported. A worker launched once by `main.main` of package
`main` is not reported, because program exit stops it.

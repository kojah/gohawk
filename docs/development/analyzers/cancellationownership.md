# cancellationownership design notes

The public page is [cancellationownership](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

For a standard context derived directly from a fresh local cancelable context,
the classifier also considers use of the exact parent's cancel function after
the child is created. Parent cancellation or ownership handoff makes child
cleanup uncertain rather than proving a leak. This reuses the existing
path-sensitive classification; an unrelated or only conditionally canceled
parent does not cover an otherwise unowned return. A parent held in a local
cell is resolved at the child-creation instruction, including when that cell
later holds the child. Ambiguous or detached parents remain outside this
boundary. Signal registrations still need their own stop
function even when their parent is canceled.

Receiving the exact standard context child's `Done` channel declines a loss
claim on that path. In a `select`, this applies only to returns dominated by
the selected receive arm; other cases and default arms still need cleanup.
This is uncertainty about outstanding cleanup, not proof of a synchronous
cancel call. Signal contexts still require unregistration after a signal.

A helper that calls the cancel function only behind a Boolean parameter
settles it at a call whose constant argument selects the calling branch,
through the same argument binding and summary cases as `resourcelifetime`.
For an imported helper, whose body is not visible, the classifier asks the
lifecycle evidence once the body search is undecided: a summary that calls
the exact cancel function synchronously, on every return or in the case the
constant arguments select, is a release. Only a proof counts. A helper that
calls it on another goroutine, or calls a different argument, stays unknown.
A local helper called with the constant that skips the call is reported. An
imported one is not: summary cases carry only positive guarantees, so a
summary that does not claim the call is not proof that the helper never
cancels, and the call stays unknown. Fixtures:
`cancellationownership/argument_cases.go`.

Local callback invocation uses the same structured completion engine as
method cleanup, including exact identity, stable capture mapping, callback
bindings and bounded recursion. The Boolean adapter projects only a proven
result; unavailable or recursively rejected bodies remain unknown to the
underlying proof. Invocation on another goroutine never establishes a
synchronous release. Shared tests in `lifecycle/completion_invocation_test.go`
cover replaced and mixed callbacks, conditional invocation, bound invokers,
asynchronous launches, recursion and budget exhaustion.

A deferred literal that captures the cancel function is judged exactly when
the capture is simple: the cancel function is stored once into a cell that
only directly deferred literals read, each deferred after the store. Such a
literal that calls cancel on every return releases it where it is deferred.
One whose call turns on a named result, as `defer func() { if err != nil {
cancel() } }()` does, settles nothing at the defer; each return it dominates
asks the shared completion search (`lifecycle.ResultGuards`) whether the
literal calls cancel given the value that return stores, so a success path
returning nil without cancelling is reported, as grpc-go's ALTS handshake
was before its fix. Any other capture keeps the store an opaque use: a cell
the function also reads (a call through the loaded variable is not
recognized), a literal deferred before the store (the walk never meets it),
or one launched or handed to a callee. Fixtures:
`cancellationownership/result_guarded_defers.go`.

Literal outcomes are checked before asking the summary broker. An unconditional
result guarantee is projected by `resultfacts.Guarantee.Outcome`, shared with
resource lifetime; unknown guarantees cannot select the cleanup branch. The
classifier retains its own budget and absent-provider boundary.

Elapsed sleep durations and command-wide process lifetimes remain known
precision gaps, not blanket exemptions for timers or command entry points.

### Cancels owned by a returned struct

A constructor commonly stores its cancel in a field of the struct it
returns, directly or as a closure that calls it, and returns early with an
error before that struct reaches the caller. When the struct is allocated in
this function, the cancel reaches exactly one of its fields, and the struct is
used only through field addresses and returns, the struct owns the cancel:
returning it is a transfer, and a return that drops it leaves the cancel
uncalled. A closure qualifies when it captures a cell written once with the
cancel (`ssaflow.WrittenOnceCell`) and is stored straight into the field. Any
other use of the struct, such as passing it to a function, publishing it, or
capturing it, keeps the ordinary classification and the obligation unknown;
so do a cell written twice, written after capture, or read before the store,
and a closure handed to opaque code. Reading the cancel back out of the
field is not an exact release. Fixtures:
`cancellationownership/owner_structs.go`.

Of the recall audit's captured-cell misses (stage 3), grpc-go's
`ClientHandshake` is now reported through the result-guarded defer rule, not
this one. go-test's `prepareTestRun` hands the cancel to goroutines that call
it on failure, which cannot be ordered against the function's returns, so it
stays unknown. hermesx drops its cancel on an error return before the owning
struct exists, after passing the context to other components, which is also
unknown.

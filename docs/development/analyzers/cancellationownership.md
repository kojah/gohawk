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

Elapsed sleep durations and command-wide process lifetimes remain known
precision gaps, not blanket exemptions for timers or command entry points.

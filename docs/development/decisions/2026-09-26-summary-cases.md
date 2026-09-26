# Summary cases: bounded, positive, and selected by the call

Decided 2026-09-26.

A lifecycle summary may hold cases: positive cleanup guarantees that hold
under a condition a caller can check, on a result or on Boolean arguments
fixed to constants. This follows Infer Pulse's disjunctive summaries in shape,
but not in approximation. Pulse drops disjuncts when it has too many, which is
sound for reporting a bug it found and unsound for the other half of gohawk's
facts, the claims that suppress a diagnostic because a callee is proven to
clean up. So a gohawk case is only ever a positive guarantee proven on every
normal return under its condition, a missing case is unknown, and a function
whose cases would exceed the bound keeps exactly the unconditional summary it
had before.

The bound is structural. Result conditions cover at most four result slots, as
before. Argument conditions cover at most two guarding Boolean parameters, so
at most eight assignments. Cases need no solver: a condition is either a
result outcome the caller tests or a constant the call passes, and a branch is
decided only when it tests the bound value itself or its negation.

The result-conditioned records that existed before are cases with no argument
condition. There is one list, one proof, `lifecycle.ProveCompletionForCase`,
and one condition type, `ssaflow.CallCondition`, with one matching rule,
`Matches`. Every other summary that holds under a condition moves onto the same
type, so there is one vocabulary to prove, export, and match; the completion
search's own result-test enum was the first duplicate removed.

The first uses are `resourcelifetime` and `cancellationownership`: a helper
that cleans up only behind a flag settles the resource at a call passing the
constant that selects the cleanup, locally and across packages, and a deferred
call passing the other constant is reported. Binding the callee's branches
also exposed a completion-search mapping that credited an unlock of one mutex
as releasing a sibling mutex of the same owner; the target now maps as the
mirrored field, and `lockorder` reports the order that was lost.

## One condition vocabulary

Every claim of the form "X holds when Y" now names Y with
`ssaflow.CallCondition`: cleanup cases and the unconditional discharges share
one `Discharges` list whose empty condition is the Must claim; synchronous
invocation is a discharge of its own method; result facts are result cases,
an outcome of one result under a condition that may name a parameter's
nilness or a paired error's outcome; and `lockorder` names the result a lock
is held under the same way. The completion search's private result-test enum
and the result facts' relation kinds are gone.

Two conditional shapes stay separate on purpose. The concurrency facts' path
alternatives carry a path-condition language over SSA values: comparisons with
integer, string, nil, and Boolean constants, scoped by the call sites a path
was bound through, and implied facts about what a helper returned. They select
feasible effect sequences while summaries compose, rather than guaranteeing a
claim, and only a small subset overlaps `CallCondition`; folding that subset
would split one condition across two representations, and folding all of it
would load opaque branch identities into the shared claim condition. The heap
projection's must-hold flag is judged on every return with a non-nil result
for every hold alike, so it is not a condition a caller selects.

Still excluded: predicates on non-Boolean arguments, relations between
arguments, and negative cases, which a use-after-release or latent-bug report
would need. Those are the next two phases and get their own decision.
